package config

import "flag"

// DebugFlagName is the CLI flag (on `foxxycode acp` / `http` / `gateway` / `desktop`)
// that forces debug.enabled=true in this process.
const DebugFlagName = "debug"

// ApplyDebugFlag forces debug.enabled=true only when the -debug flag was explicitly
// provided on fs; otherwise the config value is left untouched. Mirrors
// ApplyPlanNoSelfRunFlag so a default-false flag never silently disables a
// config-enabled debug layer.
func ApplyDebugFlag(fs *flag.FlagSet, cfg *Config, val *bool) {
	if fs == nil || cfg == nil || val == nil {
		return
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == DebugFlagName && *val {
			cfg.Debug.Enabled = true
		}
	})
}

// Debug is the YAML debug/diagnostics section (key debug).
//
// It is the master switch for verbose diagnostics: when Enabled, the process
// logger runs at debug level, raw LLM HTTP request/response bodies are captured
// (unless CaptureLLM is explicitly false), and per-session debug trace events
// are emitted and persisted. The CLI flag --debug forces Enabled at startup;
// PUT /foxxycode/config toggles it at runtime (the logger level is switched in
// place via a slog.LevelVar, without rebuilding the logger).
type Debug struct {
	// Enabled turns the whole diagnostics layer on.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// CaptureLLM gates raw LLM HTTP request/response logging. It is a pointer so
	// an unset value (the default) means "follow Enabled" rather than false: when
	// Enabled is true and CaptureLLM is nil, raw capture is on. Set it explicitly
	// to false to keep debug logs while suppressing potentially large bodies.
	CaptureLLM *bool `yaml:"capture_llm" json:"capture_llm"`
}

// ApplyDefaults normalises the debug section in place. Today there is nothing to
// default (CaptureLLM == nil already means "follow Enabled"), but the hook keeps
// future knobs consistent with the other config sections.
func (c *Debug) ApplyDefaults() {}

// Validate reports whether the debug section is well-formed. No constraints today.
func (c *Debug) Validate() error { return nil }

// Effective reports whether the diagnostics layer is on.
func (c Debug) Effective() bool { return c.Enabled }

// EffectiveCapture reports whether raw LLM HTTP bodies should be logged. Capture
// follows Enabled unless an operator explicitly disabled it.
func (c Debug) EffectiveCapture() bool {
	if !c.Enabled {
		return false
	}
	if c.CaptureLLM == nil {
		return true
	}
	return *c.CaptureLLM
}
