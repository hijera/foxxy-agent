//go:build browser

package browser

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func TestBrowserRestartsWhenPreviousManagerStillOwnsProfile(t *testing.T) {
	first := newTestManager(t)
	profile := profileDirFor(t.TempDir())
	const key = "restart"
	oldBrowser, err := first.get(key, profile, first.cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.closeSession(key) })

	// A replacement backend does not own the old manager or its Chrome,
	// but reads the same session directory from disk.
	second := NewManager(first.cfg)
	recovered, err := second.get(key, profile, second.cfg)
	if err != nil {
		t.Fatalf("cannot reopen the session while the original browser is still running: %v", err)
	}
	t.Cleanup(func() { second.closeSession(key) })
	if recovered.recoveryProfile == "" || recovered.recoveryProfile == profile {
		t.Fatal("reopening must use a separate profile while the original is live")
	}
	var got int
	if err := recovered.run(chromedp.Evaluate("6 * 7", &got)); err != nil || got != 42 {
		t.Fatalf("reopened browser is unusable: value=%d err=%v", got, err)
	}
	if err := oldBrowser.run(chromedp.Evaluate("1", nil)); err != nil {
		t.Fatalf("reopening interfered with the original profile owner: %v", err)
	}
	second.closeSession(key)
	if _, err := os.Stat(recovered.recoveryProfile); !os.IsNotExist(err) {
		t.Fatalf("temporary profile was not removed: %v", err)
	}
	first.closeSession(key)
	reopened, err := second.get(key, profile, second.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.recoveryProfile != "" {
		t.Fatal("reopening must return to the original profile once it is free")
	}
}

func TestBrowserRestartsAfterCachedChromeExits(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "chrome-exit")
	original, err := m.open(env)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.closeSession(env.SessionID) })
	tree := platform.CaptureProcessTree(original.chromePID)
	defer tree.Release()
	process, err := os.FindProcess(original.chromePID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Release() }()
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-chromedp.FromContext(original.rootCtx).Browser.LostConnection:
	case <-time.After(10 * time.Second):
		t.Fatal("Chrome exited without closing its debugger connection")
	}
	if !tree.WaitExit(browserExitGrace) {
		tree.TerminateSurvivors()
	}
	reopened, err := m.open(env)
	if err != nil {
		t.Fatal(err)
	}
	if reopened == original || reopened.recoveryProfile != "" {
		t.Fatal("expected a new browser using the now-free session profile")
	}
}

func TestBrowserRestartFeature(t *testing.T) {
	first := newTestManager(t)
	env := testEnv(t, "shared")
	state := &session.State{}
	t.Cleanup(state.CloseAll)
	env.SessionBrowser = state.SessionBrowser
	second := NewManager(first.cfg)
	var original, reused *Browser
	suite := godog.TestSuite{
		Name:    "browser-restart",
		Options: &godog.Options{Format: "progress", Paths: []string{"../../../features/browser_restart.feature"}, TestingT: t},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a live session with a running browser$`, func() (err error) {
				original, err = first.open(env)
				return err
			})
			sc.Step(`^a new browser tool registry opens that session's browser$`, func() (err error) {
				reused, err = second.open(env)
				return err
			})
			sc.Step(`^it reuses the original Chrome connection$`, func() error {
				if reused != original || !reused.connected() {
					return fmt.Errorf("the next registry started another Chrome process")
				}
				return nil
			})
			sc.Step(`^releasing the session closes Chrome and prevents a late relaunch$`, func() error {
				state.CloseAll()
				if original.connected() {
					return fmt.Errorf("session release left Chrome connected")
				}
				if _, err := second.open(&tooling.Env{SessionBrowser: state.SessionBrowser}); err == nil {
					return fmt.Errorf("released session let a late tool call start Chrome")
				}
				return nil
			})
		},
	}
	if suite.Run() != 0 {
		t.Fatal("browser restart feature failed")
	}
}
