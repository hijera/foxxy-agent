package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type endTurnProvider struct{}

func (endTurnProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "ok", StopReason: "end_turn"}, nil
}

func (endTurnProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	onChunk(llm.StreamChunk{TextDelta: "ok"})
	return &llm.Response{Content: "ok", StopReason: "end_turn"}, nil
}

func TestRunInjectsIdeContextIntoUserMessage(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	cwd := t.TempDir()
	ideenv.Set([]string{cwd + "/main.go"}, cwd+"/main.go", nil)

	st := &session.State{
		ID:         "sess_ide_ctx",
		CWD:        cwd,
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return endTurnProvider{}, nil }

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hi"}}); err != nil {
		t.Fatal(err)
	}

	var userMsg string
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleUser {
			userMsg = m.Content
			break
		}
	}
	if !strings.Contains(userMsg, "<foxxycode_ide_context>") || !strings.Contains(userMsg, "main.go") {
		t.Fatalf("user message missing IDE context block: %q", userMsg)
	}
}
