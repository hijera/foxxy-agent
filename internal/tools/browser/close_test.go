//go:build browser

package browser

import (
	"os"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/platform"
)

// The root tab owns the browser context, while child tabs have their own
// cancellation functions. Closing that page must leave the other tab usable,
// and ending the session must still wait for the whole Chrome process tree.
func TestCloseWaitsAfterRootTabClosed(t *testing.T) {
	newTestManager(t)
	profile := profileDirFor(t.TempDir())
	b, err := launch(&config.BrowserConfig{Enabled: true}, profile)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			b.close()
		}
	})
	root := b.currentID
	id, err := b.newTab()
	if err != nil {
		t.Fatalf("new tab: %v", err)
	}
	if err := b.selectTab(id); err != nil {
		t.Fatalf("select child tab: %v", err)
	}
	if err := b.closeTab(root); err != nil {
		t.Fatalf("close root tab: %v", err)
	}
	var result int
	if err := b.run(chromedp.Evaluate("1 + 1", &result)); err != nil || result != 2 {
		t.Fatalf("child tab after root closes: result=%d, err=%v", result, err)
	}
	tree := platform.CaptureProcessTree(b.chromePID)
	defer tree.Release()
	b.close()
	closed = true
	if !tree.WaitExit(0) {
		t.Error("close returned while a Chrome child process was still running")
	}
	if err := os.RemoveAll(profile); err != nil {
		t.Errorf("profile is still held after closing multiple tabs: %v", err)
	}
}

// Closing a browser has to mean Chrome and everything it spawned are gone, not
// only the one process chromedp started. Chrome's renderers, GPU process and
// crashpad handler are separate processes that leave a moment later, and until
// the crashpad handler does it keeps CrashpadMetrics-active.pma mapped inside
// the profile directory - which on Windows makes that directory undeletable.
//
// The symptom was a test suite that cleaned up after itself fine on its own and
// failed to on a busy machine, because the race is with how long Chrome's
// children take to notice the browser is gone.
func TestCloseWaitsForEverythingChromeSpawned(t *testing.T) {
	newTestManager(t)

	profile := profileDirFor(t.TempDir())
	b, err := launch(&config.BrowserConfig{Enabled: true}, profile)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if b.chromePID == 0 {
		t.Fatal("launch recorded no Chrome process, so close has nothing to wait for")
	}

	// Captured while Chrome is still running, for the same reason close does it
	// there: once the leader is gone nothing can tell its children from a
	// stranger holding its pid.
	tree := platform.CaptureProcessTree(b.chromePID)
	defer tree.Release()

	b.close()

	// Zero grace: close is the thing that was supposed to have waited.
	if !tree.WaitExit(0) {
		t.Error("close() returned while a process Chrome had spawned was still running")
	}
	if err := os.RemoveAll(profile); err != nil {
		t.Errorf("the profile directory is still held after close(): %v", err)
	}
}

// The Chrome command is chromedp's, and the only hook onto it also replaces
// chromedp's own setup of it. Losing the pid is what silently turns close back
// into the version that did not wait, so it is worth one assertion of its own.
func TestLaunchRecordsTheChromeProcess(t *testing.T) {
	newTestManager(t)

	b, err := launch(&config.BrowserConfig{Enabled: true}, "")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer b.close()

	if b.chromePID <= 0 {
		t.Fatalf("chromePID = %d, want the pid of the browser chromedp started", b.chromePID)
	}
	if b.chromePID == os.Getpid() {
		t.Fatal("chromePID is this process, not the browser")
	}
}
