// Remote MCP transports: streamable HTTP (2025-03-26 spec) and the legacy
// HTTP+SSE transport (2024-11-05 spec), plus the shared SSE stream parser.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// httpClientFor returns the client a remote transport talks through. With
// insecure set, TLS certificates are accepted without verification, which is
// what lets a server behind a self-signed or expired certificate connect at
// all; it also removes the protection against a man in the middle, so it is
// per-server and opt-in.
func httpClientFor(insecure bool) *http.Client {
	if !insecure {
		return http.DefaultClient
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	t := base.Clone()
	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	t.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // intentional: the per-server insecure_skip_verify opt-in
	return &http.Client{Transport: t}
}

// NewHTTPClient connects over the streamable HTTP transport: JSON-RPC
// messages are POSTed to url and answered either as application/json bodies
// or as text/event-stream chunks. When the endpoint rejects the handshake
// (legacy servers answer POST with 4xx), the client falls back to the
// HTTP+SSE transport at the same URL, mirroring what Cursor and Claude Code
// do for url-only entries.
//
// insecure disables TLS certificate verification for this server.
func NewHTTPClient(ctx context.Context, name, rawURL string, headers map[string]string, insecure bool, log *slog.Logger) (*Client, error) {
	tr := &streamableHTTPTransport{
		url:     rawURL,
		headers: headers,
		hc:      httpClientFor(insecure),
		msgs:    make(chan []byte, 16),
		closed:  make(chan struct{}),
	}
	client, streamErr := newClientWithTransport(ctx, name, tr, log)
	if streamErr == nil {
		return client, nil
	}
	sseClient, sseErr := NewSSEClient(ctx, name, rawURL, headers, insecure, log)
	if sseErr == nil {
		log.Info("mcp streamable http failed; connected via legacy SSE", "server", name, "error", streamErr)
		return sseClient, nil
	}
	return nil, fmt.Errorf("streamable http: %v; sse fallback: %w", streamErr, sseErr)
}

// NewSSEClient connects over the legacy HTTP+SSE transport: a GET stream at
// url announces the POST endpoint in its first event and then carries every
// server->client JSON-RPC message.
func NewSSEClient(ctx context.Context, name, rawURL string, headers map[string]string, insecure bool, log *slog.Logger) (*Client, error) {
	tr, err := newSSETransport(ctx, name, rawURL, headers, insecure)
	if err != nil {
		return nil, err
	}
	return newClientWithTransport(ctx, name, tr, log)
}

// ---- streamable HTTP transport ----

type streamableHTTPTransport struct {
	url     string
	headers map[string]string
	hc      *http.Client
	msgs    chan []byte

	mu        sync.Mutex
	sessionID string

	closed chan struct{}
}

// httpStatusError reports a non-2xx response to a JSON-RPC POST.
type httpStatusError struct {
	status int
	body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("http %d: %s", e.status, strings.TrimSpace(e.body))
}

func (t *streamableHTTPTransport) Send(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	t.mu.Lock()
	if t.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	resp, err := t.hc.Do(req)
	if err != nil {
		return err
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		t.sessionID = sid
		t.mu.Unlock()
	}

	switch {
	case resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent:
		// Notification (or fire-and-forget request) accepted.
		_ = resp.Body.Close()
		return nil
	case resp.StatusCode >= 400:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return &httpStatusError{status: resp.StatusCode, body: string(body)}
	case strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"):
		// Responses stream in as SSE events; read them in the background
		// until the server closes this response stream (its lifetime is
		// bounded by the request ctx of the call that opened it).
		go func() {
			defer func() { _ = resp.Body.Close() }()
			_ = readSSE(resp.Body, func(event, data string) {
				t.deliver([]byte(data))
			})
		}()
		return nil
	default:
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(body)) > 0 {
			t.deliver(body)
		}
		return nil
	}
}

func (t *streamableHTTPTransport) deliver(data []byte) {
	select {
	case t.msgs <- data:
	case <-t.closed:
	}
}

func (t *streamableHTTPTransport) Messages() <-chan []byte { return t.msgs }

func (t *streamableHTTPTransport) Close() error {
	select {
	case <-t.closed:
	default:
		close(t.closed)
	}
	return nil
}

// ---- legacy HTTP+SSE transport ----

