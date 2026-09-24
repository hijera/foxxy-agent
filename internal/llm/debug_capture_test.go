package llm

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// TestSetDebugCapture verifies the process-wide capture switch flips atomically. The
// HTTP server's ReplaceConfig calls ApplyDebugConfig on PUT /foxxycode/config so the
// debug.enable toggle takes effect without a restart.
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

// The connection trace carries no payloads, so it follows debug.enable alone:
// capture_llm: false silences the bodies, not the trace of a hanging request.
func TestApplyDebugConfig(t *testing.T) {
	off := false
	cases := []struct {
		name         string
		debug        config.Debug
		capture, net bool
	}{
		{"debug off", config.Debug{}, false, false},
		{"debug on", config.Debug{Enabled: true}, true, true},
		{"bodies off", config.Debug{Enabled: true, CaptureLLM: &off}, false, true},
	}
	defer ApplyDebugConfig(config.Debug{})
	for _, c := range cases {
		ApplyDebugConfig(c.debug)
		if DebugCaptureEnabled() != c.capture || NetTraceEnabled() != c.net {
			t.Errorf("%s: capture=%v trace=%v, want capture=%v trace=%v",
				c.name, DebugCaptureEnabled(), NetTraceEnabled(), c.capture, c.net)
		}
	}
}
