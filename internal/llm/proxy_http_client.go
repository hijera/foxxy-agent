package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/net/http/httpproxy"
	xproxy "golang.org/x/net/proxy"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/netx"
)

const llmResponseHeaderTimeout = 30 * time.Second

// HTTP/2 health check of LLM connections. A pooled HTTP/2 connection is shared by
// every request to the provider, and behind a proxy it can die without a FIN: the
// proxy drops the tunnel's state, TCP to the proxy stays up, and nothing ever
// arrives again. Without a health check every later request - every retry of a
// silent model call included - is sent on that same connection and waits out the
// caller's whole timeout. With it, a connection that has received nothing for
// llmHTTP2SendPingTimeout is pinged, and closed when the ping is not answered
// within llmHTTP2PingTimeout, so the next request opens a new one. The server
// answers pings itself, so a model that is slow to write its answer does not
// trip it. Variables so tests can shorten them.
var (
	llmHTTP2SendPingTimeout = 15 * time.Second
	llmHTTP2PingTimeout     = 15 * time.Second
)

// A test seam for inherited routes, which net/http otherwise caches per
// process and exempts loopback from. Production always uses the netx resolver.
type proxyFunc = func(*http.Request) (*url.URL, error)

var environmentProxy atomic.Pointer[proxyFunc]

// HTTPClientForProviderProxy uses the shared provider transport for model
// listing, account usage and sign-in requests, just as for completions.
func HTTPClientForProviderProxy(setting string) (*http.Client, error) {
	t, route, err := providerTransport(setting)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: debugTransportFor(t, route)}, nil
}

// HTTPClientForOptionalProxy returns an HTTP client for the selected route.
// Supported schemes are http, https (HTTP proxy), socks5, and socks5h (SOCKS5 with remote DNS on socks5h).
//
// A configured proxy takes precedence over the process environment: it overrides HTTP_PROXY/HTTPS_PROXY,
// so a provider proxy always wins over a proxy inherited from the editor or shell. NO_PROXY is still
// honored, and loopback targets always bypass the proxy, so a local api_base (Ollama, LM Studio) keeps
// working when a proxy is configured.
//
// An empty proxyURL still gets a dedicated transport: it inherits HTTP_PROXY/HTTPS_PROXY from the
// process environment, but bounds the wait for response headers. No whole-request timeout is set,
// because streamed response bodies may legitimately remain open for a long time.
func HTTPClientForOptionalProxy(proxyURL string) (*http.Client, error) {
	t, route, err := buildLLMTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: debugTransportFor(t, route)}, nil
}

// buildLLMTransport builds a fresh LLM transport for the proxy setting, with the
// route the connection trace describes it by. The providers share one per
// setting (transport.go); the other callers get their own.
func buildLLMTransport(proxyURL string) (*http.Transport, routeFunc, error) {
	mode, u, err := config.ParseProxySetting(proxyURL)
	if err != nil {
		return nil, nil, err
	}
	switch mode {
	case config.ProxyModeInherit:
		return transportEnvironmentProxy()
	case config.ProxyModeNone:
		t, err := cloneLLMTransport()
		if err != nil {
			return nil, nil, err
		}
		t.Proxy = nil
		return t, func(*url.URL) netRoute { return netRoute{desc: "direct (proxy: none)"} }, nil
	}
	switch u.Scheme {
	case "http", "https":
		return transportHTTPProxy(u)
	case "socks5", "socks5h":
		return transportSOCKSProxy(u)
	default:
		return nil, nil, fmt.Errorf("unsupported proxy scheme %q (use http, https, socks5, or socks5h)", u.Scheme)
	}
}

// directBypassed is the route of a target a configured proxy lets through
// straight: loopback, or a host NO_PROXY names.
const directBypassed = "direct (proxy bypassed: loopback or NO_PROXY)"

// noProxyEnv returns the NO_PROXY exception list from the environment, matching the casing fallback
// net/http uses.
func noProxyEnv() string {
	if v := os.Getenv("NO_PROXY"); v != "" {
		return v
	}
	return os.Getenv("no_proxy")
}

// proxyFuncFor builds the proxy resolver for a configured proxy. It pins the proxy to u — ignoring
// HTTP_PROXY/HTTPS_PROXY so the configured proxy wins — while reusing httpproxy's rules for NO_PROXY
// and its built-in loopback exemption, which return a nil proxy (direct) for exempt targets.
func proxyFuncFor(u *url.URL) func(*url.URL) (*url.URL, error) {
	cfg := &httpproxy.Config{
		HTTPProxy:  u.String(),
		HTTPSProxy: u.String(),
		NoProxy:    noProxyEnv(),
	}
	return cfg.ProxyFunc()
}

