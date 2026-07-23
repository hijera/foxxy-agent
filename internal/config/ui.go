package config

import (
	"fmt"
	"strings"
)

// UI send_mode values: how the main composer submits a message.
const (
	// UISendModeEnter sends on plain Enter (Shift/Ctrl+Enter insert a newline).
	UISendModeEnter = "enter"
	// UISendModeCtrlEnter sends on Ctrl/Cmd+Enter (plain Enter inserts a newline).
	UISendModeCtrlEnter = "ctrl_enter"
	// UISendModeOff disables keyboard send; the Send button is the only way.
	UISendModeOff = "off"
)

// UIConfig holds embedded SPA preferences (locale, send mode, etc.).
type UIConfig struct {
	// Enabled toggles serving the embedded SPA at GET / (http,ui builds). A nil pointer means the
	// default (true). Set false to run foxxycode http as an API-only server (no SPA); /v1/* and
	// /foxxycode/* stay available. Use IsEnabled to read the effective value.
	Enabled *bool `yaml:"enabled" json:"enabled,omitempty"`
	// Locale is the UI language: empty (auto-detect), "en", or "ru".
	Locale string `yaml:"locale" json:"locale"`
	// SendMode controls how the main composer submits: "enter" (default),
	// "ctrl_enter", or "off".
	SendMode string `yaml:"send_mode" json:"send_mode"`
}

// IsEnabled reports whether the embedded SPA is served. Unset (nil) defaults to true.
func (u *UIConfig) IsEnabled() bool {
	return u.Enabled == nil || *u.Enabled
}

// Normalize trims locale and normalizes send_mode (empty -> "enter").
func (u *UIConfig) Normalize() {
	u.Locale = strings.TrimSpace(u.Locale)
	u.SendMode = strings.ToLower(strings.TrimSpace(u.SendMode))
	if u.SendMode == "" {
		u.SendMode = UISendModeEnter
	}
}

// ApplyDefaults leaves empty locale as auto-detect.
func (u *UIConfig) ApplyDefaults() {}

// Validate checks ui.locale and ui.send_mode.
func (u *UIConfig) Validate() error {
	switch u.Locale {
	case "", "en", "ru":
	default:
		return fmt.Errorf("ui.locale must be empty, en, or ru (got %q)", u.Locale)
	}
	switch u.SendMode {
	case "", UISendModeEnter, UISendModeCtrlEnter, UISendModeOff:
		return nil
	default:
		return fmt.Errorf("ui.send_mode must be enter, ctrl_enter, or off (got %q)", u.SendMode)
	}
}
