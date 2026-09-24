package llm

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Connection-level trace of LLM HTTP traffic, on while debug.enable is.
//
// A call that hangs behind a proxy looks the same from the agent loop whatever
// actually broke: the proxy never answered the tunnel, TLS never finished, a
// pooled connection was already dead, or the upstream took the request and went
// quiet. The trace logs every request's route and each step of it - DNS, dial,
// SOCKS dial, proxy CONNECT, TLS, connection reuse, first byte, body reads -
// plus a heartbeat while it sits silent, so a debug log from a remote machine
// says which step the time went into and how the request ended.
//
// Unlike the body capture next to it (debug_transport.go) it logs no payloads,
// so it follows debug.enable alone and stays on under capture_llm: false.

var netTrace atomic.Bool

// SetNetTrace turns the connection-level trace on or off process-wide.
func SetNetTrace(on bool) { netTrace.Store(on) }

// NetTraceEnabled reports whether the connection-level trace is on.
func NetTraceEnabled() bool { return netTrace.Load() }

// netTraceHeartbeatNS is how long a request may sit without any network event
// before the trace says so, and how often it says it again while the silence
// lasts. Atomic because a watcher of an earlier request may still read it.
var netTraceHeartbeatNS atomic.Int64

func init() { setNetTraceHeartbeat(15 * time.Second) }

func netTraceHeartbeat() time.Duration     { return time.Duration(netTraceHeartbeatNS.Load()) }
func setNetTraceHeartbeat(d time.Duration) { netTraceHeartbeatNS.Store(int64(d)) }

var netTraceSeq atomic.Uint64

type netTraceAttrsKey struct{}

type netTracerKey struct{}

// WithNetTraceAttrs labels every trace line of the requests made under ctx. The
// agent passes its session, turn and call number, so a request that hung can be
// matched to the turn that waited on it.
func WithNetTraceAttrs(ctx context.Context, attrs ...any) context.Context {
	prev, _ := ctx.Value(netTraceAttrsKey{}).([]any)
	merged := append(append([]any(nil), prev...), attrs...)
	return context.WithValue(ctx, netTraceAttrsKey{}, merged)
}

// netRoute is where a request goes: desc for the log, tunnel when the proxy is an
// HTTP one that the target's TLS has to cross with a CONNECT.
type netRoute struct {
	desc   string
	tunnel bool
}

type routeFunc func(target *url.URL) netRoute

// routeVia describes the route a proxy resolver picks. kind names the proxy
// ("proxy", "socks", "env-proxy"); direct is what a target the resolver sends
// straight gets, which differs between "no proxy at all" and "proxy bypassed".
func routeVia(kind, direct string, proxyFor func(*url.URL) (*url.URL, error)) routeFunc {
	return func(target *url.URL) netRoute {
		p, err := proxyFor(target)
		switch {
		case err != nil:
			return netRoute{desc: "unresolved: " + err.Error()}
		case p == nil:
			return netRoute{desc: direct}
		}
		tunnel := target.Scheme == "https" && (p.Scheme == "http" || p.Scheme == "https")
		return netRoute{desc: kind + " " + redactProxyURL(p), tunnel: tunnel}
	}
}

// redactProxyURL drops the proxy's credentials whole: a user name alone is often
// the token.
func redactProxyURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	c := *u
	if c.User != nil {
		c.User = url.User("redacted")
	}
	return c.String()
}

// netTracer follows one request. A nil *netTracer is a working no-op, so the
// dial hooks carry no enabled/disabled branching of their own.
type netTracer struct {
	log    *slog.Logger
	attrs  []any
	start  time.Time
	tunnel bool
	stop   chan struct{}

	mu            sync.Mutex
	phase         string
	last          time.Time // the last network event of any kind
	silentReports int       // heartbeats logged for the current silence
	dnsStart      time.Time
	dialStart     time.Time
	tlsStart      time.Time
	inSOCKS       bool // a SOCKS dial is under way: its TCP connect is to the proxy
	bytes         int64
	reads         int
	maxGap        time.Duration
	firstBody     time.Duration
	finished      bool
}

