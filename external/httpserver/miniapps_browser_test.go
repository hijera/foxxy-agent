//go:build http && ui && miniapps && browser

package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// End-to-end coverage for Mini Apps: the production handler tree with the
// embedded SPA, driven by a real browser. Everything below the browser is the
// shipped code path — hash routing, the capability probe, the catalog fetch,
// the draft fetch, and the validate round trip.
//
// Behind the `browser` tag like external/ui/layout_browser_test.go, so the
// ordinary suites stay free of a Chrome dependency. Run it with:
//
//	go test -tags=http,ui,miniapps,browser -run TestMiniAppsBrowser ./external/httpserver
//
// The embedded SPA must be built first (npm run build:go in external/ui),
// otherwise the ui tag has no assets to serve.
const (
	miniAppsBrowserStepTimeout = 30 * time.Second
	miniAppsBrowserDumpTimeout = 5 * time.Second
)

func TestMiniAppsBrowserCatalogOpensTheEditor(t *testing.T) {
	ts := newMiniAppsBrowserServer(t)
	seedMiniAppForBrowser(t, ts.URL)

	browserCtx, cancel := newMiniAppsBrowserContext(t)
	defer cancel()

	var catalogRow, editorJSON string
	runMiniAppsBrowser(t, browserCtx, chromedp.Navigate(ts.URL+"/#/miniapps"))
	// The page renders only after the capability probe reports miniapps:true,
	// so waiting for it also covers GET /foxxycode/capabilities.
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-page"]`)
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-catalog-row`)
	runMiniAppsBrowser(t, browserCtx,
		chromedp.Text(`.miniapps-catalog-row`, &catalogRow, chromedp.ByQuery),
		chromedp.Click(`.miniapps-catalog-row`, chromedp.ByQuery),
	)
	// The raw document lives behind the editor's JSON tab, the way an author
	// reaches it.
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-tab-json"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-tab-json"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-raw-json`)
	runMiniAppsBrowser(t, browserCtx,
		chromedp.Value(`.miniapps-raw-json`, &editorJSON, chromedp.ByQuery),
	)

	if !strings.Contains(catalogRow, "Greeting") {
		t.Fatalf("catalog row = %q, want the seeded app name", catalogRow)
	}
	// The editor holds the document the server actually stored, not a shell.
	var draft map[string]any
	if err := json.Unmarshal([]byte(editorJSON), &draft); err != nil {
		t.Fatalf("editor JSON is not a document: %v (%.120q)", err, editorJSON)
	}
	if draft["id"] != "greeting-app" {
		t.Fatalf("editor shows draft %v, want greeting-app", draft["id"])
	}
	if strings.TrimSpace(asString(draft["revision"])) == "" {
		t.Fatal("editor draft carries no revision, so saving would conflict")
	}
}

func TestMiniAppsBrowserValidatesTheOpenDraft(t *testing.T) {
	ts := newMiniAppsBrowserServer(t)
	seedMiniAppForBrowser(t, ts.URL)

	browserCtx, cancel := newMiniAppsBrowserContext(t)
	defer cancel()

	var report string
	runMiniAppsBrowser(t, browserCtx, chromedp.Navigate(ts.URL+"/#/miniapps"))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-page"]`)
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-catalog-row`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`.miniapps-catalog-row`, chromedp.ByQuery))
	// Validation lives on the permissions tab, next to the release gate.
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-tab-permissions"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-tab-permissions"]`, chromedp.ByQuery))
	// Enabled only once the editor is clean, so waiting for it also proves the
	// draft loaded rather than landing in the dirty initial state.
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-validate"]:not([disabled])`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-validate"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-validation-report"]`)
	runMiniAppsBrowser(t, browserCtx,
		chromedp.Text(`[data-testid="miniapps-validation-report"]`, &report, chromedp.ByQuery),
	)

	if strings.TrimSpace(report) == "" {
		t.Fatal("validation report rendered empty")
	}
}

// The point of the feature: an operator launches a stored workflow from the
// generated form and watches it finish. The step really executes the write
// tool in an isolated run workspace.
func TestMiniAppsBrowserRunsFromTheGeneratedForm(t *testing.T) {
	ts, srv := newMiniAppsBrowserServerAndHandle(t)
	// Seeded through the store rather than POST /foxxycode/miniapps: that route
	// creates a draft with no source evidence, and StartTestRun requires it, so
	// a catalog-created app can never be test-run.
	seedMiniAppWithEvidence(t, srv)

	browserCtx, cancel := newMiniAppsBrowserContext(t)
	defer cancel()

	runMiniAppsBrowser(t, browserCtx, chromedp.Navigate(ts.URL+"/#/miniapps"))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-page"]`)
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-catalog-row`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`.miniapps-catalog-row`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-test-run"]:not([disabled])`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-test-run"]`, chromedp.ByQuery))

	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-run-start"]:not([disabled])`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-run-start"]`, chromedp.ByQuery))
	// The status dot carries the run state, so this waits on the real run
	// reaching a terminal success rather than on the form disappearing.
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-status-dot--succeeded`)
}

