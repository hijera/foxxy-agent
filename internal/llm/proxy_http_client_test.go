package llm

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// proxyForRequest resolves the proxy the client's transport would use for rawURL.
func proxyForRequest(t *testing.T, c *http.Client, rawURL string) *url.URL {
	t.Helper()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is not *http.Transport")
	}
	if tr.Proxy == nil {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	p, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("proxy func: %v", err)
	}
	return p
}

// The configured provider proxy must win over an inherited environment proxy (the editor forwards its
// own proxy as HTTP_PROXY/HTTPS_PROXY). Regression: an empty value inherits the env proxy instead.
func TestProviderProxyOverridesEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://ide-proxy.local:8080")
	t.Setenv("HTTPS_PROXY", "http://ide-proxy.local:8080")
	t.Setenv("NO_PROXY", "")

	t.Run("http proxy wins over env", func(t *testing.T) {
		c, err := HTTPClientForOptionalProxy("http://provider-proxy.local:3128")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		got := proxyForRequest(t, c, "https://api.openai.com/v1/models")
		if got == nil || got.Host != "provider-proxy.local:3128" {
			t.Fatalf("expected the provider proxy, got %v", got)
		}
	})

	t.Run("socks proxy clears the env proxy", func(t *testing.T) {
		c, err := HTTPClientForOptionalProxy("socks5://provider-proxy.local:1080")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		tr := c.Transport.(*http.Transport)
		if tr.Proxy != nil {
			t.Fatal("SOCKS transport must not carry an HTTP proxy from the environment")
		}
		if tr.DialContext == nil {
			t.Fatal("SOCKS transport must dial through the proxy")
		}
	})

	// Empty means "inherit the environment (editor) proxy": a nil client leaves the SDK on
	// http.DefaultTransport, whose ProxyFromEnvironment reads HTTP_PROXY/HTTPS_PROXY.
	t.Run("empty falls back to the env proxy", func(t *testing.T) {
		c, err := HTTPClientForOptionalProxy("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if c != nil {
			t.Fatalf("expected nil client so the env proxy applies, got %v", c)
		}
	})
}

// A configured provider proxy must not swallow traffic that should stay direct: loopback (a local
// api_base such as Ollama) and hosts listed in NO_PROXY.
func TestProviderProxyBypassesLoopbackAndNoProxy(t *testing.T) {
	t.Setenv("NO_PROXY", "internal.corp")

	c, err := HTTPClientForOptionalProxy("http://provider-proxy.local:3128")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for _, tc := range []struct {
		name       string
		url        string
		wantDirect bool
	}{
		{"loopback ip", "http://127.0.0.1:11434/v1/chat/completions", true},
		{"localhost", "http://localhost:1234/v1/models", true},
		{"no_proxy host", "https://internal.corp/v1/models", true},
		{"external host", "https://api.openai.com/v1/models", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := proxyForRequest(t, c, tc.url)
			if tc.wantDirect && got != nil {
				t.Fatalf("expected a direct connection, got proxy %v", got)
			}
			if !tc.wantDirect && got == nil {
				t.Fatal("expected the provider proxy, got a direct connection")
			}
		})
	}
}

func TestHTTPClientForOptionalProxy(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		c, err := HTTPClientForOptionalProxy("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if c != nil {
			t.Fatalf("expected nil client")
		}
	})
	t.Run("whitespace", func(t *testing.T) {
		t.Parallel()
		c, err := HTTPClientForOptionalProxy("  \t  ")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if c != nil {
			t.Fatalf("expected nil client")
		}
	})
	t.Run("bad_url", func(t *testing.T) {
		t.Parallel()
		_, err := HTTPClientForOptionalProxy("http://%zz")
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("bad_scheme", func(t *testing.T) {
		t.Parallel()
		_, err := HTTPClientForOptionalProxy("ftp://127.0.0.1:21")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unsupported proxy scheme") {
			t.Fatalf("unexpected err: %v", err)
		}
	})
	t.Run("http_ok", func(t *testing.T) {
		t.Parallel()
		c, err := HTTPClientForOptionalProxy("http://127.0.0.1:3128")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if c == nil || c.Transport == nil {
			t.Fatal("expected non-nil client and transport")
		}
	})
	t.Run("socks5_ok", func(t *testing.T) {
		t.Parallel()
		c, err := HTTPClientForOptionalProxy("socks5://127.0.0.1:1080")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if c == nil || c.Transport == nil {
			t.Fatal("expected non-nil client and transport")
		}
	})
}