// startNetTrace logs the request and returns it re-bound to a context that
// carries the tracer and its httptrace hooks.
func startNetTrace(req *http.Request, route routeFunc) (*netTracer, *http.Request) {
	now := time.Now()
	nt := &netTracer{
		log:   debugLogger(),
		attrs: []any{"req", netTraceSeq.Add(1)},
		start: now,
		stop:  make(chan struct{}),
		phase: "get_conn",
		last:  now,
	}
	if labels, ok := req.Context().Value(netTraceAttrsKey{}).([]any); ok {
		nt.attrs = append(nt.attrs, labels...)
	}
	r := netRoute{desc: "unknown"}
	if route != nil {
		r = route(req.URL)
	}
	nt.tunnel = r.tunnel
	nt.debug("llm net: request",
		"method", req.Method,
		"url", req.URL.Scheme+"://"+req.URL.Host+req.URL.Path,
		"route", r.desc)

	ctx := context.WithValue(req.Context(), netTracerKey{}, nt)
	ctx = httptrace.WithClientTrace(ctx, nt.clientTrace())
	req = req.WithContext(ctx)
	go nt.watch(ctx)
	return nt, req
}

// netTracerFrom finds the tracer of the request a dial or a CONNECT belongs to.
func netTracerFrom(ctx context.Context) *netTracer {
	if ctx == nil {
		return nil
	}
	nt, _ := ctx.Value(netTracerKey{}).(*netTracer)
	return nt
}

func (nt *netTracer) debug(msg string, kv ...any) {
	if nt == nil {
		debugLogger().Debug(msg, kv...)
		return
	}
	nt.log.Debug(msg, append(append([]any(nil), nt.attrs...), kv...)...)
}

// touch records a network event. A silence the heartbeat already reported is
// closed with a line saying how long it lasted.
func (nt *netTracer) touch(now time.Time, phase string) {
	if nt == nil {
		return
	}
	nt.mu.Lock()
	silence := now.Sub(nt.last)
	reported := nt.silentReports > 0
	prev := nt.phase
	nt.last = now
	nt.silentReports = 0
	if phase != "" {
		nt.phase = phase
	}
	nt.mu.Unlock()
	if reported {
		nt.debug("llm net: activity resumed", "phase", prev, "silence", silence.Round(time.Millisecond))
	}
}

func (nt *netTracer) setSOCKS(on bool) {
	if nt == nil {
		return
	}
	nt.mu.Lock()
	nt.inSOCKS = on
	nt.mu.Unlock()
}

func (nt *netTracer) elapsed(now time.Time) time.Duration {
	return now.Sub(nt.start).Round(time.Millisecond)
}

