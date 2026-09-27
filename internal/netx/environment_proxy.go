package netx

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/net/http/httpproxy"

	"github.com/hijera/foxxycode-agent/internal/platform"
)

// The proxy a request takes when nothing names one for it: HTTP_PROXY /
// HTTPS_PROXY when the environment sets them, and otherwise the operating
// system's own setting - on Windows the proxy, the PAC script or the WPAD
// discovery the Proxy settings page holds, which Go's net/http never reads.
//
// The IDE plugins pass their proxy in the environment, so under a plugin nothing
// changes; the desktop app and VS Code with an empty http.proxy have no such
// parent, and without this their requests went direct on a network that only
// lets a proxy out.

// ProxyRoute is where the environment sends one request.
type ProxyRoute struct {
	// Proxy is nil for a direct connection.
	Proxy *url.URL
	// Source is "env" (HTTP_PROXY / HTTPS_PROXY), "system" (the operating
	// system's manual proxy) or "system-pac" (its PAC script or WPAD); empty for
	// a direct connection.
	Source string
	// Note explains a system-proxy decision: "bypass", "loopback", "pac failed: …".
	Note string
}

var (
	systemProxyMu   sync.Mutex
	systemProxyOnce = sync.OnceValue(platform.LoadSystemProxy)
	systemProxyOver *platform.SystemProxy
	systemProxySet  bool
)

func systemProxy() *platform.SystemProxy {
	systemProxyMu.Lock()
	defer systemProxyMu.Unlock()
	if systemProxySet {
		return systemProxyOver
	}
	return systemProxyOnce()
}

// SetSystemProxyForTesting replaces the system proxy the resolver falls back to
// (nil for none) and returns the function that restores it.
func SetSystemProxyForTesting(sp *platform.SystemProxy) (restore func()) {
	systemProxyMu.Lock()
	prevSet, prev := systemProxySet, systemProxyOver
	systemProxySet, systemProxyOver = true, sp
	systemProxyMu.Unlock()
	return func() {
		systemProxyMu.Lock()
		systemProxySet, systemProxyOver = prevSet, prev
		systemProxyMu.Unlock()
	}
}

// environmentNamesProxy reports whether the environment names a proxy, the same
// variables httpproxy reads.
func environmentNamesProxy() bool {
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// EnvironmentProxyKey names what EnvironmentProxyResolver reads - the proxy
// variables, NO_PROXY and the system proxy in effect - so a caller that keeps
// what it built from them can tell when that no longer holds.
func EnvironmentProxyKey() string {
	var b strings.Builder
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"} {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(os.Getenv(k))
		b.WriteByte(0)
	}
	fmt.Fprintf(&b, "system=%p", systemProxy())
	return b.String()
}

// EnvironmentProxyResolver returns the route resolver for requests nothing else
// names a proxy for, as the environment stands now. Explicit proxy variables win;
// otherwise the system proxy decides, with NO_PROXY and loopback still going
// direct.
func EnvironmentProxyResolver() func(target *url.URL) (ProxyRoute, error) {
	if sp := systemProxy(); sp != nil && !environmentNamesProxy() {
		// Any proxy value would do: the function only answers whether NO_PROXY or
		// the loopback rule exempts the target.
		exempt := (&httpproxy.Config{
			HTTPProxy:  "http://exempt.invalid",
			HTTPSProxy: "http://exempt.invalid",
			NoProxy:    noProxyFromEnv(),
		}).ProxyFunc()
		return func(target *url.URL) (ProxyRoute, error) {
			if p, err := exempt(target); err == nil && p == nil {
				return ProxyRoute{Note: "NO_PROXY or loopback"}, nil
			}
			r := sp.Resolve(target)
			route := ProxyRoute{Proxy: r.Proxy, Note: r.Via}
			if r.Proxy != nil {
				route.Source = "system"
				if r.Via == "pac" {
					route.Source = "system-pac"
				}
			}
			return route, nil
		}
	}
	env := httpproxy.FromEnvironment().ProxyFunc()
	return func(target *url.URL) (ProxyRoute, error) {
		p, err := env(target)
		if p == nil {
			return ProxyRoute{}, err
		}
		return ProxyRoute{Proxy: p, Source: "env"}, err
	}
}

// EnvironmentProxyFunc is EnvironmentProxyResolver shaped for http.Transport.Proxy.
func EnvironmentProxyFunc() func(*http.Request) (*url.URL, error) {
	resolve := EnvironmentProxyResolver()
	return func(req *http.Request) (*url.URL, error) {
		r, err := resolve(req.URL)
		return r.Proxy, err
	}
}

func noProxyFromEnv() string {
	if v := os.Getenv("NO_PROXY"); v != "" {
		return v
	}
	return os.Getenv("no_proxy")
}

// InstallSystemProxy points http.DefaultTransport at the system proxy when the
// environment names none, so http.DefaultClient and every transport cloned from
// the default one - MCP servers, skill downloads, web tools - follow it too. It
// reports the system proxy it installed, "" when it changed nothing. Call it once
// at startup, before the first request.
func InstallSystemProxy() string {
	sp := systemProxy()
	if sp == nil || environmentNamesProxy() {
		return ""
	}
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return ""
	}
	t.Proxy = EnvironmentProxyFunc()
	desc := sp.Describe()
	installedSystemProxy.Store(&desc)
	return desc
}

var installedSystemProxy atomic.Pointer[string]

// LogSystemProxy records in log the system proxy InstallSystemProxy put in
// place, if any. The entry points call it once their logger exists: the proxy
// is installed before it, ahead of the first request.
func LogSystemProxy(log *slog.Logger) {
	if log == nil {
		return
	}
	if d := installedSystemProxy.Load(); d != nil {
		log.Info("using the system proxy", "system_proxy", *d,
			"hint", "set HTTPS_PROXY, a provider proxy, or "+platform.SystemProxyEnvVar+"=off to override")
	}
}
