//go:build http && ui && browser

package httpserver

// Godog harness for features/ide_panel_resize_loop.feature: drives the real HTTP
// server with its embedded SPA in a headless Chrome, the way the IntelliJ and
// VS Code panels embed it (?embed=intellij), and watches the browser's resize
// observation loop from inside the page.
//
// The failure it guards was only ever visible in JCEF (Chromium 104), which
// raises "ResizeObserver loop limit exceeded" the moment a resize callback
// re-layouts what it measured. A current Chrome tolerates that same pattern,
// so the scenario does not wait for the notice: it instruments ResizeObserver
// and checks the contract directly - the composer reserve (the scroll tail's
// height) may only change in a later animation frame than the observation
// that measured it. The second scenario runs the IntelliJ bootstrap script
// itself and checks that the overlay it installs ignores the loop notice.
//
// Needs a Chrome/Chromium (FOXXYCODE_UI_BROWSER overrides the lookup) - run it
// with `npm run test:panel` from external/ui, next to test:layout.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const (
	panelLoopBrowserTimeout = 2 * time.Minute
	panelLoopWSURLTimeout   = 60 * time.Second
	panelLoopReserveSettle  = 500 * time.Millisecond
	panelLoopSessionID      = "sess_panel_loop_long_transcript"
)

// panelProbeScript runs at document start in the embedded panel's page. It
// counts animation frames, wraps ResizeObserver so every observation remembers
// the frame it ran in and the composer reserve it saw, and flags a reserve
// write that lands in that same frame. It also records the browser's own loop
// notice, should the engine raise one.
const panelProbeScript = `(function () {
  var W = window;
  W.__panelProbe = { roErrors: [], violations: [], writes: 0, callbacks: 0, frame: 0 };
  W.addEventListener("error", function (ev) {
    if (/ResizeObserver loop/.test(ev.message || "")) W.__panelProbe.roErrors.push(ev.message);
  });
  (function tick() { W.__panelProbe.frame++; W.requestAnimationFrame(tick); })();
  var Orig = W.ResizeObserver;
  if (!Orig) return;
  var reserveEl = function () { return document.querySelector("[style*='--chat-composer-reserve']"); };
  var readReserve = function () {
    var el = reserveEl();
    return el ? el.style.getPropertyValue("--chat-composer-reserve") : null;
  };
  var last = { frame: -1, reserve: null };
  var watch = function () {
    var el = reserveEl();
    if (!el || el.__panelProbeWatched) return;
    el.__panelProbeWatched = true;
    new MutationObserver(function () {
      var now = readReserve();
      if (now === last.reserve) return;
      W.__panelProbe.writes++;
      if (last.frame === W.__panelProbe.frame) {
        W.__panelProbe.violations.push({ frame: last.frame, from: last.reserve, to: now });
      }
      last.reserve = now;
    }).observe(el, { attributes: true, attributeFilter: ["style"] });
  };
  W.ResizeObserver = function (cb) {
    return new Orig(function (entries, obs) {
      W.__panelProbe.callbacks++;
      watch();
      last = { frame: W.__panelProbe.frame, reserve: readReserve() };
      return cb(entries, obs);
    });
  };
  W.ResizeObserver.prototype = Orig.prototype;
})();`

type panelLoopState struct {
	root       string
	sessRoot   string
	ts         *httptest.Server
	cancels    []context.CancelFunc
	tab        context.Context
	browserLog bytes.Buffer
}

