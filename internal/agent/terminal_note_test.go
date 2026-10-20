package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestRunInjectsTerminalContextIntoUserMessage(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	cwd := t.TempDir()
	ideterm.Set([]ideterm.Terminal{{ID: "1", Name: "zsh", Output: "ok internal/agent\n", Active: true}})

	st := &session.State{
		ID:         "sess_term_ctx",
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
	if !strings.Contains(userMsg, "<foxxycode_terminal_context>") || !strings.Contains(userMsg, "zsh") {
		t.Fatalf("user message missing terminal context block: %q", userMsg)
	}
}
