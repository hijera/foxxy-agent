package config_test

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestCompactionDefaults(t *testing.T) {
	var c config.CompactionConfig
	c.Normalize()
	c.ApplyDefaults()

	if !c.CompactionEnabled() {
		t.Error("compaction should default to enabled")
	}
	if c.ThresholdPercent != config.CompactionDefaultThresholdPercent {
		t.Errorf("ThresholdPercent = %d, want %d", c.ThresholdPercent, config.CompactionDefaultThresholdPercent)
	}
	if c.KeepLastTurns != config.CompactionDefaultKeepLastTurns {
		t.Errorf("KeepLastTurns = %d, want %d", c.KeepLastTurns, config.CompactionDefaultKeepLastTurns)
	}
	if c.MaxTokens != config.CompactionDefaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", c.MaxTokens, config.CompactionDefaultMaxTokens)
	}
}

func TestCompactionExplicitDisablePreserved(t *testing.T) {
	off := false
	c := config.CompactionConfig{Enabled: &off}
	c.ApplyDefaults()
	if c.CompactionEnabled() {
		t.Error("explicit compaction.enabled=false must be preserved")
	}
}

func TestCompactionThresholdClamped(t *testing.T) {
	low := config.CompactionConfig{ThresholdPercent: 10}
	low.ApplyDefaults()
	if low.ThresholdPercent != 50 {
		t.Errorf("low ThresholdPercent = %d, want clamp to 50", low.ThresholdPercent)
	}

	high := config.CompactionConfig{ThresholdPercent: 250}
	high.ApplyDefaults()
	if high.ThresholdPercent != 99 {
		t.Errorf("high ThresholdPercent = %d, want clamp to 99", high.ThresholdPercent)
	}
}

func TestCompactionValidateModelReference(t *testing.T) {
	on := true
	cfg := &config.Config{
		Compaction: config.CompactionConfig{Enabled: &on, Model: "missing/model"},
	}
	if err := cfg.Compaction.Validate(cfg); err == nil {
		t.Error("expected error for unknown compaction.model")
	}

	// Disabled compaction should not validate the model reference.
	off := false
	cfg.Compaction.Enabled = &off
	if err := cfg.Compaction.Validate(cfg); err != nil {
		t.Errorf("disabled compaction should skip model validation, got %v", err)
	}
}
