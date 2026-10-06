//go:build http && ui && browser

package httpserver

// Godog harness for features/ide_panel_error_overlay.feature. It runs the real
// bootstrap script the IntelliJ panel injects — read straight out of the Kotlin
// source by intellijBootstrapScript — on a blank page, and drives the promise
// rejections the SPA produces when the local backend drops a request.
//
// The state machine, the browser tab and the overlay assertions are shared with
// bdd_ide_panel_resize_loop_test.go; only the rejection steps are new.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// panelRejectionSettled is how long a dispatched unhandledrejection gets to reach
// the bootstrap's listener. The steps below do not sleep for it: they wait on a
// probe listener that the browser sets once the event has actually been delivered,
// so "the overlay stays hidden" is a fact rather than a race.
const panelRejectionSettled = 10 * time.Second

// rejectWith fires an unhandled rejection carrying reasonExpr (a JS expression) and
// returns once the page has delivered the event to every listener.
func (s *panelLoopState) rejectWith(reasonExpr string) error {
	script := fmt.Sprintf(`(function () {
  window.__foxxyRejSeen = false;
  if (!window.__foxxyRejProbe) {
    window.__foxxyRejProbe = true;
    // Registered after the bootstrap's listener, so by the time this flips the
    // bootstrap has already decided whether to paint.
    window.addEventListener("unhandledrejection", function () { window.__foxxyRejSeen = true; });
  }
  Promise.reject(%s);
})()`, reasonExpr)
	if err := s.eval(script, nil); err != nil {
		return err
	}
	return s.waitUntil(`window.__foxxyRejSeen === true`, panelRejectionSettled)
}

func (s *panelLoopState) backgroundRequestRejects(message string) error {
	return s.rejectWith(fmt.Sprintf(`new TypeError(%q)`, message))
}

func (s *panelLoopState) requestAbortedByThePage() error {
	return s.rejectWith(`(function () {
  var e = new Error("The user aborted a request.");
  e.name = "AbortError";
  return e;
})()`)
}

func (s *panelLoopState) promiseRejectsUncaught(message string) error {
	return s.rejectWith(fmt.Sprintf(`new Error(%q)`, message))
}

func (s *panelLoopState) overlayCanBeDismissed() error {
	var clicked bool
	if err := s.eval(`(function () {
  var btn = document.getElementById("foxxycode-err-overlay-close");
  if (!btn) return false;
  btn.click();
  return true;
})()`, &clicked); err != nil {
		return err
	}
	if !clicked {
		return fmt.Errorf("the overlay has no close button")
	}
	return s.overlayStaysHidden()
}

func TestIDEPanelErrorOverlayFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "ide_panel_error_overlay",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			s := &panelLoopState{}
			sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
				s.close()
				return ctx, err
			})
			sc.Step(`^the IntelliJ panel bootstrap script is installed on a blank page$`, s.bootstrapInstalledOnBlankPage)
			sc.Step(`^a background request rejects with "([^"]*)"$`, s.backgroundRequestRejects)
			sc.Step(`^a request is aborted by the page$`, s.requestAbortedByThePage)
			sc.Step(`^a promise rejects with an uncaught "([^"]*)"$`, s.promiseRejectsUncaught)
			sc.Step(`^the error overlay stays hidden$`, s.overlayStaysHidden)
			sc.Step(`^the error overlay reports "([^"]*)"$`, s.overlayReports)
			sc.Step(`^the error overlay can be dismissed$`, s.overlayCanBeDismissed)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/ide_panel_error_overlay.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("ide_panel_error_overlay feature failed")
	}
}
