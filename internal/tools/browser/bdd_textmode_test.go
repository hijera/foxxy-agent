//go:build browser

package browser

// Godog harness for features/browser_text_mode.feature: drives the real tools
// against a real Chrome and a real HTTP server, so the scenarios assert what the
// agent actually receives rather than what a stub would report.

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

type textModeState struct {
	t *testing.T

	m   *Manager
	env *tooling.Env
	srv *httptest.Server

	handedImages int
	outline      string
	pageLog      string
	answer       string
	report       string
}

func (s *textModeState) reset(screenshots bool) error {
	s.closeAll()
	cfg := &config.BrowserConfig{Enabled: true}
	if !screenshots {
		off := false
		cfg.Screenshots = &off
	}
	s.m = NewManager(cfg)
	s.handedImages = 0
	s.env = &tooling.Env{
		SessionID:    "bdd-textmode",
		SessionDir:   s.t.TempDir(),
		AddToolImage: func(_, _, _ string) { s.handedImages++ },
	}
	s.outline, s.pageLog, s.answer, s.report = "", "", "", ""
	if s.srv == nil {
		s.srv = diagnosticServer(s.t)
	}
	return nil
}

func (s *textModeState) closeAll() {
	if s.m != nil {
		s.m.closeSession("bdd-textmode")
	}
}

func (s *textModeState) sessionOnDiagnosticPage() error {
	if err := s.reset(true); err != nil {
		return err
	}
	return s.navigate()
}

func (s *textModeState) configuredWithScreenshotsOff() error {
	return s.reset(false)
}

func (s *textModeState) navigate() error {
	out, err := s.m.executeNavigate(context.Background(), `{"url":"`+s.srv.URL+`"}`, s.env)
	if err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	if strings.HasPrefix(out, "error:") {
		return fmt.Errorf("navigate reported %q", out)
	}
	s.answer = out
	return nil
}

func (s *textModeState) readPage() error {
	out, err := s.m.executeReadPage(context.Background(), "{}", s.env)
	if err != nil {
		return err
	}
	s.outline = out
	return nil
}

func (s *textModeState) readPageLogUntilComplete() error {
	var all strings.Builder
	// The navigate answer took whatever had already arrived; the asynchronous
	// exception and failed fetch land over the next few hundred milliseconds.
	all.WriteString(s.answer)
	for range 10 {
		out, err := s.m.executePageLog(context.Background(), "{}", s.env)
		if err != nil {
			return err
		}
		all.WriteString("\n")
		all.WriteString(out)
		time.Sleep(150 * time.Millisecond)
	}
	s.pageLog = all.String()
	return nil
}

func (s *textModeState) outlineNamesHeadingAndButton() error {
	for _, want := range []string{"Report page", "Run report"} {
		if !strings.Contains(s.outline, want) {
			return fmt.Errorf("outline does not name %q:\n%s", want, s.outline)
		}
	}
	return nil
}

func (s *textModeState) outlineCarriesSelectorForSearchField() error {
	if _, err := s.searchSelector(); err != nil {
		return err
	}
	return nil
}

func (s *textModeState) searchSelector() (string, error) {
	for _, line := range strings.Split(s.outline, "\n") {
		if !strings.Contains(line, "Search books") {
			continue
		}
		_, sel, ok := strings.Cut(line, "selector=")
		if !ok {
			return "", fmt.Errorf("the search field line carries no selector: %q", line)
		}
		return strings.TrimSpace(sel), nil
	}
	return "", fmt.Errorf("outline has no search field:\n%s", s.outline)
}

func (s *textModeState) fillUsingReportedSelector() error {
	sel, err := s.searchSelector()
	if err != nil {
		return err
	}
	out, err := s.m.executeFill(context.Background(), `{"selector":"`+sel+`","text":"ithaca"}`, s.env)
	if err != nil {
		return err
	}
	if strings.HasPrefix(out, "error:") {
		return fmt.Errorf("the selector the outline reported (%q) did not resolve: %s", sel, out)
	}
	return nil
}

func (s *textModeState) outlineReportsTypedValue() error {
	if err := s.readPage(); err != nil {
		return err
	}
	if !strings.Contains(s.outline, `value="ithaca"`) {
		return fmt.Errorf("outline does not reflect the typed value:\n%s", s.outline)
	}
	return nil
}

func (s *textModeState) pageLogReports(what string) error {
	wants := map[string]string{
		"console error": "boom from console",
		"exception":     "uncaught kaboom",
		"failed":        "[network] 500",
	}
	want, ok := wants[what]
	if !ok {
		return fmt.Errorf("unknown expectation %q", what)
	}
	if !strings.Contains(s.pageLog, want) {
		return fmt.Errorf("page log never reported %q:\n%s", want, s.pageLog)
	}
	return nil
}

func (s *textModeState) inspect(what string) error {
	out, err := s.m.executeInspect(context.Background(), `{"what":"`+what+`"}`, s.env)
	if err != nil {
		return err
	}
	if strings.HasPrefix(out, "error:") {
		return fmt.Errorf("inspect %s reported %q", what, out)
	}
	s.report = out
	return nil
}

func (s *textModeState) reportNamesEveryStore() error {
	for _, want := range []string{"localStorage", "sessionStorage", "cookies", "auth_token", "draft", "visitor"} {
		if !strings.Contains(s.report, want) {
			return fmt.Errorf("storage report is missing %q:\n%s", want, s.report)
		}
	}
	return nil
}

