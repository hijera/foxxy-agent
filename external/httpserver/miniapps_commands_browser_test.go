//go:build http && ui && miniapps && browser

package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
)

// TestMiniAppsBrowserCommandsPanel drives the commands panel in the real SPA:
// the row shows the untrusted embedded profile, the trust button records the
// approval, and the row flips to trusted. Set FOXXYCODE_SHOT_DIR to also save
// PNG screenshots of both states (used for the PR's UI evidence).
func TestMiniAppsBrowserCommandsPanel(t *testing.T) {
	fake, err := cmdtest.Build(t.TempDir(), "fakeenc")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ts := newMiniAppsBrowserServer(t)
	body, err := json.Marshal(commandsTestDocument())
	if err != nil {
		t.Fatal(err)
	}
	created, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("seed status %d", created.StatusCode)
	}

	browserCtx, cancel := newMiniAppsBrowserContext(t)
	defer cancel()

	runMiniAppsBrowser(t, browserCtx, chromedp.Navigate(ts.URL+"/#/miniapps"))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-page"]`)
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-catalog-row`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`.miniapps-catalog-row`, chromedp.ByQuery))
	// The commands panel lives on the permissions tab, next to the release gates.
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-tab-permissions"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-tab-permissions"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-command-fakeenc_convert"]`)
	saveMiniAppsScreenshot(t, browserCtx, "commands-panel-untrusted.png")

	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-command-trust-fakeenc_convert"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-command-trust-fakeenc_convert"]`, chromedp.ByQuery))
	// After trusting, the trust button disappears and the row shows trusted.
	waitForMiniAppsGone(t, browserCtx, `[data-testid="miniapps-command-trust-fakeenc_convert"]`)
	saveMiniAppsScreenshot(t, browserCtx, "commands-panel-trusted.png")
}

// waitForMiniAppsGone polls until the selector matches nothing.
func waitForMiniAppsGone(t *testing.T, browserCtx context.Context, selector string) {
	t.Helper()
	script := `!document.querySelector(` + jsString(selector) + `)`
	deadline := miniAppsPollDeadline()
	for deadline() {
		var gone bool
		if err := chromedp.Run(browserCtx, chromedp.Evaluate(script, &gone)); err != nil {
			t.Fatalf("polling for absence of %s: %v", selector, err)
		}
		if gone {
			return
		}
	}
	t.Fatalf("%s never disappeared\npage was showing:\n%s", selector, miniAppsScreenText(browserCtx))
}

// saveMiniAppsScreenshot writes a full-page capture when FOXXYCODE_SHOT_DIR is
// set; otherwise it is a no-op so CI stays artifact-free.
func saveMiniAppsScreenshot(t *testing.T, browserCtx context.Context, name string) {
	t.Helper()
	dir := strings.TrimSpace(os.Getenv("FOXXYCODE_SHOT_DIR"))
	if dir == "" {
		return
	}
	var shot []byte
	if err := chromedp.Run(browserCtx, chromedp.FullScreenshot(&shot, 90)); err != nil {
		t.Fatalf("screenshot %s: %v", name, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), shot, 0o644); err != nil {
		t.Fatal(err)
	}
}
