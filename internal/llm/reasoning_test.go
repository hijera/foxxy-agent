package llm

import (
	"testing"

	"github.com/openai/openai-go"
)

func TestOpenAIBuildParamsReasoningEffort(t *testing.T) {
	msgs := []Message{{Role: RoleUser, Content: "hi"}}

	p := newOpenAIProvider("gpt-5", "", "", nil, 1024, 0.5, "high")
	params := p.buildParams(msgs, nil)
	if params.ReasoningEffort != openai.ReasoningEffort("high") {
		t.Errorf("reasoning_effort = %q, want high", params.ReasoningEffort)
	}

	// Empty reasoning effort is omitted (zero value).
	none := newOpenAIProvider("gpt-4o", "", "", nil, 1024, 0.5, "")
	if got := none.buildParams(msgs, nil).ReasoningEffort; got != "" {
		t.Errorf("reasoning_effort = %q, want empty", got)
	}
}

func TestAnthropicBuildParamsThinking(t *testing.T) {
	p := newAnthropicProvider("claude-sonnet-4-5", "", nil, 8192, 0.7, "high")
	params := p.buildParams("", nil, nil)

	if params.Thinking.OfEnabled == nil {
		t.Fatal("expected thinking enabled for reasoning level high")
	}
	if got := params.Thinking.OfEnabled.BudgetTokens; got <= 0 || got >= 8192 {
		t.Errorf("budget_tokens = %d, want >0 and < max_tokens(8192)", got)
	}
	// Extended thinking requires temperature to be unset.
	if params.Temperature.Valid() {
		t.Error("temperature must be unset when thinking is enabled")
	}
}

func TestAnthropicBuildParamsNoThinkingKeepsTemperature(t *testing.T) {
	p := newAnthropicProvider("claude-3-5-sonnet", "", nil, 8192, 0.7, "")
	params := p.buildParams("", nil, nil)
	if params.Thinking.OfEnabled != nil {
		t.Error("thinking must be disabled when no reasoning level set")
	}
	if !params.Temperature.Valid() {
		t.Error("temperature should be set when thinking is disabled")
	}
}

func TestAnthropicThinkingBudgetBumpsMaxTokens(t *testing.T) {
	// Tiny max_tokens still yields a valid budget < max_tokens after bump.
	p := newAnthropicProvider("claude-sonnet-4-5", "", nil, 512, 0, "high")
	params := p.buildParams("", nil, nil)
	if params.Thinking.OfEnabled == nil {
		t.Fatal("expected thinking enabled")
	}
	if params.Thinking.OfEnabled.BudgetTokens >= params.MaxTokens {
		t.Errorf("budget %d must be < max_tokens %d", params.Thinking.OfEnabled.BudgetTokens, params.MaxTokens)
	}
}