func (s *panelLoopState) close() {
	for i := len(s.cancels) - 1; i >= 0; i-- {
		s.cancels[i]()
	}
	s.cancels = nil
	s.tab = nil
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

func (s *panelLoopState) serverWithTranscript(exchanges int) error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-panel-loop-")
	if err != nil {
		return err
	}
	s.root = root
	home := filepath.Join(root, "home")
	s.sessRoot = filepath.Join(root, "sessions")
	dir := filepath.Join(s.sessRoot, panelLoopSessionID)
	for _, d := range []string{home, dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	meta := map[string]interface{}{
		"version":   1,
		"id":        panelLoopSessionID,
		"cwd":       root,
		"mode":      "agent",
		"title":     "long transcript",
		"updatedAt": "2026-09-01T10:00:00Z",
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), b, 0o644); err != nil {
		return err
	}
	// Each exchange is a user prompt, an assistant tool call, its result and the
	// answer - the row mix of a real transcript with tool cards.
	msgs := make([]map[string]interface{}, 0, exchanges*4)
	for i := 0; i < exchanges; i++ {
		callID := fmt.Sprintf("call_%03d", i)
		msgs = append(msgs,
			map[string]interface{}{"role": "user", "content": fmt.Sprintf("Read notes.txt and report line %d.", i)},
			map[string]interface{}{
				"role":    "assistant",
				"content": fmt.Sprintf("Reading line %d.", i),
				"tool_calls": []map[string]interface{}{{
					"id":    callID,
					"name":  "read",
					"input": `{"path":"notes.txt"}`,
				}},
			},
			map[string]interface{}{"role": "tool", "content": fmt.Sprintf("MARKER-LINE-%d", i), "tool_call_id": callID},
			map[string]interface{}{"role": "assistant", "content": fmt.Sprintf("Line %d carries the marker.", i)},
		)
	}
	payload, err := json.Marshal(map[string]interface{}{"version": 1, "messages": msgs})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.json"), payload, 0o644); err != nil {
		return err
	}

	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, &session.FileStore{Root: s.sessRoot})
	srv := New(cfg, mgr, slog.Default(), root)
	s.ts = httptest.NewServer(srv.Handler())
	return nil
}

// openTab launches a headless Chrome sized like a narrow IDE tool window and
// returns a tab context with the probe installed for every document.
func (s *panelLoopState) openTab(width, height int, probe bool) error {
	browserCtx, browserCancel := context.WithTimeout(context.Background(), panelLoopBrowserTimeout)
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	userData, err := os.MkdirTemp("", "foxxycode-panel-loop-profile-")
	if err != nil {
		browserCancel()
		return err
	}
	opts = append(opts,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-gpu-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WindowSize(width, height),
		chromedp.UserDataDir(userData),
		chromedp.CombinedOutput(&s.browserLog),
		// chromedp gives Chrome 20 s to publish its DevTools websocket URL, and a
		// two-core runner starting several browsers at once does not always make
		// it. Past that the failure reads "websocket url timeout reached", which
		// looks like a hang rather than a slow start. external/ui sets the same
		// bound for the same reason; keep the two in step.
		chromedp.WSURLReadTimeout(panelLoopWSURLTimeout),
	)
	if executable := strings.TrimSpace(os.Getenv("FOXXYCODE_UI_BROWSER")); executable != "" {
		opts = append(opts, chromedp.ExecPath(executable))
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(browserCtx, opts...)
	tab, tabCancel := chromedp.NewContext(allocCtx)
	s.cancels = append(s.cancels, browserCancel, allocCancel, tabCancel, func() { _ = os.RemoveAll(userData) })
	s.tab = tab
	if !probe {
		return chromedp.Run(tab, chromedp.Navigate("about:blank"))
	}
	return chromedp.Run(tab,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(panelProbeScript).Do(ctx)
			return err
		}),
		chromedp.Navigate("about:blank"),
	)
}

func (s *panelLoopState) eval(expr string, out interface{}) error {
	if s.tab == nil {
		return fmt.Errorf("no browser tab is open")
	}
	return chromedp.Run(s.tab, chromedp.Evaluate(expr, out))
}

