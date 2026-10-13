//go:build desktop && windows

package desktop

import "testing"

func TestNativeNotificationWindowActivity(t *testing.T) {
	if nativeNotificationWindowActive(0) {
		t.Fatal("a missing window must not be active")
	}
	foreground, _, _ := user32DLL.NewProc("GetForegroundWindow").Call()
	if foreground == 0 {
		t.Skip("no interactive foreground window")
	}
	active := nativeNotificationWindowActive(foreground)
	after, _, _ := user32DLL.NewProc("GetForegroundWindow").Call()
	if after != foreground {
		t.Skip("foreground changed during the native check")
	}
	if !active {
		t.Fatal("Win32 foreground window was not recognized as active")
	}
}
