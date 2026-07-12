package config

import (
	"fmt"
	"strings"
)

// Default values for CompactionConfig.
const (
	CompactionDefaultThresholdPercent = 85
	CompactionDefaultKeepLastTurns    = 2
	CompactionDefaultMaxTokens        = 4096

	compactionMinThresholdPercent = 50
	compactionMaxThresholdPercent = 99
)

// CompactionConfig controls automatic context compaction (summarization of older turns when the
// conversation approaches the model's context window). Implementation lives in internal/agent.
type CompactionConfig struct {
	// Enabled is a pointer so an unset value defaults to true while an explicit false is
	// preserved. Use CompactionEnabled to read the effective value. Auto-compaction only fires
	// when real prompt tokens approach the context window, so leaving it on is a safe default.
	Enabled *bool `yaml:"enabled"`

	// Model selects a cfg.models entry for the summarization pass. Empty uses agent.model.
	Model string `yaml:"model"`

	// ThresholdPercent triggers compaction when real prompt tokens exceed this percentage of the
	// usable context (max_context_tokens - max_tokens). Default 85, clamped to [50, 99].
	ThresholdPercent int `yaml:"threshold_percent"`

	// KeepLastTurns is the number of most recent user turns preserved verbatim. Default 2, min 1.
	KeepLastTurns int `yaml:"keep_last_turns"`

	// MaxTokens caps the summary completion size. Default 4096.
	MaxTokens int `yaml:"max_tokens"`
}

// CompactionEnabled reports whether auto-compaction is active. Unset (nil) defaults to true.
func (c *CompactionConfig) CompactionEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// Normalize trims string fields in place.
func (c *CompactionConfig) Normalize() {
	c.Model = strings.TrimSpace(c.Model)
}

// ApplyDefaults sets zero values to safe defaults. An omitted enabled key defaults to true.
func (c *CompactionConfig) ApplyDefaults() {
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.ThresholdPercent <= 0 {
		c.ThresholdPercent = CompactionDefaultThresholdPercent
	}
	if c.ThresholdPercent < compactionMinThresholdPercent {
		c.ThresholdPercent = compactionMinThresholdPercent
	}
	if c.ThresholdPercent > compactionMaxThresholdPercent {
		c.ThresholdPercent = compactionMaxThresholdPercent
	}
	if c.KeepLastTurns < 1 {
		c.KeepLastTurns = CompactionDefaultKeepLastTurns
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = CompactionDefaultMaxTokens
	}
}

// Validate checks the compaction model reference resolves when set.
func (c *CompactionConfig) Validate(cfg *Config) error {
	if !c.CompactionEnabled() {
		return nil
	}
	if c.Model != "" && cfg.FindModelEntry(c.Model) == nil {
		return fmt.Errorf("model %q: not found in models list", c.Model)
	}
	return nil
}
