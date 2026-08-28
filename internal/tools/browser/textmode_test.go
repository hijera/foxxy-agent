//go:build browser

package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// diagnosticPageHTML logs to the console, throws, and requests a URL that 500s —
// the three things a model needs to see when a page "looks blank".
const diagnosticPageHTML = `<html><body>
<h1>Report page</h1>
<button id="go">Run report</button>
<input id="q" aria-label="Search books" value="dune">
<a href="/next">Next page</a>
<script>
console.error("boom from console");
fetch("/api/broken").catch(function(){});
setTimeout(function(){ throw new Error("uncaught kaboom"); }, 0);
localStorage.setItem("auth_token", "` + longToken + `");
localStorage.setItem("theme", "dark");
sessionStorage.setItem("draft", "unsent message");
document.cookie = "visitor=42; path=/";
</script>
</body></html>`

func diagnosticServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(diagnosticPageHTML))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func navigateTo(t *testing.T, m *Manager, env *tooling.Env, url string) string {
	t.Helper()
	res, err := m.executeNavigate(context.Background(), `{"url":"`+url+`"}`, env)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return res
}

// TestPageLogReportsConsoleExceptionsAndNetworkWithoutAScreenshot is the whole
// point of the text-only path: everything that went wrong on the page, readable
// without an image.
func TestPageLogReportsConsoleExceptionsAndNetworkWithoutAScreenshot(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "pagelog")
	defer m.closeSession("pagelog")
	srv := diagnosticServer(t)

	// The log is reported by whoever reads it first and cleared by that read: the
	// navigate result takes whatever had arrived by then, this tool takes the rest.
	// The union is what a caller actually gets to see, so assert on that.
	var combined strings.Builder
	combined.WriteString(navigateTo(t, m, env, srv.URL))
	combined.WriteString("\n")

	var toolReads strings.Builder
	for range 10 {
		out, err := m.executePageLog(context.Background(), "{}", env)
		if err != nil {
			t.Fatalf("page log: %v", err)
		}
		toolReads.WriteString(out)
		toolReads.WriteString("\n")
		// The exception and the failed fetch are asynchronous; give them a beat.
		time.Sleep(150 * time.Millisecond)
	}
	combined.WriteString(toolReads.String())
	got := combined.String()

	for _, want := range []string{"boom from console", "uncaught kaboom", "[network] 500"} {
		if !strings.Contains(got, want) {
			t.Errorf("page log never reported %q; got:\n%s", want, got)
		}
	}
	// Only the navigate line may mention a screenshot; the tool itself must not.
	if strings.Contains(toolReads.String(), "screenshot") {
		t.Errorf("the page-log tool captured a screenshot; it must not:\n%s", toolReads.String())
	}
}

// TestReadPageOutlinesTheStructure covers the alternative to guessing selectors.
func TestReadPageOutlinesTheStructure(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "readpage")
	defer m.closeSession("readpage")
	srv := diagnosticServer(t)
	navigateTo(t, m, env, srv.URL)

	out, err := m.executeReadPage(context.Background(), "{}", env)
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	for _, want := range []string{"Report page", "Run report", "Search books", "Next page"} {
		if !strings.Contains(out, want) {
			t.Errorf("outline is missing %q; got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "button") || !strings.Contains(out, "link") {
		t.Errorf("outline does not name roles; got:\n%s", out)
	}
	if strings.Contains(out, "screenshot:") {
		t.Errorf("read_page captured a screenshot; it must not:\n%s", out)
	}

	// interactive_only drops the heading but keeps the controls.
	only, err := m.executeReadPage(context.Background(), `{"interactive_only":true}`, env)
	if err != nil {
		t.Fatalf("read page (interactive_only): %v", err)
	}
	if !strings.Contains(only, "Run report") {
		t.Errorf("interactive_only dropped a button; got:\n%s", only)
	}
	if strings.Contains(only, `heading "Report page"`) {
		t.Errorf("interactive_only kept a heading; got:\n%s", only)
	}

	// The selectors are the point of the outline: a model reads one and passes it
	// straight to fill or click. An outline whose selectors do not resolve would be
	// worse than none, because it looks authoritative.
	sel := selectorFor(t, only, "Search books")
	res, err := m.executeFill(context.Background(), `{"selector":"`+sel+`","text":"ithaca"}`, env)
	if err != nil {
		t.Fatalf("fill by reported selector %q: %v", sel, err)
	}
	if strings.HasPrefix(res, "error:") {
		t.Fatalf("reported selector %q did not resolve: %s", sel, res)
	}
	after, err := m.executeReadPage(context.Background(), `{"interactive_only":true}`, env)
	if err != nil {
		t.Fatalf("read page after fill: %v", err)
	}
	if !strings.Contains(after, `value="ithaca"`) {
		t.Errorf("outline does not reflect the value just typed; got:\n%s", after)
	}
}

// selectorFor pulls the selector the outline reported for the line naming want.
func selectorFor(t *testing.T, outline, want string) string {
	t.Helper()
	for _, line := range strings.Split(outline, "\n") {
		if !strings.Contains(line, want) {
			continue
		}
		_, sel, ok := strings.Cut(line, "selector=")
		if !ok {
			t.Fatalf("line for %q carries no selector: %q", want, line)
		}
		return strings.TrimSpace(sel)
	}
	t.Fatalf("outline has no line for %q:\n%s", want, outline)
	return ""
}

// TestScreenshotsCanBeTurnedOff covers the config switch: no image is captured and
// none is handed to the model, but the action still reports what it can.
func TestScreenshotsCanBeTurnedOff(t *testing.T) {
	off := false
	m := NewManager(&config.BrowserConfig{Enabled: true, Screenshots: &off})
	defer m.closeSession("noshots")

	var handedImages int
	env := &tooling.Env{
		SessionID:    "noshots",
		SessionDir:   t.TempDir(),
		AddToolImage: func(_, _, _ string) { handedImages++ },
	}
	srv := diagnosticServer(t)

	res, err := m.executeNavigate(context.Background(), `{"url":"`+srv.URL+`"}`, env)
	if err != nil {
		t.Skipf("no browser available: %v", err)
	}
	if handedImages != 0 {
		t.Errorf("screenshots are off but %d image(s) were handed to the model", handedImages)
	}
	if !strings.Contains(res, "screenshot: disabled") {
		t.Errorf("result does not say screenshots are off; got:\n%s", res)
	}
	if !strings.Contains(res, "url:") {
		t.Errorf("result lost the URL, which is what is left to report; got:\n%s", res)
	}
}

// TestScreenshotsOnByDefault keeps the switch opt-in.
func TestScreenshotsOnByDefault(t *testing.T) {
	if !(&config.BrowserConfig{}).ScreenshotsEnabled() {
		t.Error("screenshots must default to on")
	}
	off := false
	if (&config.BrowserConfig{Screenshots: &off}).ScreenshotsEnabled() {
		t.Error("screenshots: false must turn them off")
	}
}
