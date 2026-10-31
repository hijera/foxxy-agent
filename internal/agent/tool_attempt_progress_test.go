package agent

import (
	"context"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type retryPreviewProvider struct{}

func (retryPreviewProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (retryPreviewProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	tc := llm.ToolCall{ID: "write1", Name: "write"}
	onChunk(llm.StreamChunk{Attempt: 1, ToolCallNamed: &tc})
	tc.InputJSON = `{"path":"x","content":"old`
	onChunk(llm.StreamChunk{Attempt: 1, ToolCallDelta: &tc})
	onChunk(llm.StreamChunk{Attempt: 2, ToolCallNamed: &tc})
	tc.InputJSON = `{"path":"x","content":"new"}`
	onChunk(llm.StreamChunk{Attempt: 2, ToolCallDelta: &tc})
	onChunk(llm.StreamChunk{Attempt: 2, ToolCall: &tc})
	return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
}

func TestToolInputProgressResetsOnProviderAttempt(t *testing.T) {
	st := &session.State{ID: "sess_retry_preview", CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	sender := &progressSender{}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model"}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 4},
	}, st, sender, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return retryPreviewProvider{}, nil }
	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hi"}}); err != nil {
		t.Fatalf("run: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	var last map[string]interface{}
	for _, update := range sender.updates {
		u, ok := update.(acp.ToolCallStatusUpdate)
		if !ok || u.Meta == nil {
			continue
		}
		foxxycode, _ := u.Meta["foxxycode"].(map[string]interface{})
		progress, _ := foxxycode["toolInputProgress"].(map[string]interface{})
		if progress != nil {
			last = progress
		}
	}
	if last == nil {
		t.Fatal("no tool input progress updates")
	}
	if got := last["preview"]; got != "new" {
		t.Fatalf("preview = %q, want only the second attempt's content", got)
	}
	if got := last["argumentBytes"]; got != len(`{"path":"x","content":"new"}`) {
		t.Fatalf("argumentBytes = %v, want only the second attempt's bytes", got)
	}
}
