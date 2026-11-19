//go:build desktop && windows

package desktop

import (
	"syscall"
	"unsafe"
)

var (
	user32DLL   = syscall.NewLazyDLL("user32.dll")
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")

	procGetForegroundWindow      = user32DLL.NewProc("GetForegroundWindow")
	procGetAncestor              = user32DLL.NewProc("GetAncestor")
	procIsIconic                 = user32DLL.NewProc("IsIconic")
	procShowWindow               = user32DLL.NewProc("ShowWindow")
	procSetForegroundWindow      = user32DLL.NewProc("SetForegroundWindow")
	procGetWindowThreadProcessId = user32DLL.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadId       = kernel32DLL.NewProc("GetCurrentThreadId")
	procAttachThreadInput        = user32DLL.NewProc("AttachThreadInput")
	procFlashWindowEx            = user32DLL.NewProc("FlashWindowEx")
)

func nativeNotificationWindowActive(window uintptr) bool {
	foreground, _, _ := procGetForegroundWindow.Call()
	owner, _, _ := procGetAncestor.Call(foreground, 3) // GA_ROOTOWNER
	minimized, _, _ := procIsIconic.Call(window)
	return (notificationWindowState{
		foreground: foreground, rootOwner: owner, minimized: minimized != 0,
	}).active(window)
}

const (
	swRestore       = 9
	flashwAll       = 0x3
	flashwTimerNoFG = 0xc
)

type flashWindowInfo struct {
	size    uint32
	hwnd    uintptr
	flags   uint32
	count   uint32
	timeout uint32
}

// bringWindowToFront raises the chat window when a toast is clicked.
// SetForegroundWindow is denied while another process owns the foreground, and
// the WinRT activation callback does not run on the process's input thread; the
// attach-input dance lifts that restriction, and a taskbar flash keeps the
// signal when even that fails, so a granted permission is never fully invisible.
func bringWindowToFront(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	_, _, _ = procShowWindow.Call(hwnd, swRestore)
	ok, _, _ := procSetForegroundWindow.Call(hwnd)
	if ok == 0 {
		if foreign, _, _ := procGetForegroundWindow.Call(); foreign != 0 && foreign != hwnd {
			var foreignThread uintptr
			us, _, _ := procGetCurrentThreadId.Call()
			them, _, _ := procGetWindowThreadProcessId.Call(foreign, uintptr(unsafe.Pointer(&foreignThread)))
			if us != 0 && them != 0 {
				_, _, _ = procAttachThreadInput.Call(them, us, 1)
				_, _, _ = procAttachThreadInput.Call(us, them, 1)
				_, _, _ = procSetForegroundWindow.Call(hwnd)
				_, _, _ = procAttachThreadInput.Call(them, us, 0)
				_, _, _ = procAttachThreadInput.Call(us, them, 0)
			}
		}
	}
	if foreground, _, _ := procGetForegroundWindow.Call(); foreground != hwnd {
		info := flashWindowInfo{
			size:    uint32(unsafe.Sizeof(flashWindowInfo{})),
			hwnd:    hwnd,
			flags:   flashwAll | flashwTimerNoFG,
			count:   3,
			timeout: 0,
		}
		_, _, _ = procFlashWindowEx.Call(uintptr(unsafe.Pointer(&info)))
	}
}
