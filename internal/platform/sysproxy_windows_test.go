//go:build windows

package platform

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Reading the current user's settings works on any Windows, whatever they hold.
func TestReadSystemProxyConfigOnWindows(t *testing.T) {
	if _, err := readSystemProxyConfigOS(); err != nil {
		t.Fatalf("readSystemProxyConfigOS: %v", err)
	}
}

// A real PAC script, served over HTTP and run by WinHTTP: the proxy it names for
// one host, DIRECT for another. This is the path a corporate "Use setup script"
// takes.
func TestPACIsEvaluatedByWindows(t *testing.T) {
	const script = `function FindProxyForURL(url, host) {
  if (dnsDomainIs(host, ".llm.test") || host == "llm.test") return "PROXY 127.0.0.1:3128; DIRECT";
  return "DIRECT";
}`
	pac := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		_, _ = io.WriteString(w, script)
	}))
	defer pac.Close()

	s := NewSystemProxy(SystemProxyConfig{AutoConfigURL: pac.URL + "/proxy.pac"})
	r := s.Resolve(mustURL(t, "https://llm.test/v1/chat"))
	if r.Via != "pac" || proxyString(r.Proxy) != "http://127.0.0.1:3128" {
		t.Fatalf("llm.test = %s via %q, want the script's proxy", proxyString(r.Proxy), r.Via)
	}
	r = s.Resolve(mustURL(t, "https://elsewhere.test/"))
	if r.Via != "pac" || r.Proxy != nil {
		t.Fatalf("elsewhere.test = %s via %q, want DIRECT from the script", proxyString(r.Proxy), r.Via)
	}
}

// A PAC address that serves nothing fails the lookup, and the request falls back
// instead of failing.
func TestUnreachablePACFallsBack(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL + "/proxy.pac"
	dead.Close()

	s := NewSystemProxy(SystemProxyConfig{AutoConfigURL: url, Proxy: "static.corp:3128"})
	r := s.Resolve(mustURL(t, "https://llm.test/"))
	if proxyString(r.Proxy) != "http://static.corp:3128" || r.Via == "pac" {
		t.Fatalf("unreachable pac = %s via %q, want the manual proxy", proxyString(r.Proxy), r.Via)
	}
}