func (nt *netTracer) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			nt.touch(time.Now(), "get_conn")
		},
		DNSStart: func(info httptrace.DNSStartInfo) {
			now := time.Now()
			nt.mu.Lock()
			nt.dnsStart = now
			nt.mu.Unlock()
			nt.touch(now, "dns")
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			now := time.Now()
			nt.mu.Lock()
			took := now.Sub(nt.dnsStart)
			nt.mu.Unlock()
			nt.touch(now, "")
			addrs := make([]string, 0, len(info.Addrs))
			for _, a := range info.Addrs {
				addrs = append(addrs, a.String())
			}
			kv := []any{"addrs", strings.Join(addrs, ","), "took", took.Round(time.Millisecond)}
			if info.Err != nil {
				kv = append(kv, netErrAttrs(info.Err)...)
			}
			nt.debug("llm net: dns", kv...)
		},
		ConnectStart: func(network, addr string) {
			now := time.Now()
			nt.mu.Lock()
			nt.dialStart = now
			nt.mu.Unlock()
			nt.touch(now, "dial")
		},
		ConnectDone: func(network, addr string, err error) {
			now := time.Now()
			nt.mu.Lock()
			took := now.Sub(nt.dialStart)
			nt.mu.Unlock()
			// Behind an HTTP proxy the next step for a TLS target is the CONNECT
			// exchange: a proxy that never answers it hangs here.
			nt.mu.Lock()
			next := "connected"
			switch {
			case err != nil:
				// A failed attempt leaves the request in the dial: the next
				// address, or the failure, comes from there.
				next = ""
			case nt.inSOCKS:
				next = "socks_handshake"
			case nt.tunnel:
				next = "proxy_connect"
			}
			nt.mu.Unlock()
			nt.touch(now, next)
			kv := []any{"addr", addr, "took", took.Round(time.Millisecond)}
			if err != nil {
				kv = append(kv, netErrAttrs(err)...)
			}
			nt.debug("llm net: dialed", kv...)
		},
		TLSHandshakeStart: func() {
			now := time.Now()
			nt.mu.Lock()
			nt.tlsStart = now
			nt.mu.Unlock()
			nt.touch(now, "tls")
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			now := time.Now()
			nt.mu.Lock()
			took := now.Sub(nt.tlsStart)
			nt.mu.Unlock()
			nt.touch(now, "")
			kv := []any{
				"server_name", state.ServerName,
				"version", tls.VersionName(state.Version),
				"alpn", state.NegotiatedProtocol,
				"resumed", state.DidResume,
				"took", took.Round(time.Millisecond),
			}
			if err != nil {
				kv = append(kv, netErrAttrs(err)...)
			}
			nt.debug("llm net: tls", kv...)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			now := time.Now()
			nt.touch(now, "sending_request")
			kv := []any{"reused", info.Reused, "was_idle", info.WasIdle}
			if info.WasIdle {
				kv = append(kv, "idle_for", info.IdleTime.Round(time.Millisecond))
			}
			// The local address names the connection: a request that hangs on a
			// reused one shows the same local port as the request before it.
			if info.Conn != nil {
				kv = append(kv, "local", info.Conn.LocalAddr().String(), "remote", info.Conn.RemoteAddr().String())
			}
			kv = append(kv, "elapsed", nt.elapsed(now))
			nt.debug("llm net: conn", kv...)
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			now := time.Now()
			nt.touch(now, "awaiting_response")
			kv := []any{"elapsed", nt.elapsed(now)}
			if info.Err != nil {
				kv = append(kv, netErrAttrs(info.Err)...)
			}
			nt.debug("llm net: request sent", kv...)
		},
		GotFirstResponseByte: func() {
			now := time.Now()
			nt.touch(now, "reading_headers")
			nt.debug("llm net: first response byte", "elapsed", nt.elapsed(now))
		},
	}
}

// watch logs the silence of a request that is waiting on the network, and the
// moment its context ends. It stops when the request finishes.
func (nt *netTracer) watch(ctx context.Context) {
	hb := netTraceHeartbeat()
	tick := hb / 4
	if tick < time.Millisecond {
		tick = time.Millisecond
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-nt.stop:
			return
		case <-ctx.Done():
			nt.mu.Lock()
			finished := nt.finished
			phase := nt.phase
			idle := time.Since(nt.last)
			nt.mu.Unlock()
			if finished {
				return
			}
			kv := []any{"phase", phase, "error", ctx.Err().Error(), "idle", idle.Round(time.Millisecond), "elapsed", nt.elapsed(time.Now())}
			if cause := context.Cause(ctx); cause != nil && cause != ctx.Err() {
				kv = append(kv, "cause", cause.Error())
			}
			nt.debug("llm net: request context ended", kv...)
			return
		case now := <-t.C:
			nt.mu.Lock()
			if nt.finished {
				nt.mu.Unlock()
				return
			}
			idle := now.Sub(nt.last)
			due := idle >= hb*time.Duration(nt.silentReports+1)
			if due {
				nt.silentReports++
			}
			phase, bytes := nt.phase, nt.bytes
			nt.mu.Unlock()
			if due {
				nt.debug("llm net: no activity",
					"phase", phase,
					"idle", idle.Round(time.Millisecond),
					"elapsed", nt.elapsed(now),
					"bytes", bytes)
			}
		}
	}
}

