package llm

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/netx/proxytest"
)

// Passwords an operator actually has: an @ (the reported one), a colon, the
// characters that cut a URL short, a non-Latin one and a space. Each is written
// the way the proxy editor in Settings writes it - percent-encoded - and must reach
// the proxy decoded, byte for byte.
var proxyPasswords = []string{"p@ss", "a:b", "/?#%", "пароль", "sp ace"}

func proxyURLWith(scheme, addr, user, password string) string {
	return (&url.URL{Scheme: scheme, User: url.UserPassword(user, password), Host: addr}).String()
}

func getThrough(t *testing.T, c *http.Client, target string) (int, error) {
	t.Helper()
	resp, err := c.Get(target)
	if err != nil {
		return 0, err
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func TestProviderProxyAuthenticatesWithAnyPassword(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer secure.Close()
	plainTarget := "http://llm.test:" + targetPort(t, plain) + "/v1/models"
	secureTarget := "https://llm.test:" + targetPort(t, secure) + "/v1/models"

	for _, password := range proxyPasswords {
		t.Run("http "+password, func(t *testing.T) {
			p := proxytest.HTTP(t, "vlasov", password)
			c, err := HTTPClientForOptionalProxy(proxyURLWith("http", p.Addr, "vlasov", password))
			if err != nil {
				t.Fatalf("client: %v", err)
			}
			if code, err := getThrough(t, c, plainTarget); err != nil || code != http.StatusOK {
				t.Fatalf("plain request: code %d err %v; proxy saw %+v", code, err, p.Offered())
			}
			UnwrapTransport(c.Transport).(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test server certificate
			if code, err := getThrough(t, c, secureTarget); err != nil || code != http.StatusOK {
				t.Fatalf("tunnelled request: code %d err %v; proxy saw %+v", code, err, p.Offered())
			}
			if p.Used() != 2 {
				t.Fatalf("proxy carried %d requests, want 2", p.Used())
			}
		})
		t.Run("socks5 "+password, func(t *testing.T) {
			p := proxytest.SOCKS5(t, "vlasov", password)
			c, err := HTTPClientForOptionalProxy(proxyURLWith("socks5h", p.Addr, "vlasov", password))
			if err != nil {
				t.Fatalf("client: %v", err)
			}
			if code, err := getThrough(t, c, plainTarget); err != nil || code != http.StatusOK {
				t.Fatalf("request: code %d err %v; proxy saw %+v", code, err, p.Offered())
			}
		})
	}
}

// The reported form: the password typed with a raw @, not encoded. It parses (the
// authority is split at the last @) and authenticates as typed.
func TestProviderProxyRawAtInPassword(t *testing.T) {
	t.Setenv("NO_PROXY", "")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()

	p := proxytest.HTTP(t, "vlasov", "p@ss")
	c, err := HTTPClientForOptionalProxy("http://vlasov:p@ss@" + p.Addr)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if code, err := getThrough(t, c, "http://llm.test:"+targetPort(t, target)+"/"); err != nil || code != http.StatusOK {
		t.Fatalf("code %d err %v; proxy saw %+v", code, err, p.Offered())
	}
}

// A wrong password is the proxy's refusal, visible as such, and the password is in
// neither the error nor the trace.
func TestProviderProxyWrongPassword(t *testing.T) {
	buf := withNetTrace(t, time.Minute)
	t.Setenv("NO_PROXY", "")
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer target.Close()

	p := proxytest.HTTP(t, "vlasov", "right")
	c, err := HTTPClientForOptionalProxy(proxyURLWith("http", p.Addr, "vlasov", "Wr0ngPw"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = getThrough(t, c, "https://llm.test:"+targetPort(t, target)+"/")
	if err == nil {
		t.Fatal("expected the tunnel to be refused")
	}
	if !strings.Contains(err.Error(), "Proxy Authentication Required") {
		t.Errorf("error does not name the refusal: %v", err)
	}
	if strings.Contains(err.Error(), "Wr0ngPw") {
		t.Errorf("error leaks the password: %v", err)
	}
	logs := waitForLog(t, buf, `status="407 Proxy Authentication Required"`)
	if strings.Contains(logs, "Wr0ngPw") {
		t.Errorf("trace leaks the password:\n%s", logs)
	}
}
