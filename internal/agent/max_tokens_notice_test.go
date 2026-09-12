package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// cappedProvider ends a turn the way an output cap does: a complete-looking
// stream whose finish_reason says the answer was cut short.
type cappedProvider struct{}

func (cappedProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (cappedProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	const partial = "Step 1. Add the summarizer. Step 2. Wire it into"
	onChunk(llm.StreamChunk{TextDelta: partial})
	return &llm.Response{Content: partial, StopReason: "max_tokens", OutputTokens: 8192}, nil
}

func cappedAgent(t *testing.T, maxTokens int) (*Agent, *session.State) {
	t.Helper()
	st := &session.State{
		ID:         "sess_capped",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: maxTokens}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 4},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return cappedProvider{}, nil }
	return ag, st
}

// Hitting the output cap closes the stream cleanly, so on screen it is
// indistinguishable from a finished answer. The turn has to say so.
func TestMaxTokensTruncationIsReportedInTheTranscript(t *testing.T) {
	ag, st := cappedAgent(t, 8192)

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "write the plan"}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stop != string(acp.StopReasonMaxTokens) {
		t.Fatalf("stop reason = %q, want max_tokens", stop)
	}

	msgs := st.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant {
		t.Fatalf("last message is not the notice: %+v", last)
	}
	for _, want := range []string{"cut off", "max_tokens", "8192"} {
		if !strings.Contains(last.Content, want) {
			t.Errorf("notice missing %q: %q", want, last.Content)
		}
	}
	// The partial answer must survive, ahead of the notice.
	if !strings.Contains(msgs[len(msgs)-2].Content, "Add the summarizer") {
		t.Errorf("the truncated answer was lost: %+v", msgs[len(msgs)-2])
	}
}

// With no cap configured the provider's own default applied, so naming a
// configured limit (or printing a zero) would misdirect the reader.
func TestMaxTokensNoticeNamesTheProviderDefaultWhenNoCapIsSet(t *testing.T) {
	got := maxTokensNotice(0, 4096)
	if !strings.Contains(got, "provider's own default") {
		t.Errorf("notice should explain that no cap is configured: %q", got)
	}
	if strings.Contains(got, " 0 ") {
		t.Errorf("notice must not print a zero cap as a number: %q", got)
	}
}

func TestEffectiveMaxTokensReadsTheResolvedModel(t *testing.T) {
	ag, _ := cappedAgent(t, 32000)
	if got := ag.effectiveMaxTokens(); got != 32000 {
		t.Errorf("effectiveMaxTokens = %d, want 32000", got)
	}
}
