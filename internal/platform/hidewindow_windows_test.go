//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The desktop shell owns no console, so Windows gives each console child one of
// its own - with a window and a taskbar button. Both flags have to be asked for:
// CREATE_NO_WINDOW is what keeps the console windowless, HideWindow covers a
// child that opens a window itself.
func TestHideConsoleWindowAsksForAConsoleWithoutAWindow(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit")
	hideConsoleWindow(cmd, false)

	if cmd.SysProcAttr == nil {
		t.Fatal("hideConsoleWindow() left SysProcAttr nil for a process that owns no console")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("hideConsoleWindow() did not set HideWindow")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NO_WINDOW (%#x) set", cmd.SysProcAttr.CreationFlags, windows.CREATE_NO_WINDOW)
	}
}

// A run from a terminal must keep behaving exactly as before: the child inherits
// the console it is meant to inherit, and nothing flashes there anyway. Giving
// it a private console instead would cut it off from the terminal the operator
// is watching.
func TestHideConsoleWindowLeavesAnInheritedConsoleAlone(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit")
	hideConsoleWindow(cmd, true)

	if cmd.SysProcAttr != nil {
		t.Fatalf("hideConsoleWindow() touched SysProcAttr (%+v) for a process that owns a console", cmd.SysProcAttr)
	}
}

// run_command and the background pool need both attributes: the process group so
// stopping a task reaches the whole tree, and the windowless console so starting
// one does not blink. Whichever is applied second must not drop the first.
func TestHideConsoleWindowKeepsTheProcessGroupFlag(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit")
	DetachProcessGroup(cmd)
	hideConsoleWindow(cmd, false)

	flags := cmd.SysProcAttr.CreationFlags
	if flags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP kept", flags)
	}
	if flags&windows.CREATE_NO_WINDOW == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NO_WINDOW added", flags)
	}
}

// Spawn sites call this on whatever exec.Command returned, and that is nil when
// the command could not be built. Reporting the real error is the caller's job,
// not this helper's.
func TestHideConsoleWindowIgnoresANilCommand(t *testing.T) {
	hideConsoleWindow(nil, false)
}

// The flags are only worth anything if the child really comes up windowless.
// This starts one and asks it, because the same STARTUPINFO can be read by
// Windows as a request for a hidden console or as a request for none at all.
func TestHiddenChildHasNoConsoleWindow(t *testing.T) {
	cmd := consoleProbeCommand(t)
	hideConsoleWindow(cmd, false)

	if hwnd := runConsoleProbe(t, cmd); hwnd != 0 {
		t.Fatalf("child console window = %#x, want none", hwnd)
	}
}

// The contrast case, and the bug itself: without the flags a child of a process
// that owns a console inherits it rather than opening one. That inheritance is
// what a console run relies on and what this fix must not take away - the
// desktop shell simply has no console to pass on, which is why it gets the
// flags instead.
func TestPlainChildInheritsThisConsoleWindow(t *testing.T) {
	if !hasConsoleWindow() {
		t.Skip("this test process owns no console window to inherit")
	}
	cmd := consoleProbeCommand(t)

	if hwnd := runConsoleProbe(t, cmd); hwnd == 0 {
		t.Fatal("child reported no console window, want the one this process owns")
	}
}

// consoleProbeCommand re-runs this test binary as the probe below. A helper
// process is the only way to read a child's console: the answer lives in the
// child, and nothing about it is visible from here.
func consoleProbeCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	cmd := exec.Command(exe, "-test.run", "^TestConsoleProbeHelper$") // #nosec G204 -- the test binary itself
	cmd.Env = append(os.Environ(), consoleProbeEnv+"=1")
	return cmd
}

// runConsoleProbe runs the probe and returns the console window handle it saw.
func runConsoleProbe(t *testing.T, cmd *exec.Cmd) uintptr {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("console probe: %v: %s", err, out)
	}
	const marker = consoleProbeMarker
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		var hwnd uintptr
		if _, err := fmt.Sscanf(strings.TrimPrefix(line, marker), "%d", &hwnd); err != nil {
			t.Fatalf("unreadable probe line %q: %v", line, err)
		}
		return hwnd
	}
	t.Fatalf("console probe printed no %q line: %s", marker, out)
	return 0
}

const (
	consoleProbeEnv    = "FOXXYCODE_CONSOLE_PROBE"
	consoleProbeMarker = "console-window="
)

// TestConsoleProbeHelper is not a test of its own: it is the child half of the
// two tests above, and it does nothing unless they started it.
func TestConsoleProbeHelper(t *testing.T) {
	if os.Getenv(consoleProbeEnv) != "1" {
		t.Skip("child half of the console window tests")
	}
	hwnd, _, _ := procGetConsoleWindow.Call()
	fmt.Printf("%s%d\n", consoleProbeMarker, hwnd)
}
