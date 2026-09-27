package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/netx/proxytest"
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

	proxy := proxytest.HTTP(t, "user", "s3cret")
	proxyURL := "http://user:s3cret@" + proxy.Addr
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

func TestNetTraceOmitsTargetPathAndURLParameters(t *testing.T) {
	buf := withNetTrace(t, time.Minute)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	c, err := HTTPClientForOptionalProxy("")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL + "/private-token?key=query-token")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	logs := waitForLog(t, buf, `msg="llm net: request"`, "url="+srv.URL)
	for _, secret := range []string{"private-token", "query-token"} {
		if strings.Contains(logs, secret) {
			t.Errorf("target URL parameter %q leaked into trace:\n%s", secret, logs)
		}
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

	proxy := proxytest.HTTP(t, "", "")

	c, err := HTTPClientForOptionalProxy("http://" + proxy.Addr)
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

func TestNetTraceLogsTheSOCKSDial(t *testing.T) {
	buf := withNetTrace(t, time.Minute)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "via socks")
	}))
	defer target.Close()
	port := targetPort(t, target)

	c, err := HTTPClientForOptionalProxy("socks5h://" + proxytest.SOCKS5(t, "", "").Addr)
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

	// The final line names the cause whichever notices the cancel first: the
	// watcher ("request context ended") or the body read that fails on it (on
	// Linux the read usually wins, and the watcher then stays quiet).
	logs := waitForLog(t, buf, `msg="llm net: done"`, "session=sess_trace")
	var done string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, `msg="llm net: done"`) {
			done = line
		}
	}
	if !strings.Contains(done, `cause="guard fired"`) || !strings.Contains(done, "ctx_err=") {
		t.Fatalf("final line does not name the cancel and its cause: %s", done)
	}
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

func TestNetRouteRedactsProxyURLParameters(t *testing.T) {
	proxy, err := url.Parse("http://alice:hunter2@proxy.local:3128/private-token?key=query-token#fragment-token")
	if err != nil {
		t.Fatal(err)
	}
	got := redactProxyURL(proxy)
	if got != "http://redacted@proxy.local:3128" {
		t.Fatalf("redacted proxy URL = %q", got)
	}
}

func TestSOCKSRouteMatchesPortSpecificNoProxy(t *testing.T) {
	t.Setenv("NO_PROXY", "api.example.com:443")
	proxy, err := url.Parse("socks5://proxy.local:1080")
	if err != nil {
		t.Fatal(err)
	}
	proxyFor := proxyFuncFor(proxy)
	target, err := url.Parse("https://api.example.com/v1/chat")
	if err != nil {
		t.Fatal(err)
	}
	if got := routeVia("socks", directBypassed, proxyFor)(target).desc; got != directBypassed {
		t.Fatalf("logged route = %q, want direct", got)
	}
	if !bypassProxy(proxyFor, "api.example.com:443") {
		t.Fatal("SOCKS dial would use the proxy despite the logged direct route")
	}
	if bypassProxy(proxyFor, "api.example.com:8443") {
		t.Fatal("SOCKS dial bypassed a port not named by NO_PROXY")
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
