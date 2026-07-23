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
	if c.Engine != config.CompactionEngineCoddy {
		t.Errorf("Engine = %q, want %q (default)", c.Engine, config.CompactionEngineCoddy)
	}
	if c.ThresholdPercent != config.CompactionDefaultThresholdCoddy {
		t.Errorf("ThresholdPercent = %d, want %d (coddy default)", c.ThresholdPercent, config.CompactionDefaultThresholdCoddy)
	}
	if got := c.EffectiveKeepRecentTurns(); got != config.CompactionDefaultKeepRecentTurns {
		t.Errorf("EffectiveKeepRecentTurns = %d, want %d", got, config.CompactionDefaultKeepRecentTurns)
	}
	if c.MaxTokens != config.CompactionDefaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", c.MaxTokens, config.CompactionDefaultMaxTokens)
	}
}

func TestCompactionOpenCodeDefaults(t *testing.T) {
	c := config.CompactionConfig{Engine: "opencode"}
	c.Normalize()
	c.ApplyDefaults()

	if !c.EngineIsOpenCode() {
		t.Fatal("EngineIsOpenCode should be true for engine: opencode")
	}
	if c.ThresholdPercent != config.CompactionDefaultThresholdOpenCode {
		t.Errorf("ThresholdPercent = %d, want %d (opencode default)", c.ThresholdPercent, config.CompactionDefaultThresholdOpenCode)
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

func TestCompactionOpenCodeThresholdClamped(t *testing.T) {
	low := config.CompactionConfig{Engine: "opencode", ThresholdPercent: 10}
	low.ApplyDefaults()
	if low.ThresholdPercent != 50 {
		t.Errorf("low ThresholdPercent = %d, want clamp to 50", low.ThresholdPercent)
	}

	high := config.CompactionConfig{Engine: "opencode", ThresholdPercent: 250}
	high.ApplyDefaults()
	if high.ThresholdPercent != 99 {
		t.Errorf("high ThresholdPercent = %d, want clamp to 99", high.ThresholdPercent)
	}
}

func TestCompactionCoddyThresholdNotClamped(t *testing.T) {
	// The coddy engine accepts the full 1..100 range; only opencode clamps to 50..99.
	c := config.CompactionConfig{ThresholdPercent: 10}
	c.ApplyDefaults()
	if c.ThresholdPercent != 10 {
		t.Errorf("coddy ThresholdPercent = %d, want unclamped 10", c.ThresholdPercent)
	}
}

func TestCompactionKeepRecentExplicitZero(t *testing.T) {
	zero := 0
	c := config.CompactionConfig{KeepRecentTurns: &zero}
	c.ApplyDefaults()
	if got := c.EffectiveKeepRecentTurns(); got != 0 {
		t.Errorf("EffectiveKeepRecentTurns = %d, want explicit 0", got)
	}
}

func TestCompactionValidateEngine(t *testing.T) {
	cfg := &config.Config{Compaction: config.CompactionConfig{Engine: "bogus"}}
	cfg.Compaction.Normalize()
	cfg.Compaction.ApplyDefaults()
	if err := cfg.Compaction.Validate(cfg); err == nil {
		t.Error("expected error for unknown compaction.engine")
	}
}

func TestCompactionValidateModelReference(t *testing.T) {
	on := true
	cfg := &config.Config{
		Compaction: config.CompactionConfig{Enabled: &on, Model: "missing/model"},
	}
	cfg.Compaction.ApplyDefaults()
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
