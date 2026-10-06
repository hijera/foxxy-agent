package llm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/netx"
	"github.com/hijera/foxxycode-agent/internal/netx/proxytest"
	"github.com/hijera/foxxycode-agent/internal/platform"
)

// A provider with no proxy of its own, on a machine whose only way out is the
// system proxy - the desktop app, or VS Code with an empty http.proxy. The request
// goes through it, and the trace says so.
func TestProviderWithoutAProxyFollowsTheSystemProxy(t *testing.T) {
	buf := withNetTrace(t, time.Minute)
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()

	p := proxytest.HTTP(t, "", "")
	defer netx.SetSystemProxyForTesting(platform.NewSystemProxy(platform.SystemProxyConfig{Proxy: p.Addr}))()

	c, err := HTTPClientForOptionalProxy("")
	if err != nil {
		t.Fatal(err)
	}
	if code, err := getThrough(t, c, "http://llm.test:"+targetPort(t, target)+"/v1/models"); err != nil || code != http.StatusOK {
		t.Fatalf("code %d err %v", code, err)
	}
	if p.Used() != 1 {
		t.Fatalf("the system proxy carried %d requests, want 1", p.Used())
	}
	waitForLog(t, buf, `route="system-proxy http://`+p.Addr+`"`)

	// A provider proxy still wins over the system one.
	own := proxytest.HTTP(t, "", "")
	c, err = HTTPClientForOptionalProxy("http://" + own.Addr)
	if err != nil {
		t.Fatal(err)
	}
	if code, err := getThrough(t, c, "http://llm.test:"+targetPort(t, target)+"/"); err != nil || code != http.StatusOK {
		t.Fatalf("code %d err %v", code, err)
	}
	if own.Used() != 1 || p.Used() != 1 {
		t.Fatalf("provider proxy carried %d, system proxy %d; want 1 and 1", own.Used(), p.Used())
	}
}
