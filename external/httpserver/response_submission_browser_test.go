//go:build http && ui && browser

package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/hijera/foxxycode-agent/internal/config"
)

// Exercise the shipped SPA against the real server, substituting only the receipt
// response so the interruption branch is deterministic and needs no live provider.
func TestComposerShowsInterruptedReceipt(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	cfg := s.srv.activeCfg()
	cfg.Providers = []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}}
	cfg.Paths.ConfigPath = filepath.Join(s.root, "config.yaml")
	if err := os.WriteFile(cfg.Paths.ConfigPath, []byte("agent: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	keys := make(chan string, 2)
	handler := s.srv.Handler()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/responses" {
			keys <- r.Header.Get("Idempotency-Key")
			_, sid, _, err := s.srv.resolveSession(r.Context(), r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": sid, "object": "response", "status": "interrupted"})
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer ts.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.UserDataDir(t.TempDir()), chromedp.Flag("headless", true), chromedp.Flag("no-sandbox", true))
	if path := os.Getenv("FOXXYCODE_UI_BROWSER"); path != "" {
		opts = append(opts, chromedp.ExecPath(path))
	}
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	browser, closeBrowser := chromedp.NewContext(alloc)
	defer closeBrowser()
	ctx, stop := context.WithTimeout(browser, 20*time.Second)
	defer stop()
	defer func() {
		// Close Chrome before cancelling its command context so profile writers
		// finish before testing removes the temporary user data directory.
		shutdown, shutdownCancel := context.WithTimeout(browser, 5*time.Second)
		defer shutdownCancel()
		if err := chromedp.Cancel(shutdown); err != nil {
			t.Errorf("close browser: %v", err)
		}
	}()
	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL+"/"), chromedp.WaitVisible("#composer"), chromedp.SendKeys("#composer", "continue"), chromedp.Click("#btn-send"), chromedp.Poll(`document.body.innerText.includes("The previous request was interrupted") || document.body.innerText.includes("Предыдущий запрос был прерван")`, nil)); err != nil {
		t.Fatal(err)
	}
	select {
	case key := <-keys:
		if key == "" {
			t.Fatal("composer omitted idempotency key")
		}
	default:
		t.Fatal("composer did not submit")
	}
	select {
	case <-keys:
		t.Fatal("composer resubmitted interrupted request")
	default:
	}
}