type sseTransport struct {
	endpoint string
	headers  map[string]string
	hc       *http.Client
	msgs     chan []byte
	cancel   context.CancelFunc
	closed   chan struct{}
}

func newSSETransport(ctx context.Context, name, rawURL string, headers map[string]string, insecure bool) (*sseTransport, error) {
	// One client for both halves of the transport: the GET event stream opened
	// here and the POSTs Send makes to the announced endpoint.
	hc := httpClientFor(insecure)
	// The event stream must outlive the connect ctx: it carries every later
	// response, so it gets its own lifetime, cancelled by Close. The connect
	// phase (GET headers + endpoint event), however, must honor the caller's
	// deadline - AfterFunc aborts the stream if ctx dies while connecting, so
	// a server that accepts TCP but never answers cannot hang Connect/probe.
	streamCtx, cancel := context.WithCancel(context.Background())
	stopConnectGuard := context.AfterFunc(ctx, cancel)
	defer stopConnectGuard()

	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("mcp %s: sse connect: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("mcp %s: sse connect: http %d %s: %s",
			name, resp.StatusCode, resp.Header.Get("Content-Type"), strings.TrimSpace(string(body)))
	}

	t := &sseTransport{
		headers: headers,
		hc:      hc,
		msgs:    make(chan []byte, 16),
		cancel:  cancel,
		closed:  make(chan struct{}),
	}

	endpointCh := make(chan string, 1)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		// This goroutine is the sole msgs writer: closing the channel when
		// the stream dies fails pending calls fast instead of letting them
		// hang until their ctx expires.
		defer close(t.msgs)
		_ = readSSE(resp.Body, func(event, data string) {
			if event == "endpoint" {
				select {
				case endpointCh <- data:
				default:
				}
				return
			}
			// Default (and explicit "message") events carry JSON-RPC payloads.
			select {
			case t.msgs <- []byte(data):
			case <-t.closed:
			}
		})
	}()

	select {
	case raw := <-endpointCh:
		endpoint, err := resolveSSEEndpoint(rawURL, raw)
		if err != nil {
			_ = t.Close()
			return nil, fmt.Errorf("mcp %s: sse endpoint: %w", name, err)
		}
		t.endpoint = endpoint
	case <-ctx.Done():
		_ = t.Close()
		return nil, fmt.Errorf("mcp %s: sse endpoint event not received: %w", name, ctx.Err())
	}
	return t, nil
}

// resolveSSEEndpoint resolves the endpoint event payload (usually a relative
// URI) against the SSE stream URL.
//
// The payload is server-controlled, and every later POST carries the configured
// headers (typically an Authorization bearer). An endpoint pointing at another
// origin would therefore hand those credentials to a host the operator never
// configured, so a cross-origin endpoint is refused rather than followed - the
// same restriction the MCP transport spec places on this event.
func resolveSSEEndpoint(base, endpoint string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	e, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	resolved := b.ResolveReference(e)
	if !sameOrigin(b, resolved) {
		return "", fmt.Errorf("endpoint %s is not on the stream origin %s", resolved.Redacted(), originOf(b))
	}
	return resolved.String(), nil
}

// sameOrigin compares scheme, host, and port, treating the default port of a
// scheme as equal to that port written out.
func sameOrigin(a, b *url.URL) bool {
	return originOf(a) == originOf(b)
}

// originOf renders scheme://host:port with the scheme's default port made
// explicit, so "https://h" and "https://h:443" compare equal.
func originOf(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + host + ":" + port
}

func (t *sseTransport) Send(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	resp, err := t.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &httpStatusError{status: resp.StatusCode, body: string(body)}
	}
	// Responses arrive over the event stream; any 2xx body is ignored.
	return nil
}

func (t *sseTransport) Messages() <-chan []byte { return t.msgs }

func (t *sseTransport) Close() error {
	select {
	case <-t.closed:
	default:
		close(t.closed)
	}
	t.cancel()
	return nil
}

// ---- SSE parsing ----

// readSSE parses a text/event-stream, invoking emit once per event with the
// event name ("" for unnamed = "message" events) and the joined data payload.
func readSSE(r io.Reader, emit func(event, data string)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	event := ""
	var dataLines []string
	flush := func() {
		if len(dataLines) > 0 {
			emit(event, strings.Join(dataLines, "\n"))
		}
		event = ""
		dataLines = nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, ":"):
			// comment / keep-alive
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	flush()
	return scanner.Err()
}