// observe logs what the transport returned and, for a response, wraps its body
// so reads and the end of the stream are followed too.
func (nt *netTracer) observe(resp *http.Response, err error) (*http.Response, error) {
	if nt == nil {
		return resp, err
	}
	if err != nil {
		nt.finish("request_failed", err)
		return resp, err
	}
	now := time.Now()
	nt.touch(now, "streaming")
	kv := []any{
		"status", resp.Status,
		"proto", resp.Proto,
		"content_type", resp.Header.Get("Content-Type"),
		"content_length", resp.ContentLength,
		"elapsed", nt.elapsed(now),
	}
	if id := responseRequestID(resp.Header); id != "" {
		kv = append(kv, "request_id", id)
	}
	nt.debug("llm net: response", kv...)
	if resp.Body == nil {
		nt.finish("no_body", nil)
		return resp, nil
	}
	resp.Body = &tracedBody{src: resp.Body, nt: nt}
	return resp, nil
}

// responseRequestID is the id a provider or a proxy in front of it stamped on the
// response, the one thing their support can look a request up by.
func responseRequestID(h http.Header) string {
	for _, k := range []string{"X-Request-Id", "Request-Id", "X-Amzn-Requestid", "Cf-Ray"} {
		if v := h.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func (nt *netTracer) onRead(n int, now time.Time) {
	if n <= 0 {
		return
	}
	nt.mu.Lock()
	gap := now.Sub(nt.last)
	if nt.reads == 0 {
		nt.firstBody = now.Sub(nt.start)
	} else if gap > nt.maxGap {
		nt.maxGap = gap
	}
	nt.reads++
	nt.bytes += int64(n)
	nt.mu.Unlock()
	nt.touch(now, "streaming")
}

func (nt *netTracer) readFailed(err error) {
	nt.mu.Lock()
	phase := nt.phase
	idle := time.Since(nt.last)
	bytes := nt.bytes
	nt.mu.Unlock()
	kv := append([]any{"phase", phase, "idle", idle.Round(time.Millisecond), "bytes", bytes, "elapsed", nt.elapsed(time.Now())}, netErrAttrs(err)...)
	nt.debug("llm net: body read failed", kv...)
}

// finish logs how the request ended, once, and stops the watcher.
func (nt *netTracer) finish(outcome string, err error) {
	nt.mu.Lock()
	if nt.finished {
		nt.mu.Unlock()
		return
	}
	nt.finished = true
	now := time.Now()
	kv := []any{
		"outcome", outcome,
		"phase", nt.phase,
		"elapsed", nt.elapsed(now),
		"bytes", nt.bytes,
		"reads", nt.reads,
		"idle", now.Sub(nt.last).Round(time.Millisecond),
	}
	if nt.reads > 0 {
		kv = append(kv, "first_body_after", nt.firstBody.Round(time.Millisecond), "max_gap", nt.maxGap.Round(time.Millisecond))
	}
	nt.mu.Unlock()
	close(nt.stop)
	if err != nil {
		kv = append(kv, netErrAttrs(err)...)
	}
	msg := "llm net: done"
	if outcome == "request_failed" {
		msg = "llm net: request failed"
	}
	nt.debug(msg, kv...)
}

// tracedBody follows the response body: bytes and gaps as it is read, the first
// read error as it happens, and the outcome when the caller closes it.
type tracedBody struct {
	src io.ReadCloser
	nt  *netTracer
	eof bool
	err error
}

func (b *tracedBody) Read(p []byte) (int, error) {
	n, err := b.src.Read(p)
	b.nt.onRead(n, time.Now())
	switch {
	case err == io.EOF:
		b.eof = true
	case err != nil && b.err == nil:
		b.err = err
		b.nt.readFailed(err)
	}
	return n, err
}

func (b *tracedBody) Close() error {
	cerr := b.src.Close()
	switch {
	case b.err != nil:
		b.nt.finish("read_error", b.err)
	case b.eof:
		b.nt.finish("eof", nil)
	default:
		// The caller stopped reading before the end: a guard or a Stop that
		// cancelled the call, or a reader that had all it wanted.
		b.nt.finish("closed_early", nil)
	}
	return cerr
}

// logProxyConnect is the transport's OnProxyConnectResponse hook: it records the
// proxy's answer to CONNECT, the step a proxy that refuses or drops the tunnel
// fails in. It never changes the outcome.
func logProxyConnect(ctx context.Context, proxyURL *url.URL, connectReq *http.Request, res *http.Response) error {
	if !NetTraceEnabled() {
		return nil
	}
	nt := netTracerFrom(ctx)
	now := time.Now()
	// A CONNECT request names its target in Host; its URL is opaque.
	kv := []any{"proxy", redactProxyURL(proxyURL), "target", connectReq.Host}
	if res != nil {
		kv = append(kv, "status", res.Status)
	}
	if nt != nil {
		nt.mu.Lock()
		took := now.Sub(nt.dialStart)
		nt.mu.Unlock()
		kv = append(kv, "since_dial", took.Round(time.Millisecond))
		nt.touch(now, "tls")
	}
	nt.debug("llm net: proxy CONNECT answered", kv...)
	return nil
}

// traceSOCKSDial wraps the SOCKS dial so the handshake with the proxy, which
// httptrace does not see, is on record with its outcome and duration.
func traceSOCKSDial(proxyURL *url.URL, dial func(ctx context.Context, network, address string) (net.Conn, error)) func(ctx context.Context, network, address string) (net.Conn, error) {
	proxy := redactProxyURL(proxyURL)
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if !NetTraceEnabled() {
			return dial(ctx, network, address)
		}
		nt := netTracerFrom(ctx)
		start := time.Now()
		nt.setSOCKS(true)
		nt.touch(start, "socks_dial")
		conn, err := dial(ctx, network, address)
		now := time.Now()
		nt.setSOCKS(false)
		nt.touch(now, "connected")
		kv := []any{"proxy", proxy, "target", address, "took", now.Sub(start).Round(time.Millisecond)}
		if conn != nil {
			kv = append(kv, "local", conn.LocalAddr().String())
		}
		if err != nil {
			kv = append(kv, netErrAttrs(err)...)
		}
		nt.debug("llm net: socks dial", kv...)
		return conn, err
	}
}

// netErrAttrs spells an error for the log: its text, a kind to grep for, and the
// socket operation and errno when there is one.
func netErrAttrs(err error) []any {
	kv := []any{"error", err.Error(), "error_kind", netErrKind(err)}
	var op *net.OpError
	if errors.As(err, &op) {
		kv = append(kv, "op", op.Op)
		if op.Addr != nil {
			kv = append(kv, "op_addr", op.Addr.String())
		}
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		kv = append(kv, "errno", uintptr(errno))
	}
	return kv
}

// Winsock reports socket failures under its own numbers, which syscall's
// portable constants do not match on Windows.
const (
	wsaeconnaborted = 10053
	wsaeconnreset   = 10054
	wsaetimedout    = 10060
	wsaeconnrefused = 10061
)

// netErrKind classifies a transport failure into the few shapes that tell a
// proxy problem from an upstream one.
func netErrKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected_eof"
	case errors.Is(err, io.EOF):
		return "eof"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNRESET, wsaeconnreset:
			return "conn_reset"
		case syscall.ECONNREFUSED, wsaeconnrefused:
			return "conn_refused"
		case syscall.ECONNABORTED, wsaeconnaborted:
			return "conn_aborted"
		case syscall.ETIMEDOUT, wsaetimedout:
			return "timeout"
		}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "proxyconnect"):
		return "proxy_connect"
	case strings.Contains(s, "socks"):
		return "socks"
	case strings.Contains(s, "timeout awaiting response headers"):
		return "response_header_timeout"
	case strings.Contains(s, "TLS handshake timeout"):
		return "tls_timeout"
	case strings.Contains(s, "tls:") || strings.Contains(s, "x509:"):
		return "tls"
	case strings.Contains(s, "http2:"):
		return "http2"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	return "other"
}