func (s *panelLoopState) waitUntil(expr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var ok bool
		if err := s.eval(expr, &ok); err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", expr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *panelLoopState) panelOpensTranscript(width int) error {
	if s.ts == nil {
		return fmt.Errorf("the server is not running")
	}
	if err := s.openTab(width, 700, true); err != nil {
		return err
	}
	url := s.ts.URL + "/?embed=intellij&theme=light#/s/" + panelLoopSessionID
	if err := chromedp.Run(s.tab, chromedp.Navigate(url)); err != nil {
		return err
	}
	if err := s.waitUntil(`!!document.getElementById("composer")`, 20*time.Second); err != nil {
		return fmt.Errorf("composer never rendered: %w (browser: %s)", err, s.browserLog.String())
	}
	return s.waitUntil(`document.querySelectorAll("#messages details").length >= 10`, 20*time.Second)
}

func (s *panelLoopState) userDraftsFiveLines() error {
	if err := chromedp.Run(s.tab, chromedp.Focus("#composer", chromedp.ByQuery)); err != nil {
		return err
	}
	// Inserted as text (the way a paste arrives), so the newlines grow the
	// textarea instead of submitting the draft.
	if err := chromedp.Run(s.tab, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.InsertText("first line\nsecond line\nthird line\nfourth line\nfifth line").Do(ctx)
	})); err != nil {
		return err
	}
	// A few frames for the resize observation and the deferred reserve write.
	//
	// This scenario earned its keep here: at this window it used to fail about
	// four runs in six, and more often when the window was widened, with a
	// reserve write of 206px -> 181px - the composer shrinking back - landing in
	// the frame that measured it. The cause was in ChatScreen, not in the test:
	// the reserve went through React state, so the DOM write happened whenever
	// React committed rather than in the frame that scheduled it, and a commit
	// after the frame's resize observations is a write inside the delivery loop
	// again. It is written to the element directly now, and the scenario passes
	// ten runs in ten here and eight in eight at three seconds.
	return chromedp.Run(s.tab, chromedp.Sleep(panelLoopReserveSettle))
}

func (s *panelLoopState) transcriptAndComposerRendered() error {
	var probe struct {
		Callbacks int `json:"callbacks"`
	}
	if err := s.eval(`window.__panelProbe`, &probe); err != nil {
		return err
	}
	if probe.Callbacks == 0 {
		return fmt.Errorf("no ResizeObserver callback ran: the scenario would prove nothing")
	}
	var rows int
	if err := s.eval(`document.querySelectorAll("#messages details").length`, &rows); err != nil {
		return err
	}
	if rows < 10 {
		return fmt.Errorf("only %d tool rows rendered", rows)
	}
	return nil
}

func (s *panelLoopState) noLoopError() error {
	var errs []string
	if err := s.eval(`window.__panelProbe.roErrors`, &errs); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("the browser raised %q", errs)
	}
	return nil
}

func (s *panelLoopState) reserveWritesLandInALaterFrame() error {
	var probe struct {
		Writes     int                      `json:"writes"`
		Callbacks  int                      `json:"callbacks"`
		Frame      int                      `json:"frame"`
		Violations []map[string]interface{} `json:"violations"`
	}
	if err := s.eval(`window.__panelProbe`, &probe); err != nil {
		return err
	}
	if probe.Writes == 0 {
		// Say enough to tell the two ways this happens apart: a composer that
		// never grew (no resize callbacks at all, so nothing to defer) from one
		// whose deferred write had not landed yet when the settle ran out.
		var height string
		_ = s.eval(`String(document.getElementById("composer")?.getBoundingClientRect().height ?? "no composer")`, &height)
		return fmt.Errorf(
			"the composer reserve was never written after the draft grew the composer (resize callbacks: %d, frames: %d, composer height: %s)",
			probe.Callbacks, probe.Frame, height)
	}
	if len(probe.Violations) > 0 {
		return fmt.Errorf("%d reserve write(s) landed in the frame of the observation that measured them: %v",
			len(probe.Violations), probe.Violations)
	}
	return nil
}

