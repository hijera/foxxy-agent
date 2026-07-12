package config

import (
	"fmt"
	"strings"
)

// Default values for TitleConfig.
const (
	// TitleDefaultMaxTokens caps the title completion. Titles are a single short phrase, so this
	// is intentionally small.
	TitleDefaultMaxTokens = 64
)

// TitleConfig controls automatic LLM session-title generation. After the first exchange in a fresh,
// non-pinned session, a hidden internal "title" agent produces a short thread title. The agent is
// locked: users may tune the fields below, but the prompt is embedded and the profile is not
// selectable as a chat mode. Implementation lives in internal/agent. See also CompactionConfig.
type TitleConfig struct {
	// Enabled is a pointer so an unset value defaults to true while an explicit false is
	// preserved. Use TitleEnabled to read the effective value.
	Enabled *bool `yaml:"enabled"`

	// Model selects a cfg.models entry for the title pass. Empty uses agent.model. A small, cheap
	// model is a good choice here.
	Model string `yaml:"model"`

	// MaxTokens caps the title completion size. Default 64.
	MaxTokens int `yaml:"max_tokens"`
}

// TitleEnabled reports whether auto-title generation is active. Unset (nil) defaults to true.
func (c *TitleConfig) TitleEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// Normalize trims string fields in place.
func (c *TitleConfig) Normalize() {
	c.Model = strings.TrimSpace(c.Model)
}

// ApplyDefaults sets zero values to safe defaults. An omitted enabled key defaults to true.
func (c *TitleConfig) ApplyDefaults() {
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = TitleDefaultMaxTokens
	}
}

// Validate checks the title model reference resolves when set.
func (c *TitleConfig) Validate(cfg *Config) error {
	if !c.TitleEnabled() {
		return nil
	}
	if c.Model != "" && cfg.FindModelEntry(c.Model) == nil {
		return fmt.Errorf("model %q: not found in models list", c.Model)
	}
	return nil
}
