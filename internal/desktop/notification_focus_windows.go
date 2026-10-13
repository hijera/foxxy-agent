//go:build desktop && windows

package desktop

import "syscall"

var user32DLL = syscall.NewLazyDLL("user32.dll")

func nativeNotificationWindowActive(window uintptr) bool {
	foreground, _, _ := user32DLL.NewProc("GetForegroundWindow").Call()
	owner, _, _ := user32DLL.NewProc("GetAncestor").Call(foreground, 3) // GA_ROOTOWNER
	minimized, _, _ := user32DLL.NewProc("IsIconic").Call(window)
	return (notificationWindowState{
		foreground: foreground, rootOwner: owner, minimized: minimized != 0,
	}).active(window)
}
