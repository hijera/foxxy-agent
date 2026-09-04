package llm

import "testing"

// TestSetDebugCapture verifies the process-wide capture switch flips atomically. The
// HTTP server's ReplaceConfig calls SetDebugCapture on PUT /foxxycode/config so the
// debug.enabled toggle takes effect without a restart.
func TestSetDebugCapture(t *testing.T) {
	// Reset to a known state (other tests may have touched it).
	SetDebugCapture(false)
	if DebugCaptureEnabled() {
		t.Fatal("expected capture disabled by default")
	}
	SetDebugCapture(true)
	if !DebugCaptureEnabled() {
		t.Fatal("expected capture enabled after SetDebugCapture(true)")
	}
	SetDebugCapture(false)
	if DebugCaptureEnabled() {
		t.Fatal("expected capture disabled after SetDebugCapture(false)")
	}
}