// intellijBootstrapScript returns the JS the IntelliJ panel injects into every
// page (FoxxyCodeBrowserPanel.BOOTSTRAP_JS), read from the Kotlin source so the
// scenario runs the real thing.
func intellijBootstrapScript() (string, error) {
	path := filepath.Join("..", "..", "editors", "intellij", "src", "main", "kotlin",
		"dev", "foxxycode", "intellij", "ui", "FoxxyCodeBrowserPanel.kt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	src := strings.ReplaceAll(string(data), "\r\n", "\n")
	const open = `BOOTSTRAP_JS = """`
	start := strings.Index(src, open)
	if start < 0 {
		return "", fmt.Errorf("BOOTSTRAP_JS not found in %s", path)
	}
	rest := src[start+len(open):]
	end := strings.Index(rest, `"""`)
	if end < 0 {
		return "", fmt.Errorf("BOOTSTRAP_JS is not terminated in %s", path)
	}
	return rest[:end], nil
}

func (s *panelLoopState) bootstrapInstalledOnBlankPage() error {
	script, err := intellijBootstrapScript()
	if err != nil {
		return err
	}
	if err := s.openTab(800, 600, false); err != nil {
		return err
	}
	return s.eval(script, nil)
}

func (s *panelLoopState) pageRaises(message string) error {
	expr := fmt.Sprintf(`window.dispatchEvent(new ErrorEvent("error", { message: %q }))`, message)
	return s.eval(expr, nil)
}

func (s *panelLoopState) pageRaisesUncaught(message string) error {
	expr := fmt.Sprintf(`window.dispatchEvent(new ErrorEvent("error", { message: %q, error: new Error(%q) }))`,
		message, message)
	return s.eval(expr, nil)
}

func (s *panelLoopState) overlayStaysHidden() error {
	var shown bool
	if err := s.eval(`!!document.getElementById("foxxycode-err-overlay")`, &shown); err != nil {
		return err
	}
	if shown {
		var text string
		_ = s.eval(`document.getElementById("foxxycode-err-overlay").textContent`, &text)
		return fmt.Errorf("the overlay is showing: %q", text)
	}
	return nil
}

func (s *panelLoopState) overlayReports(text string) error {
	var got string
	if err := s.eval(`(document.getElementById("foxxycode-err-overlay") || {}).textContent || ""`, &got); err != nil {
		return err
	}
	if !strings.Contains(got, text) {
		return fmt.Errorf("overlay text %q does not mention %q", got, text)
	}
	return nil
}

func TestIDEPanelResizeLoopFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "ide_panel_resize_loop",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			s := &panelLoopState{}
			sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
				s.close()
				return ctx, err
			})
			sc.Step(`^a foxxycode HTTP server with a transcript of (\d+) tool-call exchanges$`, s.serverWithTranscript)
			sc.Step(`^the embedded panel opens that transcript at (\d+) pixels wide$`, s.panelOpensTranscript)
			sc.Step(`^the user drafts a five-line message in the composer$`, s.userDraftsFiveLines)
			sc.Step(`^the transcript and the composer are rendered$`, s.transcriptAndComposerRendered)
			sc.Step(`^no ResizeObserver loop error was raised$`, s.noLoopError)
			sc.Step(`^every composer reserve write landed in a later frame than the resize observation that measured it$`, s.reserveWritesLandInALaterFrame)
			sc.Step(`^the IntelliJ panel bootstrap script is installed on a blank page$`, s.bootstrapInstalledOnBlankPage)
			sc.Step(`^the page raises "([^"]*)"$`, s.pageRaises)
			sc.Step(`^the page raises an uncaught "([^"]*)"$`, s.pageRaisesUncaught)
			sc.Step(`^the error overlay stays hidden$`, s.overlayStaysHidden)
			sc.Step(`^the error overlay reports "([^"]*)"$`, s.overlayReports)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/ide_panel_resize_loop.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("ide_panel_resize_loop feature failed")
	}
}