// runMiniAppsBrowser runs each action on the tab context itself. Wrapping the
// run in a timeout would tear the tab down on expiry, leaving nothing to
// diagnose, so waits poll instead (see waitForMiniAppsSelector) and every step
// here is expected to return promptly.
func runMiniAppsBrowser(t *testing.T, browserCtx context.Context, actions ...chromedp.Action) {
	t.Helper()
	if err := chromedp.Run(browserCtx, actions...); err != nil {
		t.Fatalf("browser run: %v\npage was showing:\n%s", err, miniAppsScreenText(browserCtx))
	}
}

// waitForMiniAppsSelector polls for a selector instead of using WaitVisible so
// a miss fails with the page contents rather than a bare deadline error.
func waitForMiniAppsSelector(t *testing.T, browserCtx context.Context, selector string) {
	t.Helper()
	script := `!!document.querySelector(` + jsString(selector) + `)`
	deadline := time.Now().Add(miniAppsBrowserStepTimeout)
	for time.Now().Before(deadline) {
		var present bool
		if err := chromedp.Run(browserCtx, chromedp.Evaluate(script, &present)); err != nil {
			t.Fatalf("polling for %s: %v", selector, err)
		}
		if present {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s never appeared\npage was showing:\n%s", selector, miniAppsScreenText(browserCtx))
}

func jsString(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func miniAppsScreenText(browserCtx context.Context) string {
	dumpCtx, cancel := context.WithTimeout(browserCtx, miniAppsBrowserDumpTimeout)
	defer cancel()
	var url, text string
	if err := chromedp.Run(dumpCtx,
		chromedp.Location(&url),
		chromedp.Evaluate(`document.body ? document.body.innerText.slice(0, 1200) : "(no body)"`, &text),
	); err != nil {
		return "  (could not read the page: " + err.Error() + ")"
	}
	return "  url: " + url + "\n  " + strings.ReplaceAll(strings.TrimSpace(text), "\n", "\n  ")
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}

// newMiniAppsBrowserServer mirrors newMiniAppsHTTPTestServer but writes a real
// config file and points Paths.ConfigPath at it. GET /foxxycode/onboarding/status
// reports first_run from that file's absence, and the SPA covers the whole
// screen with the provider picker while first_run is true — the Mini Apps page
// never mounts. A configured install is also what this flow is written for.
func newMiniAppsBrowserServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts, _ := newMiniAppsBrowserServerAndHandle(t)
	return ts
}

func newMiniAppsBrowserServerAndHandle(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	home := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	const configFile = "providers:\n" +
		"  - name: fake\n    type: openai\n    api_key: test\n" +
		"models:\n  - model: fake/model\n" +
		"agent:\n  model: fake/model\n"
	if err := os.WriteFile(configPath, []byte(configFile), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Paths:     config.Paths{Home: home, CWD: t.TempDir(), ConfigPath: configPath},
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model"}},
		Agent:     config.Agent{Model: "fake/model"},
	}
	mgr := session.NewManager(cfg, noopSender{}, func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}, slog.Default(), cfg.Paths.CWD, &session.FileStore{Root: t.TempDir()})
	srv := New(cfg, mgr, slog.Default(), cfg.Paths.CWD)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Drain() })
	return ts, srv
}

// seedMiniAppWithEvidence stores a draft together with the source evidence a
// test run replays, which the catalog create route does not write.
func seedMiniAppWithEvidence(t *testing.T, srv *Server) {
	t.Helper()
	raw, err := json.Marshal(miniAppsTestDocument())
	if err != nil {
		t.Fatal(err)
	}
	var app miniapps.MiniApp
	if err := json.Unmarshal(raw, &app); err != nil {
		t.Fatal(err)
	}
	// A test run is a replay against the source session: with no accepted result
	// recorded, verification passes when the artifacts match, and the expected
	// set comes from the sanitized trace. Distillation normally writes this;
	// here it stands in for the greeting file the write step produces.
	const accepted = "hello"
	digest := sha256.Sum256([]byte(accepted))
	evidence := &miniapps.SourceEvidence{
		SessionID: "seeded",
		SanitizedTrace: &miniapps.NormalizedTrace{Actions: []miniapps.TraceAction{{
			Index: 0, Name: "write", Kind: "builtin",
			Status:      miniapps.TraceActionSucceeded,
			ResultFound: true,
			Artifacts: []miniapps.TraceArtifact{{
				Path: "greeting.txt", Kind: "file",
				SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(accepted)),
			}},
		}}},
	}
	if err := srv.miniAppsHTTPState().store.CreateDraft(app, evidence); err != nil {
		t.Fatal(err)
	}
}

func seedMiniAppForBrowser(t *testing.T, baseURL string) {
	t.Helper()
	body, err := json.Marshal(miniAppsTestDocument())
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(baseURL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("seed status %d", response.StatusCode)
	}
}

func newMiniAppsBrowserContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	options := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	options = append(options,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(context.Background(), options...)
	browserCtx, browserCancel := chromedp.NewContext(allocatorCtx)
	return browserCtx, func() {
		browserCancel()
		allocatorCancel()
	}
}
