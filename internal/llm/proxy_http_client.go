package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/http/httpproxy"
	xproxy "golang.org/x/net/proxy"
)

// HTTPClientForOptionalProxy returns an HTTP client that sends traffic through the given proxy URL.
// Supported schemes are http, https (HTTP proxy), socks5, and socks5h (SOCKS5 with remote DNS on socks5h).
//
// A configured proxy takes precedence over the process environment: it overrides HTTP_PROXY/HTTPS_PROXY,
// so a provider proxy always wins over a proxy inherited from the editor or shell. NO_PROXY is still
// honored, and loopback targets always bypass the proxy, so a local api_base (Ollama, LM Studio) keeps
// working when a proxy is configured.
//
// For an empty proxyURL it returns nil, nil so callers keep the SDK default client. That client uses
// http.DefaultTransport, whose ProxyFromEnvironment picks up HTTP_PROXY/HTTPS_PROXY — so an empty value
// means "inherit the environment (i.e. the editor's) proxy, else connect directly", not "force direct".
func HTTPClientForOptionalProxy(proxyURL string) (*http.Client, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return nil, nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("proxy url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https":
		t, err := transportHTTPProxy(u)
		if err != nil {
			return nil, err
		}
		return &http.Client{Transport: t}, nil
	case "socks5", "socks5h":
		t, err := transportSOCKSProxy(u)
		if err != nil {
			return nil, err
		}
		return &http.Client{Transport: t}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (use http, https, socks5, or socks5h)", u.Scheme)
	}
}

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

func transportHTTPProxy(u *url.URL) (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is not *http.Transport")
	}
	t := base.Clone()
	proxyFor := proxyFuncFor(u)
	t.Proxy = func(req *http.Request) (*url.URL, error) { return proxyFor(req.URL) }
	return t, nil
}

func transportSOCKSProxy(u *url.URL) (*http.Transport, error) {
	dialer, err := xproxy.FromURL(u, xproxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("socks proxy: %w", err)
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport is not *http.Transport")
	}
	t := base.Clone()
	// A SOCKS proxy is applied by dialing through it, not via Transport.Proxy; clear the inherited
	// ProxyFromEnvironment so an ambient HTTP_PROXY cannot also be layered on top.
	t.Proxy = nil

	socksDial := func(ctx context.Context, network, address string) (net.Conn, error) {
		if xd, ok := dialer.(xproxy.ContextDialer); ok {
			return xd.DialContext(ctx, network, address)
		}
		return dialer.Dial(network, address)
	}
	proxyFor := proxyFuncFor(u)
	direct := base.DialContext
	if direct == nil {
		direct = (&net.Dialer{}).DialContext
	}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if bypassProxy(proxyFor, address) {
			return direct(ctx, network, address)
		}
		return socksDial(ctx, network, address)
	}
	return t, nil
}

// bypassProxy reports whether address (host:port) is exempt from the proxy — loopback, or matched by
// NO_PROXY. Resolved through the same httpproxy rules used for HTTP proxies, which signal "direct" by
// returning a nil proxy.
func bypassProxy(proxyFor func(*url.URL) (*url.URL, error), address string) bool {
	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}
	p, err := proxyFor(&url.URL{Scheme: "http", Host: host})
	return err == nil && p == nil
}