func (s *textModeState) longValueTruncated() error {
	if strings.Contains(s.report, longToken) {
		return fmt.Errorf("a long stored value was reported in full:\n%s", s.report)
	}
	if !strings.Contains(s.report, "chars)") {
		return fmt.Errorf("the report does not mark the truncation:\n%s", s.report)
	}
	return nil
}

func (s *textModeState) reportBreaksLoadIntoPhases() error {
	for _, want := range []string{"ttfb", "dom_content_loaded", "load"} {
		if !strings.Contains(s.report, want) {
			return fmt.Errorf("timing report is missing phase %q:\n%s", want, s.report)
		}
	}
	return nil
}

func (s *textModeState) reportNamesSlowestRequests() error {
	if !strings.Contains(s.report, "requests:") {
		return fmt.Errorf("timing report does not count requests:\n%s", s.report)
	}
	return nil
}

func (s *textModeState) reportCountsDOMNodes() error {
	if !strings.Contains(s.report, "dom_nodes:") {
		return fmt.Errorf("memory report does not count DOM nodes:\n%s", s.report)
	}
	return nil
}

func (s *textModeState) reportSizesHeapOrAdmitsItCannot() error {
	if !strings.Contains(s.report, "js_heap:") {
		return fmt.Errorf("memory report says nothing about the heap:\n%s", s.report)
	}
	return nil
}

func (s *textModeState) noScreenshotTaken() error {
	// The text tools must not capture; the navigate in Background may have.
	body := s.outline + s.pageLog + s.report
	body = strings.ReplaceAll(body, s.answer, "")
	if strings.Contains(body, "screenshot") {
		return fmt.Errorf("a text tool captured a screenshot:\n%s", body)
	}
	return nil
}

func (s *textModeState) noImageHandedToModel() error {
	if s.handedImages != 0 {
		return fmt.Errorf("screenshots are off but %d image(s) reached the model", s.handedImages)
	}
	return nil
}

func (s *textModeState) answerReportsURL() error {
	if !strings.Contains(s.answer, "url:") {
		return fmt.Errorf("answer does not report the URL:\n%s", s.answer)
	}
	return nil
}

func (s *textModeState) answerSaysScreenshotsDisabled() error {
	if !strings.Contains(s.answer, "screenshot: disabled") {
		return fmt.Errorf("answer does not say screenshots are off:\n%s", s.answer)
	}
	return nil
}

func initializeTextModeScenario(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &textModeState{t: t}
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			s.closeAll()
			return ctx, nil
		})
		sc.Step(`^a browser session on a page that logs an error, throws, requests a URL that fails, and writes to every store$`, s.sessionOnDiagnosticPage)
		sc.Step(`^the browser is configured with screenshots off$`, s.configuredWithScreenshotsOff)
		sc.Step(`^I navigate to the page$`, s.navigate)
		sc.Step(`^I read the page as text$`, s.readPage)
		sc.Step(`^I read the page log until it reports everything$`, s.readPageLogUntilComplete)
		sc.Step(`^I fill the search field using the selector the outline reported$`, s.fillUsingReportedSelector)
		sc.Step(`^the outline names the heading and the button$`, s.outlineNamesHeadingAndButton)
		sc.Step(`^the outline carries a selector for the search field$`, s.outlineCarriesSelectorForSearchField)
		sc.Step(`^the outline reports the value I typed$`, s.outlineReportsTypedValue)
		sc.Step(`^the page log reports the console error$`, func() error { return s.pageLogReports("console error") })
		sc.Step(`^the page log reports the uncaught exception with its message$`, func() error { return s.pageLogReports("exception") })
		sc.Step(`^the page log reports the failed request and its status$`, func() error { return s.pageLogReports("failed") })
		sc.Step(`^I inspect the page storage$`, func() error { return s.inspect("storage") })
		sc.Step(`^I inspect the page timing$`, func() error { return s.inspect("timing") })
		sc.Step(`^I inspect the page memory$`, func() error { return s.inspect("memory") })
		sc.Step(`^the report names every store the page wrote$`, s.reportNamesEveryStore)
		sc.Step(`^a long stored value is truncated$`, s.longValueTruncated)
		sc.Step(`^the report breaks the load into phases$`, s.reportBreaksLoadIntoPhases)
		sc.Step(`^the report names the slowest requests$`, s.reportNamesSlowestRequests)
		sc.Step(`^the report counts the DOM nodes$`, s.reportCountsDOMNodes)
		sc.Step(`^the report either sizes the JS heap or says it is unavailable$`, s.reportSizesHeapOrAdmitsItCannot)
		sc.Step(`^no screenshot was taken$`, s.noScreenshotTaken)
		sc.Step(`^no image is handed to the model$`, s.noImageHandedToModel)
		sc.Step(`^the answer still reports the page URL$`, s.answerReportsURL)
		sc.Step(`^the answer says screenshots are disabled$`, s.answerSaysScreenshotsDisabled)
	}
}

func TestBrowserTextModeFeature(t *testing.T) {
	// Skip cleanly where no Chrome is installed, like the other browser tests.
	newTestManager(t)

	suite := godog.TestSuite{
		Name:                "browser-text-mode",
		ScenarioInitializer: initializeTextModeScenario(t),
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../../features/browser_text_mode.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("browser text mode feature suite failed")
	}
}
