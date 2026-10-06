//go:build windows

package platform

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// HideConsoleWindow keeps a console child from opening a window of its own.
//
// The desktop shell is linked with -H=windowsgui and so owns no console. Windows
// therefore hands every console program it starts a brand new console, and that
// console arrives with a visible window and a taskbar button: git for the
// workspace chips, ripgrep behind a search tool, the shell behind run_command, an
// MCP stdio server. A turn starts those in bursts, so the operator watches a row
// of windows blink open and shut. CREATE_NO_WINDOW asks for the console without
// the window - the child keeps a console, so the code page DecodeOutput reads is
// unchanged - and HideWindow covers a child that opens a window of its own.
//
// It is deliberately a no-op when this process already owns a console window: the
// child inherits that console, nothing flashes, and giving it a private console
// instead would cut it off from the terminal an operator is watching.
func HideConsoleWindow(cmd *exec.Cmd) {
	hideConsoleWindow(cmd, hasConsoleWindow())
}

// hideConsoleWindow is HideConsoleWindow with the console check hoisted out so a
// test can drive both hosts, the windowless desktop shell and a console run.
func hideConsoleWindow(cmd *exec.Cmd, ownsConsole bool) {
	if cmd == nil || ownsConsole {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// procGetConsoleWindow is resolved lazily; x/sys/windows does not export it.
var procGetConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")

// hasConsoleWindow reports whether this process owns a console window. A GUI
// subsystem binary started from Explorer has none; the same binary started from a
// terminal inherits that terminal's console, and so does `foxxycode http` or the
// console UI, which is why this is asked per spawn rather than per platform.
func hasConsoleWindow() bool {
	if err := procGetConsoleWindow.Find(); err != nil {
		return false
	}
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd != 0
}
