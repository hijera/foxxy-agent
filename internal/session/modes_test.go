package session

import "testing"

func TestIsValidMode(t *testing.T) {
	for _, m := range []string{"agent", "plan", "docs", "ask", "debug"} {
		if !IsValidMode(m) {
			t.Errorf("IsValidMode(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"", "AGENT", "code", "orchestrator", "unknown"} {
		if IsValidMode(m) {
			t.Errorf("IsValidMode(%q) = true, want false", m)
		}
	}
}

// Debug is a full-access mode like agent; SetMode/GetMode round-trip it.
func TestSetGetModeDebug(t *testing.T) {
	st := &State{}
	st.SetMode("debug")
	if got := st.GetMode(); got != "debug" {
		t.Fatalf("GetMode() = %q, want debug", got)
	}
	if st.Mode != ModeDebug {
		t.Errorf("State.Mode = %q, want %q", st.Mode, ModeDebug)
	}
}
