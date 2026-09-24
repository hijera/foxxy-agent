package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a log sink the trace's watcher goroutine may write to while the
// test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// withNetTrace turns the trace on for one test, with a heartbeat short enough to
// observe, and returns the log it writes to.
func withNetTrace(t *testing.T, heartbeat time.Duration) *lockedBuffer {
	t.Helper()
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")
	buf := &lockedBuffer{}
	SetDebugLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	SetDebugCapture(false)
	SetNetTrace(true)
	prev := netTraceHeartbeat()
	setNetTraceHeartbeat(heartbeat)
	t.Cleanup(func() {
		SetNetTrace(false)
		setNetTraceHeartbeat(prev)
		SetDebugLogger(nil)
	})
	return buf
}

// waitForLog polls until the log holds every want, for lines the watcher
// goroutine writes on its own schedule.
func waitForLog(t *testing.T, buf *lockedBuffer, wants ...string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		logs := buf.String()
		missing := ""
		for _, w := range wants {
			if !strings.Contains(logs, w) {
				missing = w
				break
			}
		}
		if missing == "" {
			return logs
		}
		if time.Now().After(deadline) {
			t.Fatalf("log never contained %q; got:\n%s", missing, logs)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// targetPort is the port of an httptest server, reached below under a name that is
// not loopback so the proxy does not bypass it.
func targetPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Port()
}

func TestNetTraceOffLogsNothing(t *testing.T) {
	buf := withNetTrace(t, time.Second)
	SetNetTrace(false)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	c, err := HTTPClientForOptionalProxy("")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if logs := buf.String(); strings.Contains(logs, "llm net") {
		t.Fatalf("trace off must not log, got:\n%s", logs)
	}
}

// A plain-HTTP target through an HTTP proxy: the route names the proxy without its
// password, and the request is followed from the dial to the end of the body.
func TestNetTraceFollowsARequestThroughAnHTTPProxy(t *testing.T) {
	buf := withNetTrace(t, time.Minute)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello")
	}))
	defer target.Close()
	port := targetPort(t, target)

	// A forward proxy: the request arrives in absolute form and is passed on to the
	// loopback target whatever host it names.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := http.NewRequest(r.Method, "http://127.0.0.1:"+port+r.URL.Path, r.Body)
		resp, err := http.DefaultTransport.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	proxyURL := strings.Replace(proxy.URL, "http://", "http://user:s3cret@", 1)
	c, err := HTTPClientForOptionalProxy(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get("http://llm.test:" + port + "/v1/chat")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}

	logs := waitForLog(t, buf,
		`msg="llm net: request"`,
		`route="proxy http://`,
		`msg="llm net: dialed"`,
		`msg="llm net: conn"`,
		`msg="llm net: response"`,
		`msg="llm net: done"`,
		"outcome=eof",
		"bytes=5",
	)
	if strings.Contains(logs, "s3cret") {
		t.Fatalf("the proxy password leaked into the log:\n%s", logs)
	}
}

// An HTTPS target through an HTTP proxy goes through a CONNECT tunnel: the proxy's
// answer and the TLS handshake inside the tunnel are both on record.
func TestNetTraceLogsTheProxyTunnelAndTLS(t *testing.T) {
	buf := withNetTrace(t, time.Minute)

	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "secure")
	}))
	defer target.Close()
	port := targetPort(t, target)

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}
		upstream, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		_ = rw.Flush()
		go func() { _, _ = io.Copy(upstream, conn); _ = upstream.Close() }()
		_, _ = io.Copy(conn, upstream)
		_ = conn.Close()
	}))
	defer proxy.Close()

	c, err := HTTPClientForOptionalProxy(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	tr := UnwrapTransport(c.Transport).(*http.Transport)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test server certificate
	resp, err := c.Get("https://llm.test:" + port + "/")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	waitForLog(t, buf,
		`msg="llm net: proxy CONNECT answered"`,
		`status="200 Connection established"`,
		"target=llm.test:"+port,
		`msg="llm net: tls"`,
		`msg="llm net: done"`,
	)
}

