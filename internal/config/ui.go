package config

import (
	"fmt"
	"strings"
)

// UIConfig holds embedded SPA preferences (locale, etc.).
type UIConfig struct {
	// Locale is the UI language: empty (auto-detect), "en", or "ru".
	Locale string `yaml:"locale" json:"locale"`
}

// Normalize trims locale.
func (u *UIConfig) Normalize() {
	u.Locale = strings.TrimSpace(u.Locale)
}

// ApplyDefaults leaves empty locale as auto-detect.
func (u *UIConfig) ApplyDefaults() {}

// Validate checks ui.locale is empty, en, or ru.
func (u *UIConfig) Validate() error {
	switch u.Locale {
	case "", "en", "ru":
		return nil
	default:
		return fmt.Errorf("ui.locale must be empty, en, or ru (got %q)", u.Locale)
	}
}
