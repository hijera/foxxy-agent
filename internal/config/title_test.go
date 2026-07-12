package config_test

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestTitleDefaults(t *testing.T) {
	var c config.TitleConfig
	c.Normalize()
	c.ApplyDefaults()

	if !c.TitleEnabled() {
		t.Error("title should default to enabled")
	}
	if c.MaxTokens != config.TitleDefaultMaxTokens {
		t.Errorf("MaxTokens = %d, want %d", c.MaxTokens, config.TitleDefaultMaxTokens)
	}
}

func TestTitleExplicitDisablePreserved(t *testing.T) {
	off := false
	c := config.TitleConfig{Enabled: &off}
	c.ApplyDefaults()
	if c.TitleEnabled() {
		t.Error("explicit title.enabled=false must be preserved")
	}
}

func TestTitleValidateModelReference(t *testing.T) {
	on := true
	cfg := &config.Config{
		Title: config.TitleConfig{Enabled: &on, Model: "missing/model"},
	}
	if err := cfg.Title.Validate(cfg); err == nil {
		t.Error("expected error for unknown title.model")
	}

	// Disabled title generation should not validate the model reference.
	off := false
	cfg.Title.Enabled = &off
	if err := cfg.Title.Validate(cfg); err != nil {
		t.Errorf("disabled title should skip model validation, got %v", err)
	}
}
