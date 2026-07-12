package config_test

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestPromptsPerProviderDefaultsToEnabled(t *testing.T) {
	var p config.Prompts
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !p.PerProviderEnabled() {
		t.Error("per-provider prompts should default to enabled")
	}
}

func TestPromptsPerProviderExplicitFalsePreserved(t *testing.T) {
	off := false
	p := config.Prompts{PerProvider: config.PerProviderPrompts{Enabled: &off}}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if p.PerProviderEnabled() {
		t.Error("explicit per_provider.enabled=false must be preserved")
	}
}
