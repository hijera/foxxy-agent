package llm

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// freezingProxy is a CONNECT proxy that can freeze the tunnels it already holds:
// they stay open and pass nothing, the way a proxy that lost its state for them
// behaves, while tunnels opened afterwards work normally. Every destination is
// reached on loopback, so a test can name a host that is not loopback.
type freezingProxy struct {
	addr string

	mu      sync.Mutex
	cutoff  int // tunnels numbered below this are frozen
	tunnels int
}

func newFreezingProxy(t *testing.T) *freezingProxy {
	t.Helper()
	p := &freezingProxy{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}
		_, port, _ := net.SplitHostPort(r.Host)
		up, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", port))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		p.mu.Lock()
		id := p.tunnels
		p.tunnels++
		p.mu.Unlock()
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = up.Close()
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		_ = rw.Flush()
		go p.pipe(id, up, rw)
		p.pipe(id, conn, up)
		_ = conn.Close()
		_ = up.Close()
	}))
	t.Cleanup(srv.Close)
	p.addr = srv.Listener.Addr().String()
	return p
}

// pipe copies src to dst for tunnel id, dropping everything once it is frozen.
func (p *freezingProxy) pipe(id int, dst io.Writer, src io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 && !p.frozen(id) {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *freezingProxy) frozen(id int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return id < p.cutoff
}

// freeze freezes every tunnel open so far.
func (p *freezingProxy) freeze() {
	p.mu.Lock()
	p.cutoff = p.tunnels
	p.mu.Unlock()
}

func (p *freezingProxy) tunnelCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tunnels
}

func shortenHTTP2Pings(t *testing.T) {
	t.Helper()
	prevSend, prevWait := llmHTTP2SendPingTimeout, llmHTTP2PingTimeout
	llmHTTP2SendPingTimeout, llmHTTP2PingTimeout = 200*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { llmHTTP2SendPingTimeout, llmHTTP2PingTimeout = prevSend, prevWait })
}

func getWithin(c *http.Client, target string, d time.Duration) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, nil
}

// The hang seen behind a proxy: every retry of a model call went out on the same
// pooled HTTP/2 connection (reused=true, one local port in the trace) after the
// proxy had stopped passing anything on it, so each attempt waited out the whole
// first-token timeout and the turn sat on "provider is not responding" for as
// long as the retry ladder ran. With HTTP/2 pings the dead connection is noticed
// and dropped, and the next request opens a new tunnel.
func TestDeadHTTP2ConnectionBehindAProxyIsReplaced(t *testing.T) {
	shortenHTTP2Pings(t)
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		t.Setenv(k, "")
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	target := "https://llm.test:" + u.Port() + "/v1/models"

	p := newFreezingProxy(t)
	c, err := HTTPClientForOptionalProxy("http://" + p.addr)
	if err != nil {
		t.Fatal(err)
	}
	UnwrapTransport(c.Transport).(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test server certificate

	resp, err := getWithin(c, target, 5*time.Second)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if resp.ProtoMajor != 2 {
		t.Fatalf("the test needs HTTP/2 to the target, got %s", resp.Proto)
	}

	p.freeze()

	// Within a few seconds a request must get through again, on a new tunnel.
	deadline := time.Now().Add(6 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := getWithin(c, target, 2*time.Second)
		if err == nil && resp.StatusCode == http.StatusOK {
			if p.tunnelCount() < 2 {
				t.Fatalf("answered without a new tunnel: the frozen connection cannot have carried it")
			}
			return
		}
		lastErr = err
	}
	t.Fatalf("no request got through after the proxy froze the pooled connection (tunnels opened: %d, last error: %v)", p.tunnelCount(), lastErr)
}

// A connection the HTTP/2 health check closed under a request fails that request
// with "client connection lost"; nothing reached the caller, so the retry layer
// repeats it on a fresh connection instead of ending the call.
func TestLostHTTP2ConnectionIsRetryable(t *testing.T) {
	err := errors.New(`openai stream: Post "https://api.neuraldeep.ru/v1/chat/completions": http2: client connection lost`)
	if !isRetryableLLMError(err) {
		t.Fatal("a lost HTTP/2 connection must be retried")
	}
}

// The same failure one layer up, as the agent sees it: a streamed completion on
// the provider after the proxy froze the pooled connection. The health check
// closes the connection under the call, the retry layer repeats it on a new
// tunnel, and the call returns the answer instead of hanging until the caller's
// first-token timer cuts it.
func TestProviderStreamRecoversFromADeadHTTP2Connection(t *testing.T) {
	shortenHTTP2Pings(t)
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		t.Setenv(k, "")
	}
	const sse = "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	// NewProvider builds its own client from a clone of the default transport;
	// let that clone trust the test server's certificate for this test only.
	dt := http.DefaultTransport.(*http.Transport)
	prevTLS := dt.TLSClientConfig
	dt.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test server certificate
	t.Cleanup(func() { dt.TLSClientConfig = prevTLS })

	p := newFreezingProxy(t)
	provider, err := NewProvider(ProviderInput{
		Type:          "openai",
		Model:         "m",
		APIKey:        "k",
		BaseURL:       "https://llm.test:" + u.Port() + "/v1",
		ProxyURL:      "http://" + p.addr,
		RetryMax:      2,
		RetryBase:     time.Millisecond,
		RetryMaxDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := func(d time.Duration) (*Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		return provider.Stream(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	}
	if resp, err := stream(5 * time.Second); err != nil || resp.Content != "ok" {
		t.Fatalf("first call: %v %+v", err, resp)
	}

	p.freeze()
	start := time.Now()
	resp, err := stream(8 * time.Second)
	if err != nil || resp.Content != "ok" {
		t.Fatalf("call over the frozen connection: %v %+v (tunnels opened: %d)", err, resp, p.tunnelCount())
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("recovered only after %v", took)
	}
	if p.tunnelCount() < 2 {
		t.Fatal("answered without a new tunnel")
	}
}
