package netx

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/platform"
)

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
}

func resolveOnce(t *testing.T, raw string) ProxyRoute {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	r, err := EnvironmentProxyResolver()(u)
	if err != nil {
		t.Fatalf("resolve %s: %v", raw, err)
	}
	return r
}

func routeString(r ProxyRoute) string {
	if r.Proxy == nil {
		return "direct"
	}
	return r.Source + " " + r.Proxy.String()
}

// With no proxy in the environment the system proxy decides, and NO_PROXY and
// loopback still go direct; a proxy named in the environment always wins, which
// is what keeps the IDE plugins (they pass the IDE's proxy that way) unchanged.
func TestEnvironmentProxyFallsBackToTheSystemProxy(t *testing.T) {
	clearProxyEnv(t)
	defer SetSystemProxyForTesting(platform.NewSystemProxy(platform.SystemProxyConfig{
		Proxy:  "sys.corp:3128",
		Bypass: "*.intranet.corp",
	}))()

	cases := []struct{ target, want string }{
		{"https://api.openai.com/v1", "system http://sys.corp:3128"},
		{"https://llm.intranet.corp/", "direct"},
		{"http://127.0.0.1:11434/", "direct"},
		{"http://localhost:1234/", "direct"},
	}
	for _, c := range cases {
		if got := routeString(resolveOnce(t, c.target)); got != c.want {
			t.Errorf("%s: %s, want %s", c.target, got, c.want)
		}
	}
	if r := resolveOnce(t, "https://llm.intranet.corp/"); r.Note != "bypass" {
		t.Errorf("bypass note = %q", r.Note)
	}

	t.Setenv("NO_PROXY", "api.openai.com")
	if got := routeString(resolveOnce(t, "https://api.openai.com/v1")); got != "direct" {
		t.Errorf("NO_PROXY ignored under the system proxy: %s", got)
	}
	t.Setenv("NO_PROXY", "")

	t.Setenv("HTTPS_PROXY", "http://env.corp:8080")
	if got := routeString(resolveOnce(t, "https://api.openai.com/v1")); got != "env http://env.corp:8080" {
		t.Errorf("an explicit HTTPS_PROXY did not win: %s", got)
	}
}

func TestEnvironmentProxyWithoutASystemProxy(t *testing.T) {
	clearProxyEnv(t)
	defer SetSystemProxyForTesting(nil)()
	if got := routeString(resolveOnce(t, "https://api.openai.com/v1")); got != "direct" {
		t.Fatalf("no proxy anywhere = %s, want direct", got)
	}
	t.Setenv("HTTPS_PROXY", "http://env.corp:8080")
	if got := routeString(resolveOnce(t, "https://api.openai.com/v1")); got != "env http://env.corp:8080" {
		t.Fatalf("env proxy = %s", got)
	}
}

// InstallSystemProxy reaches http.DefaultTransport, so clients built on it (MCP,
// skills, web tools) follow the system proxy too - and leaves it alone when the
// environment names a proxy or there is no system proxy.
func TestInstallSystemProxy(t *testing.T) {
	clearProxyEnv(t)
	dt := http.DefaultTransport.(*http.Transport)
	prev := dt.Proxy
	t.Cleanup(func() { dt.Proxy = prev })

	defer SetSystemProxyForTesting(nil)()
	if got := InstallSystemProxy(); got != "" {
		t.Fatalf("installed %q without a system proxy", got)
	}

	restore := SetSystemProxyForTesting(platform.NewSystemProxy(platform.SystemProxyConfig{Proxy: "sys.corp:3128"}))
	defer restore()
	t.Setenv("HTTPS_PROXY", "http://env.corp:8080")
	if got := InstallSystemProxy(); got != "" {
		t.Fatalf("installed %q over an explicit HTTPS_PROXY", got)
	}
	t.Setenv("HTTPS_PROXY", "")
	if got := InstallSystemProxy(); got == "" {
		t.Fatal("the system proxy was not installed")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	p, err := dt.Proxy(req)
	if err != nil || p == nil || p.Host != "sys.corp:3128" {
		t.Fatalf("DefaultTransport proxy = %v, %v", p, err)
	}
}
