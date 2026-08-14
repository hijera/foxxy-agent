package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mcp"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

type miniAppOptionsSender struct {
	permissionRequests int
}

func (s *miniAppOptionsSender) SendSessionUpdate(string, interface{}) error { return nil }

func (s *miniAppOptionsSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	s.permissionRequests++
	return &acp.PermissionResult{Outcome: "allow", OptionID: "allow"}, nil
}

func (s *miniAppOptionsSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

type miniAppDefinitionProvider struct {
	definitions []llm.ToolDefinition
}

func (p *miniAppDefinitionProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Complete should not be called")
}

func (p *miniAppDefinitionProvider) Stream(_ context.Context, _ []llm.Message, defs []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.definitions = append([]llm.ToolDefinition(nil), defs...)
	onChunk(llm.StreamChunk{TextDelta: "done"})
	return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
}

type miniAppToolLoopProvider struct {
	calls int
}

func (p *miniAppToolLoopProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Complete should not be called")
}

func (p *miniAppToolLoopProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, _ func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	tc := llm.ToolCall{ID: fmt.Sprintf("read-%d", p.calls), Name: "read", InputJSON: `{"path":"missing"}`}
	return &llm.Response{ToolCalls: []llm.ToolCall{tc}, StopReason: "tool_use"}, nil
}

func miniAppAgentConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}
}

func TestAgentOptionsAllowlistFiltersDefinitionsAndRejectsBuiltinBeforePermission(t *testing.T) {
	st := &session.State{ID: "miniapp_allowlist", CWD: t.TempDir(), Mode: session.ModeAgent}
	sender := &miniAppOptionsSender{}
	ag := NewAgentWithOptions(miniAppAgentConfig(), st, sender, nil, AgentOptions{
		BuiltinToolAllowlist: []string{"read"},
	})
	provider := &miniAppDefinitionProvider{}
	ag.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return provider, nil })

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "inspect the file"}}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	seen := make(map[string]bool, len(provider.definitions))
	for _, definition := range provider.definitions {
		seen[definition.Name] = true
	}
	if !seen["read"] {
		t.Fatal("allowlisted read tool was not exposed to the model")
	}
	for _, name := range []string{"write", "run_command", "apply_patch"} {
		if seen[name] {
			t.Fatalf("undeclared builtin %q was exposed to the model", name)
		}
	}

	env := &tools.Env{CWD: st.GetCWD(), PermissionMode: config.PermModeAsk}
	_, err := ag.executeToolCall(context.Background(), llm.ToolCall{
		ID:        "blocked-write",
		Name:      "write",
		InputJSON: `{"path":"blocked.txt","content":"must not be written"}`,
	}, env, string(session.ModeAgent), st.GetID(), false)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("undeclared builtin call error = %v, want not-allowed error", err)
	}
	if sender.permissionRequests != 0 {
		t.Fatalf("undeclared builtin reached permission handling %d time(s)", sender.permissionRequests)
	}
}

func TestAgentOptionsAllowlistFiltersAndRejectsMCPTools(t *testing.T) {
	st := &session.State{
		ID:         "miniapp_mcp_allowlist",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		MCPClients: []*mcp.Client{mcp.NewStaticClient("srv", []mcp.ToolInfo{{Name: "echo", ReadOnly: true}})},
	}
	sender := &miniAppOptionsSender{}
	ag := NewAgentWithOptions(miniAppAgentConfig(), st, sender, nil, AgentOptions{
		BuiltinToolAllowlist: []string{"read"},
	})
	for _, definition := range ag.mcpToolDefinitions(string(session.ModeAgent), false) {
		if definition.Name == "srv__echo" {
			t.Fatal("undeclared MCP tool was exposed to the model")
		}
	}

	env := &tools.Env{CWD: st.GetCWD(), PermissionMode: config.PermModeAsk}
	_, err := ag.executeToolCall(context.Background(), llm.ToolCall{
		ID:        "blocked-mcp",
		Name:      "srv__echo",
		InputJSON: `{}`,
	}, env, string(session.ModeAgent), st.GetID(), false)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("undeclared MCP call error = %v, want not-allowed error", err)
	}
	if sender.permissionRequests != 0 {
		t.Fatalf("undeclared MCP call reached permission handling %d time(s)", sender.permissionRequests)
	}
}

func TestAgentOptionsToolCallGuardRunsBeforePermission(t *testing.T) {
	st := &session.State{ID: "miniapp_guard", CWD: t.TempDir(), Mode: session.ModeAgent}
	sender := &miniAppOptionsSender{}
	guardCalls := 0
	ag := NewAgentWithOptions(miniAppAgentConfig(), st, sender, nil, AgentOptions{
		BuiltinToolAllowlist: []string{"write"},
		ToolCallGuard: func(name, argsJSON, cwd string) error {
			guardCalls++
			return fmt.Errorf("outside isolated workspace")
		},
	})

	env := &tools.Env{CWD: st.GetCWD(), PermissionMode: config.PermModeAsk}
	_, err := ag.executeToolCall(context.Background(), llm.ToolCall{
		ID: "guarded-write", Name: "write",
		InputJSON: `{"path":"../outside.txt","content":"blocked"}`,
	}, env, string(session.ModeAgent), st.GetID(), false)
	if err == nil || !strings.Contains(err.Error(), "outside isolated workspace") {
		t.Fatalf("guarded call error = %v", err)
	}
	if guardCalls != 1 {
		t.Fatalf("guard calls = %d, want 1", guardCalls)
	}
	if sender.permissionRequests != 0 {
		t.Fatalf("guarded call reached permission handling %d time(s)", sender.permissionRequests)
	}
}

func TestAgentOptionsMaxTurnsOverridesConfiguredLimit(t *testing.T) {
	st := &session.State{ID: "miniapp_max_turns", CWD: t.TempDir(), Mode: session.ModeAgent}
	ag := NewAgentWithOptions(miniAppAgentConfig(), st, &miniAppOptionsSender{}, nil, AgentOptions{
		BuiltinToolAllowlist: []string{"read"},
		MaxTurns:             1,
	})
	provider := &miniAppToolLoopProvider{}
	ag.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return provider, nil })

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "inspect repeatedly"}})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if stop != string(acp.StopReasonMaxTurns) {
		t.Fatalf("stop = %q, want max_turns", stop)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one call under per-run max turns", provider.calls)
	}
}
