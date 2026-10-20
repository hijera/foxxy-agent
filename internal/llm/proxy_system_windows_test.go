//go:build windows

package llm

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/netx"
	"github.com/hijera/foxxycode-agent/internal/netx/proxytest"
	"github.com/hijera/foxxycode-agent/internal/platform"
)

// The corporate case end to end: Windows is set to "Use setup script", the PAC
// file sends the model's host through the proxy and everything else direct, and
// Windows itself runs the script. The provider request reaches the proxy.
func TestProviderFollowsAWindowsPACScript(t *testing.T) {
	buf := withNetTrace(t, time.Minute)
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()
	p := proxytest.HTTP(t, "", "")

	script := fmt.Sprintf(`function FindProxyForURL(url, host) {
  if (host == "llm.test") return "PROXY %s";
  return "DIRECT";
}`, p.Addr)
	pac := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		_, _ = io.WriteString(w, script)
	}))
	defer pac.Close()
	defer netx.SetSystemProxyForTesting(platform.NewSystemProxy(platform.SystemProxyConfig{
		AutoConfigURL: pac.URL + "/proxy.pac",
	}))()

	c, err := HTTPClientForOptionalProxy("")
	if err != nil {
		t.Fatal(err)
	}
	if code, err := getThrough(t, c, "http://llm.test:"+targetPort(t, target)+"/v1/models"); err != nil || code != http.StatusOK {
		t.Fatalf("code %d err %v", code, err)
	}
	if p.Used() != 1 {
		t.Fatalf("the PAC proxy carried %d requests, want 1", p.Used())
	}
	waitForLog(t, buf, `route="system-pac http://`+p.Addr+`"`)
}
