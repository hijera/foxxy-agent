//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// newTestManager builds a headless Manager, skipping the test if no Chrome/Chromium
// is installed in the environment (so the browser-tagged suite stays green on hosts
// without a browser).
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(&config.BrowserConfig{Enabled: true})
	b, err := launch(m.cfg, "")
	if err != nil {
		t.Skipf("no browser available: %v", err)
	}
	b.close()
	return m
}

func TestValidateNavigateURL(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"https://example.com", false},
		{"http://127.0.0.1:8080/app", false}, // localhost allowed for the browser tool
		{"ftp://example.com", true},
		{"https://user:pass@example.com", true},
		{"not a url", true},
		{"", true},
	}
	for _, c := range cases {
		_, err := validateNavigateURL(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("validateNavigateURL(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
	}
}

func TestNavigateCapturesScreenshotAndVisionImage(t *testing.T) {
	m := newTestManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1 id="title">Hello Foxxy</h1></body></html>`))
	}))
	defer srv.Close()

	sessionDir := t.TempDir()
	var gotDataURL, gotPath string
	env := &tooling.Env{
		SessionID:  "s1",
		SessionDir: sessionDir,
		AddToolImage: func(dataURL, filePath, _ string) {
			gotDataURL, gotPath = dataURL, filePath
		},
	}
	defer m.closeSession("s1")

	args, _ := json.Marshal(navigateArgs{URL: srv.URL})
	res, err := m.executeNavigate(context.Background(), string(args), env)
	if err != nil {
		t.Fatalf("executeNavigate: %v", err)
	}
	if strings.HasPrefix(res, "error:") {
		t.Fatalf("navigate returned error result: %s", res)
	}
	if !strings.Contains(res, "navigated to") {
		t.Errorf("result missing action line: %s", res)
	}
	if !strings.Contains(res, "screenshot:") {
		t.Errorf("result missing screenshot path: %s", res)
	}
	// A vision image must have been handed to the agent.
	if !strings.HasPrefix(gotDataURL, "data:image/png;base64,") {
		t.Errorf("AddToolImage dataURL = %q", firstN(gotDataURL, 40))
	}
	if gotPath == "" {
		t.Fatal("AddToolImage filePath empty")
	}
	if _, err := os.Stat(gotPath); err != nil {
		t.Errorf("screenshot file not written: %v", err)
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
