//go:build !windows

package platform

import (
	"os/exec"
	"testing"
)

// Nothing outside Windows hands a child a window, so the helper must leave the
// command exactly as the caller built it - a spawn attribute set here would be
// one nobody asked for.
func TestHideConsoleWindowIsANoOpOutsideWindows(t *testing.T) {
	cmd := exec.Command("echo", "hi")
	HideConsoleWindow(cmd)

	if cmd.SysProcAttr != nil {
		t.Fatalf("HideConsoleWindow() set SysProcAttr = %+v, want it untouched", cmd.SysProcAttr)
	}
	HideConsoleWindow(nil)
}
