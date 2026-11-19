package desktop

// notificationWindowState also counts owned dialogs as part of the app window.
type notificationWindowState struct {
	foreground uintptr
	rootOwner  uintptr
	minimized  bool
}

func (s notificationWindowState) active(window uintptr) bool {
	return window != 0 && s.foreground != 0 && !s.minimized &&
		(s.foreground == window || s.rootOwner == window)
}

func (n Notification) suppressed(windowActive bool) bool {
	return windowActive && !n.WhenFocused
}
