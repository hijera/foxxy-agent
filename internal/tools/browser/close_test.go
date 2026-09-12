//go:build browser

package browser

import (
	"os"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/platform"
)

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
