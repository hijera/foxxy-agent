package config

import "testing"

func TestToolBackgroundResolvedEnabledDefaultsToTrue(t *testing.T) {
	var b *ToolBackground
	if !b.ResolvedEnabled() {
		t.Fatalf("nil section should read as enabled")
	}
	b = &ToolBackground{}
	if !b.ResolvedEnabled() {
		t.Fatalf("unset enabled should read as enabled")
	}
	off := false
	b.Enabled = &off
	if b.ResolvedEnabled() {
		t.Fatalf("enabled: false should read as disabled")
	}
}

func TestToolBackgroundResolvedFillsDefaults(t *testing.T) {
	got := (&ToolBackground{}).Resolved()
	if got.MaxConcurrent != BackgroundDefaultMaxConcurrent {
		t.Fatalf("max_concurrent = %d, want %d", got.MaxConcurrent, BackgroundDefaultMaxConcurrent)
	}
	if got.DefaultTimeoutSeconds != BackgroundDefaultTimeoutSeconds {
		t.Fatalf("default_timeout_seconds = %d, want %d", got.DefaultTimeoutSeconds, BackgroundDefaultTimeoutSeconds)
	}
	if got.MaxTimeoutSeconds != BackgroundDefaultMaxTimeoutSeconds {
		t.Fatalf("max_timeout_seconds = %d, want %d", got.MaxTimeoutSeconds, BackgroundDefaultMaxTimeoutSeconds)
	}
	if got.OutputBufferBytes != BackgroundDefaultOutputBufferBytes {
		t.Fatalf("output_buffer_bytes = %d, want %d", got.OutputBufferBytes, BackgroundDefaultOutputBufferBytes)
	}
}

// A default longer than the ceiling would let every estimate-free task outlive
// the limit the operator set, so Resolved clamps it.
func TestToolBackgroundResolvedClampsDefaultToMaxTimeout(t *testing.T) {
	got := (&ToolBackground{DefaultTimeoutSeconds: 5000, MaxTimeoutSeconds: 120}).Resolved()
	if got.DefaultTimeoutSeconds != 120 {
		t.Fatalf("default_timeout_seconds = %d, want it clamped to 120", got.DefaultTimeoutSeconds)
	}
}

func TestToolsValidateRejectsNegativeBackgroundKnobs(t *testing.T) {
	cases := map[string]Tools{
		"max_concurrent":          {Background: ToolBackground{MaxConcurrent: -1}},
		"default_timeout_seconds": {Background: ToolBackground{DefaultTimeoutSeconds: -1}},
		"max_timeout_seconds":     {Background: ToolBackground{MaxTimeoutSeconds: -1}},
		"output_buffer_bytes":     {Background: ToolBackground{OutputBufferBytes: -1}},
	}
	for name, tools := range cases {
		t.Run(name, func(t *testing.T) {
			if err := tools.Validate(); err == nil {
				t.Fatalf("negative %s accepted", name)
			}
		})
	}
}

// Zero keeps meaning "use the default", so it must not be rejected.
func TestToolsValidateAcceptsZeroBackgroundKnobs(t *testing.T) {
	tools := Tools{}
	if err := tools.Validate(); err != nil {
		t.Fatalf("zeroed background section rejected: %v", err)
	}
}

func TestBackgroundSurvivesJSONDTORoundTrip(t *testing.T) {
	off := false
	cfg := &Config{Tools: Tools{Background: ToolBackground{
		Enabled:               &off,
		MaxConcurrent:         3,
		DefaultTimeoutSeconds: 60,
		MaxTimeoutSeconds:     120,
		OutputBufferBytes:     4096,
	}}}

	back := JSONDTOToConfig(ConfigToJSONDTO(cfg), Paths{}).Tools.Background
	if back.Enabled == nil || *back.Enabled {
		t.Fatalf("enabled lost in round trip: %v", back.Enabled)
	}
	if back.MaxConcurrent != 3 || back.DefaultTimeoutSeconds != 60 ||
		back.MaxTimeoutSeconds != 120 || back.OutputBufferBytes != 4096 {
		t.Fatalf("knobs lost in round trip: %+v", back)
	}
}
