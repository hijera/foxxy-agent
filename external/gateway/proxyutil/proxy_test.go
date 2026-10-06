//go:build gateway || gateway.telegram

package proxyutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/netx/proxytest"
)

func botAPI(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	// A name that is not loopback, so nothing is tempted to bypass the proxy.
	return "http://api.telegram.test:" + u.Port() + "/bot/getMe"
}

func get(t *testing.T, c *http.Client, target string) int {
	t.Helper()
	resp, err := c.Get(target)
	if err != nil {
		t.Fatalf("GET through the proxy: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestProxyModesSelectTransport(t *testing.T) {
	for _, setting := range []string{"  ", "Inherit", "none"} {
		c, err := BuildHTTPClient(setting)
		if err != nil || c == nil || c.Transport == nil {
			t.Fatalf("BuildHTTPClient(%q) = %v, %v; want a configured transport", setting, c, err)
		}
	}
	direct, _ := BuildHTTPClient("none")
	if direct.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("none must not consult a proxy resolver")
	}
}

// The Bot API is reached through an authenticating proxy whatever the password
// carries, written the way the proxy editor in Settings writes it.
func TestBotAPIThroughAnAuthenticatingProxy(t *testing.T) {
	target := botAPI(t)
	for _, password := range []string{"p@ss", "/?#%", "пароль"} {
		creds := url.UserPassword("bot", password).String()
		t.Run("http "+password, func(t *testing.T) {
			p := proxytest.HTTP(t, "bot", password)
			c, err := BuildHTTPClient("http://" + creds + "@" + p.Addr)
			if err != nil {
				t.Fatal(err)
			}
			if code := get(t, c, target); code != http.StatusOK || p.Used() != 1 {
				t.Fatalf("code %d, proxy carried %d, saw %+v", code, p.Used(), p.Offered())
			}
		})
		t.Run("socks5 "+password, func(t *testing.T) {
			p := proxytest.SOCKS5(t, "bot", password)
			c, err := BuildHTTPClient("socks5h://" + creds + "@" + p.Addr)
			if err != nil {
				t.Fatal(err)
			}
			if code := get(t, c, target); code != http.StatusOK || p.Used() != 1 {
				t.Fatalf("code %d, proxy carried %d, saw %+v", code, p.Used(), p.Offered())
			}
		})
	}
}

func TestBrokenProxyURLIsRefusedWithoutThePassword(t *testing.T) {
	for _, raw := range []string{
		"http://bot:S3cr/x@proxy:3128",
		"ftp://bot:S3cr@proxy:21",
	} {
		_, err := BuildHTTPClient(raw)
		if err == nil {
			t.Errorf("BuildHTTPClient(%q) accepted it", raw)
			continue
		}
		if strings.Contains(err.Error(), "S3cr") {
			t.Errorf("error leaks the password: %v", err)
		}
	}
}