func transportHTTPProxy(u *url.URL) (*http.Transport, routeFunc, error) {
	t, err := cloneLLMTransport()
	if err != nil {
		return nil, nil, err
	}
	proxyFor := proxyFuncFor(u)
	t.Proxy = func(req *http.Request) (*url.URL, error) { return proxyFor(req.URL) }
	return t, routeVia("proxy", directBypassed, proxyFor), nil
}

func transportSOCKSProxy(u *url.URL) (*http.Transport, routeFunc, error) {
	dialer, err := xproxy.FromURL(u, xproxy.Direct)
	if err != nil {
		return nil, nil, fmt.Errorf("socks proxy: %w", err)
	}
	t, err := cloneLLMTransport()
	if err != nil {
		return nil, nil, err
	}
	// A SOCKS proxy is applied by dialing through it, not via Transport.Proxy; clear the inherited
	// ProxyFromEnvironment so an ambient HTTP_PROXY cannot also be layered on top.
	t.Proxy = nil

	socksDial := traceSOCKSDial(u, func(ctx context.Context, network, address string) (net.Conn, error) {
		if xd, ok := dialer.(xproxy.ContextDialer); ok {
			return xd.DialContext(ctx, network, address)
		}
		return dialer.Dial(network, address)
	})
	proxyFor := proxyFuncFor(u)
	direct := t.DialContext
	if direct == nil {
		direct = (&net.Dialer{}).DialContext
	}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if bypassProxy(proxyFor, address) {
			return direct(ctx, network, address)
		}
		return socksDial(ctx, network, address)
	}
	return t, routeVia("socks", directBypassed, proxyFor), nil
}

func transportEnvironmentProxy() (*http.Transport, routeFunc, error) {
	t, err := cloneLLMTransport()
	if err != nil {
		return nil, nil, err
	}
	// HTTP_PROXY / HTTPS_PROXY when set, otherwise the operating system's proxy
	// (on Windows its manual proxy, PAC script or WPAD), which net/http never reads.
	resolve := netx.EnvironmentProxyResolver()
	t.Proxy = func(req *http.Request) (*url.URL, error) {
		if override := environmentProxy.Load(); override != nil {
			return (*override)(req)
		}
		r, err := resolve(req.URL)
		return r.Proxy, err
	}
	return t, environmentRoute(resolve), nil
}

// environmentRoute describes, for the connection trace, the route of a request no
// provider proxy is set for: which proxy the environment or the system chose, and
// why a request goes direct when a system proxy is configured.
func environmentRoute(resolve func(*url.URL) (netx.ProxyRoute, error)) routeFunc {
	return func(target *url.URL) netRoute {
		r, err := resolve(target)
		if err != nil {
			return netRoute{desc: "unresolved: " + err.Error()}
		}
		if r.Proxy == nil {
			if r.Note != "" {
				return netRoute{desc: "direct (system proxy: " + r.Note + ")"}
			}
			return netRoute{desc: "direct"}
		}
		kind := "env-proxy"
		switch r.Source {
		case "system":
			kind = "system-proxy"
		case "system-pac":
			kind = "system-pac"
		}
		desc := kind + " " + redactProxyURL(r.Proxy)
		if r.Source == "system" && r.Note != "" && r.Note != "static" {
			desc += " (" + r.Note + ")"
		}
		tunnel := target.Scheme == "https" && (r.Proxy.Scheme == "http" || r.Proxy.Scheme == "https")
		return netRoute{desc: desc, tunnel: tunnel}
	}
}

func cloneLLMTransport() (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is not *http.Transport")
	}
	t := base.Clone()
	t.ResponseHeaderTimeout = llmResponseHeaderTimeout
	// Only logs, and only while the trace is on: the proxy's answer to CONNECT is
	// the one step of a tunnelled request httptrace does not report.
	t.OnProxyConnectResponse = logProxyConnect
	t.HTTP2 = &http.HTTP2Config{
		SendPingTimeout: llmHTTP2SendPingTimeout,
		PingTimeout:     llmHTTP2PingTimeout,
	}
	return t, nil
}

// bypassProxy reports whether address (host:port) is exempt from the proxy — loopback, or matched by
// NO_PROXY. Keep the port so rules such as api.example.com:443 match the actual dial and the route trace.
// Resolved through the same httpproxy rules used for HTTP proxies, which signal "direct" with a nil proxy.
func bypassProxy(proxyFor func(*url.URL) (*url.URL, error), address string) bool {
	p, err := proxyFor(&url.URL{Scheme: "http", Host: address})
	return err == nil && p == nil
}
