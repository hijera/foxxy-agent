package llm

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/internal/netx"
)

// fork(transport-debug-wrap): shared provider transports, built the fork's way.
//
// Upstream 1.1.47 builds one transport per proxy setting in this file and
// shares it across the providers every turn builds, so a request reuses the
// previous turn's connection instead of opening a TLS session of its own and
// leaving the last one idle. FoxxyCode takes the sharing, but builds the
// transports from its own proxy helpers (proxy_http_client.go) - the
// response-header timeout, the environment and system proxy resolver, the
// CONNECT logging and the HTTP/2 health check live there - and wraps each
// client in the debug and connection-trace transport, which upstream's type
// assertion on *http.Transport would reject. The stall guard is not a
// transport here at all (stream_idle_guard.go).

type pooledTransport struct {
	t     *http.Transport
	route routeFunc
}

var (
	providerTransportsMu sync.Mutex
	providerTransports   = map[string]pooledTransport{}
)

// providerTransportKey names what a transport for proxyURL was built from: the
// configured proxy and the NO_PROXY list it reads, or, with none, what the
// environment and system proxy resolver reads. A change to any of them builds
// a new transport instead of reusing one that routes the old way.
func providerTransportKey(proxyURL string) string {
	if proxyURL = strings.TrimSpace(proxyURL); proxyURL != "" {
		return "proxy\x00" + proxyURL + "\x00" + noProxyEnv()
	}
	return "env\x00" + netx.EnvironmentProxyKey()
}

// providerTransport returns the shared transport for proxyURL (empty means the
// environment's or the system's proxy), with its route.
func providerTransport(proxyURL string) (*http.Transport, routeFunc, error) {
	key := providerTransportKey(proxyURL)
	providerTransportsMu.Lock()
	defer providerTransportsMu.Unlock()
	if p, ok := providerTransports[key]; ok {
		return p.t, p.route, nil
	}
	t, route, err := buildLLMTransport(proxyURL)
	if err != nil {
		return nil, nil, err
	}
	providerTransports[key] = pooledTransport{t: t, route: route}
	return t, route, nil
}

// providerHTTPClient is the client NewProvider hands the SDKs: the shared
// transport for the proxy setting behind the debug wrapper, and the request
// timeout (providers[].timeout_ms) when one is configured.
func providerHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	t, route, err := providerTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: debugTransportFor(t, route), Timeout: timeout}, nil
}

// streamStalledError reports a streamed response that delivered nothing for
// idle after its first chunk: the connection is open, the model is not
// writing. The stall guard (stream_idle_guard.go) cuts such a stream and
// returns this error next to whatever the caller had already received.
type streamStalledError struct {
	idle time.Duration
}

func (e *streamStalledError) Error() string {
	// The duration never puts a space after its digits, so the status-code
	// scan in httpStatusFromError cannot mistake it for a code.
	return "stream stalled: no data from the model for " + e.idle.String()
}

// IsStreamStalled reports whether err carries a mid-response stall, so
// callers (the ReAct loop) can keep the partial answer the same way they do
// for a truncation and decide whether the turn carries on.
func IsStreamStalled(err error) bool {
	var stalled *streamStalledError
	return errors.As(err, &stalled)
}

// StreamStalledIdle returns how long the stalled stream had been silent, or
// zero when err is not a stall.
func StreamStalledIdle(err error) time.Duration {
	var stalled *streamStalledError
	if errors.As(err, &stalled) {
		return stalled.idle
	}
	return 0
}
