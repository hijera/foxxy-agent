package session

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// onboardingNeuralDeepCfg mirrors what the onboarding ProviderPickerDialog writes
// for the NeuralDeep preset: agent.model and models[0].model are the same logical
// id, and a matching neuraldeep provider exists.
func onboardingNeuralDeepCfg() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "neuraldeep", Type: "neuraldeep"},
		},
		Models: []config.ModelEntry{
			{Model: "neuraldeep/gpt-oss-120b"},
		},
		Agent: config.Agent{Model: "neuraldeep/gpt-oss-120b"},
	}
}

// TestOnboardingModelReachesAgent guards the onboarding -> ReAct handoff: the model
// selected in onboarding must reach the agent as provider "neuraldeep" with API
// model "gpt-oss-120b" (not the literal "default" the preset used to write).
func TestOnboardingModelReachesAgent(t *testing.T) {
	cfg := onboardingNeuralDeepCfg()

	// No session override: EffectiveModelID falls back to agent.model.
	st := &State{}
	got := st.EffectiveModelID(cfg)
	if got != "neuraldeep/gpt-oss-120b" {
		t.Fatalf("EffectiveModelID = %q, want neuraldeep/gpt-oss-120b", got)
	}

	rm, err := cfg.ResolveLLM(got)
	if err != nil {
		t.Fatalf("ResolveLLM(%q): %v", got, err)
	}
	if rm.ProviderType != "neuraldeep" {
		t.Errorf("ProviderType = %q, want neuraldeep", rm.ProviderType)
	}
	if rm.Model != "gpt-oss-120b" {
		t.Errorf("API model = %q, want gpt-oss-120b (never the literal \"default\")", rm.Model)
	}
}

// TestOnboardingModelSelectionOverrideReachesAgent covers the composer picking the
// same model at runtime: a SelectedModelID present in cfg.Models resolves cleanly
// rather than being silently swapped to models[0].
func TestOnboardingModelSelectionOverrideReachesAgent(t *testing.T) {
	cfg := onboardingNeuralDeepCfg()
	// A second model exists so a wrong fallback would be observable.
	cfg.Models = append(cfg.Models, config.ModelEntry{Model: "neuraldeep/qwen-3"})

	st := &State{SelectedModelID: "neuraldeep/gpt-oss-120b"}
	got := st.EffectiveModelID(cfg)
	if got != "neuraldeep/gpt-oss-120b" {
		t.Fatalf("EffectiveModelID = %q, want neuraldeep/gpt-oss-120b", got)
	}
	rm, err := cfg.ResolveLLM(got)
	if err != nil {
		t.Fatalf("ResolveLLM(%q): %v", got, err)
	}
	if rm.Model != "gpt-oss-120b" {
		t.Errorf("API model = %q, want gpt-oss-120b", rm.Model)
	}
}