// serveSOCKS5 is the smallest SOCKS5 server the client accepts: no authentication,
// CONNECT only, and every destination name resolved to loopback.
func serveSOCKS5(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer func() { _ = conn.Close() }()
				r := bufio.NewReader(conn)
				hdr := make([]byte, 2)
				if _, err := io.ReadFull(r, hdr); err != nil {
					return
				}
				if _, err := io.ReadFull(r, make([]byte, hdr[1])); err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 0})
				req := make([]byte, 4)
				if _, err := io.ReadFull(r, req); err != nil {
					return
				}
				switch req[3] {
				case 1:
					_, _ = io.ReadFull(r, make([]byte, 4))
				case 3:
					n, _ := r.ReadByte()
					_, _ = io.ReadFull(r, make([]byte, n))
				case 4:
					_, _ = io.ReadFull(r, make([]byte, 16))
				}
				pb := make([]byte, 2)
				if _, err := io.ReadFull(r, pb); err != nil {
					return
				}
				upstream, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
				if err != nil {
					_, _ = conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer func() { _ = upstream.Close() }()
				_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				go func() { _, _ = io.Copy(upstream, r) }()
				_, _ = io.Copy(conn, upstream)
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestNetTraceLogsTheSOCKSDial(t *testing.T) {
	buf := withNetTrace(t, time.Minute)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "via socks")
	}))
	defer target.Close()
	port := targetPort(t, target)

	c, err := HTTPClientForOptionalProxy("socks5h://" + serveSOCKS5(t))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get("http://llm.test:" + port + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "via socks" {
		t.Fatalf("body = %q", body)
	}

	waitForLog(t, buf,
		`route="socks socks5h://`,
		`msg="llm net: socks dial"`,
		"target=llm.test:"+port,
		`msg="llm net: done"`,
	)
}

// A request that sits silent is reported while it waits, with the step it is
// stuck in, and again when data resumes: this is what a hang looks like in the log.
func TestNetTraceReportsSilenceWhileWaiting(t *testing.T) {
	buf := withNetTrace(t, 40*time.Millisecond)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, "a")
		w.(http.Flusher).Flush()
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "b")
	}))
	defer srv.Close()

	c, err := HTTPClientForOptionalProxy("")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	waitForLog(t, buf,
		`msg="llm net: no activity"`,
		"phase=awaiting_response",
		"phase=streaming",
		`msg="llm net: activity resumed"`,
		"max_gap=",
	)
}

// A request that never connects says so, and names the step and the failure.
func TestNetTraceNamesAFailedDial(t *testing.T) {
	buf := withNetTrace(t, time.Minute)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	c, err := HTTPClientForOptionalProxy("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get("http://" + addr + "/"); err == nil {
		t.Fatal("expected a dial failure")
	}

	waitForLog(t, buf,
		`msg="llm net: request failed"`,
		"phase=dial",
		"error_kind=conn_refused",
	)
}

// The caller's labels are on every line, and a request the caller cut is logged
// with the reason the caller gave: that is how a guard's cancel is told apart from
// a connection that died.
func TestNetTraceNamesWhoCancelled(t *testing.T) {
	buf := withNetTrace(t, time.Minute)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, err := HTTPClientForOptionalProxy("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	ctx = WithNetTraceAttrs(ctx, "session", "sess_trace", "turn", 3)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resp.Body.Read(make([]byte, 5)); err != nil {
		t.Fatal(err)
	}
	cancel(errors.New("guard fired"))
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	logs := waitForLog(t, buf,
		`msg="llm net: request context ended"`,
		`cause="guard fired"`,
		`msg="llm net: done"`,
		"session=sess_trace",
	)
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if strings.Contains(line, "llm net") && !strings.Contains(line, "session=sess_trace") {
			t.Fatalf("line without the caller's labels: %s", line)
		}
	}
}

func TestNetRouteDescription(t *testing.T) {
	t.Setenv("NO_PROXY", "")
	proxied, _ := url.Parse("https://api.example.com/v1/chat")
	local, _ := url.Parse("http://127.0.0.1:11434/v1/chat")
	pu, _ := url.Parse("http://alice:hunter2@proxy.local:3128")

	route := routeVia("proxy", "direct (proxy bypassed)", proxyFuncFor(pu))
	if got := route(proxied); got.desc != "proxy http://redacted@proxy.local:3128" || !got.tunnel {
		t.Fatalf("proxied route = %+v", got)
	}
	if got := route(local); got.desc != "direct (proxy bypassed)" || got.tunnel {
		t.Fatalf("loopback route = %+v", got)
	}
	for _, s := range []string{route(proxied).desc, route(local).desc} {
		if strings.Contains(s, "hunter2") || strings.Contains(s, "alice") {
			t.Fatalf("credentials leaked: %s", s)
		}
	}
}

func TestNetErrKind(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{context.Canceled, "canceled"},
		{fmt.Errorf("wrap: %w", context.DeadlineExceeded), "deadline"},
		{io.ErrUnexpectedEOF, "unexpected_eof"},
		{errors.New("net/http: timeout awaiting response headers"), "response_header_timeout"},
		{errors.New("proxyconnect tcp: dial tcp 10.0.0.1:3128: i/o timeout"), "proxy_connect"},
		{errors.New("socks connect tcp 10.0.0.1:1080->api:443: unknown error general SOCKS server failure"), "socks"},
		{errors.New("net/http: TLS handshake timeout"), "tls_timeout"},
		{errors.New("http2: server sent GOAWAY and closed the connection"), "http2"},
		{errors.New("something else"), "other"},
	}
	for _, c := range cases {
		if got := netErrKind(c.err); got != c.want {
			t.Errorf("netErrKind(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
