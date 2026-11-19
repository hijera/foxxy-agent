package desktop

import "testing"

func TestNotificationWindowActivity(t *testing.T) {
	cases := []struct {
		name   string
		window uintptr
		state  notificationWindowState
		active bool
	}{
		{"foreground", 10, notificationWindowState{foreground: 10, rootOwner: 10}, true},
		{"owned dialog", 10, notificationWindowState{foreground: 20, rootOwner: 10}, true},
		{"another application", 10, notificationWindowState{foreground: 20, rootOwner: 20}, false},
		{"minimized", 10, notificationWindowState{foreground: 10, rootOwner: 10, minimized: true}, false},
		{"no foreground", 10, notificationWindowState{}, false},
		{"closed", 0, notificationWindowState{}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.active(tt.window); got != tt.active {
				t.Errorf("window activity = %v, want %v", got, tt.active)
			}
		})
	}
}
