package platform

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func proxyString(u *url.URL) string {
	if u == nil {
		return "direct"
	}
	return u.String()
}

// The manual proxy as Windows writes it: one proxy for everything, or one per
// scheme with socks= for the rest, and the exception list with <local> and *.
func TestSystemProxyStatic(t *testing.T) {
	cases := []struct {
		name, proxy, bypass, target, want, via string
	}{
		{"one for all", "proxy.corp:3128", "", "https://api.openai.com/v1", "http://proxy.corp:3128", "static"},
		{"one for all, http", "proxy.corp:3128", "", "http://example.com/", "http://proxy.corp:3128", "static"},
		{"per scheme https", "http=h1:80;https=h2:443", "", "https://api.openai.com/", "http://h2:443", "static"},
		{"per scheme http", "http=h1:80;https=h2:443", "", "http://example.com/", "http://h1:80", "static"},
		{"socks for the rest", "http=h1:80;socks=s:1080", "", "https://api.openai.com/", "socks5://s:1080", "static"},
		{"unnamed scheme goes direct", "http=h1:80", "", "https://api.openai.com/", "direct", "static"},
		{"scheme in the value", "https=http://h2:8080", "", "https://api.openai.com/", "http://h2:8080", "static"},
		{"loopback never proxied", "proxy.corp:3128", "", "http://127.0.0.1:11434/", "direct", "loopback"},
		{"localhost never proxied", "proxy.corp:3128", "", "http://localhost:8080/", "direct", "loopback"},
		{"<local> plain name", "proxy.corp:3128", "<local>", "http://intranet/", "direct", "bypass"},
		{"<local> dotted name proxied", "proxy.corp:3128", "<local>", "https://api.openai.com/", "http://proxy.corp:3128", "static"},
		{"wildcard domain", "proxy.corp:3128", "*.corp.example;10.*", "https://llm.corp.example/", "direct", "bypass"},
		{"wildcard ip", "proxy.corp:3128", "*.corp.example;10.*", "http://10.1.2.3:8000/", "direct", "bypass"},
		{"exact host with scheme", "proxy.corp:3128", "https://api.local.test", "https://api.local.test/", "direct", "bypass"},
		{"case", "proxy.corp:3128", "*.CORP.example", "https://LLM.corp.example/", "direct", "bypass"},
	}
	for _, c := range cases {
		s := NewSystemProxy(SystemProxyConfig{Proxy: c.proxy, Bypass: c.bypass})
		r := s.Resolve(mustURL(t, c.target))
		if got := proxyString(r.Proxy); got != c.want || r.Via != c.via {
			t.Errorf("%s: Resolve(%s) = %s via %q, want %s via %q", c.name, c.target, got, r.Via, c.want, c.via)
		}
	}
}

// A PAC answer is used as the script gave it, cached per host, and a script that
// cannot be evaluated falls back to the manual proxy - or to a direct connection,
// as a browser does - instead of failing the request.
func TestSystemProxyPAC(t *testing.T) {
	var asked []string
	answer, fail := "pac.corp:8080;backup.corp:8080", error(nil)
	prev := pacLookup
	pacLookup = func(target string, cfg SystemProxyConfig) (string, error) {
		asked = append(asked, target)
		return answer, fail
	}
	t.Cleanup(func() { pacLookup = prev })

	s := NewSystemProxy(SystemProxyConfig{AutoConfigURL: "http://wpad.corp/proxy.pac", Proxy: "static.corp:3128"})
	now := time.Unix(1000, 0)
	s.now = func() time.Time { return now }

	r := s.Resolve(mustURL(t, "https://user:pw@api.openai.com/v1/chat?x=1"))
	if proxyString(r.Proxy) != "http://pac.corp:8080" || r.Via != "pac" {
		t.Fatalf("pac route = %s via %q", proxyString(r.Proxy), r.Via)
	}
	if len(asked) != 1 || asked[0] != "https://api.openai.com/" {
		t.Fatalf("script asked about %q, want the bare origin once", asked)
	}
	s.Resolve(mustURL(t, "https://api.openai.com/v1/models"))
	if len(asked) != 1 {
		t.Fatalf("the answer for the host was not reused: %q", asked)
	}
	now = now.Add(pacTTL + time.Second)
	s.Resolve(mustURL(t, "https://api.openai.com/v1/models"))
	if len(asked) != 2 {
		t.Fatalf("an expired answer was reused: %q", asked)
	}

	answer = ""
	r = s.Resolve(mustURL(t, "https://direct.example/"))
	if r.Proxy != nil || r.Via != "pac" {
		t.Fatalf("DIRECT from the script = %s via %q", proxyString(r.Proxy), r.Via)
	}

	fail = errors.New("script download failed")
	r = s.Resolve(mustURL(t, "https://other.example/"))
	if proxyString(r.Proxy) != "http://static.corp:3128" || !strings.HasPrefix(r.Via, "pac failed: script download failed") {
		t.Fatalf("failed pac = %s via %q, want the manual proxy", proxyString(r.Proxy), r.Via)
	}
	noStatic := NewSystemProxy(SystemProxyConfig{AutoDetect: true})
	if r := noStatic.Resolve(mustURL(t, "https://other.example/")); r.Proxy != nil {
		t.Fatalf("failed auto-detect without a manual proxy = %s, want direct", proxyString(r.Proxy))
	}
	if r := s.Resolve(mustURL(t, "http://127.0.0.1:1234/")); r.Via != "loopback" {
		t.Fatalf("loopback asked the script: %+v", r)
	}
}

func TestFirstProxyOfAPACAnswer(t *testing.T) {
	cases := map[string]string{
		"":                   "direct",
		"a:1":                "http://a:1",
		"a:1; b:2":           "http://a:1",
		"PROXY a:1; DIRECT":  "http://a:1",
		"SOCKS5 s:1080":      "socks5://s:1080",
		"HTTPS h:443":        "https://h:443",
		"DIRECT":             "direct",
		"http=a:1;https=b:2": "http://b:2",
	}
	for list, want := range cases {
		if got := proxyString(firstProxy(list, "https")); got != want {
			t.Errorf("firstProxy(%q) = %s, want %s", list, got, want)
		}
	}
}

func TestLoadSystemProxy(t *testing.T) {
	prev := readSystemProxyConfig
	t.Cleanup(func() { readSystemProxyConfig = prev })
	cfg := SystemProxyConfig{Proxy: "user:secret@proxy.corp:3128", Bypass: "<local>"}
	readSystemProxyConfig = func() (SystemProxyConfig, error) { return cfg, nil }

	t.Setenv(SystemProxyEnvVar, "")
	s := LoadSystemProxy()
	if s == nil {
		t.Fatal("a configured system proxy was not loaded")
	}
	if d := s.Describe(); strings.Contains(d, "secret") || !strings.Contains(d, "proxy.corp:3128") {
		t.Fatalf("Describe() = %q", d)
	}
	t.Setenv(SystemProxyEnvVar, "off")
	if LoadSystemProxy() != nil {
		t.Fatal("FOXXYCODE_SYSTEM_PROXY=off did not turn it off")
	}
	t.Setenv(SystemProxyEnvVar, "")
	cfg = SystemProxyConfig{}
	if LoadSystemProxy() != nil {
		t.Fatal("an empty configuration produced a resolver")
	}
	readSystemProxyConfig = func() (SystemProxyConfig, error) { return SystemProxyConfig{}, errors.New("boom") }
	if LoadSystemProxy() != nil {
		t.Fatal("an unreadable configuration produced a resolver")
	}
}
