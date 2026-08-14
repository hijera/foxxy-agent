//go:build miniapps

package miniapps

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

type miniAppAgentTestProvider struct{}

func (miniAppAgentTestProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

type miniAppAgentWriteProvider struct {
	calls int
}

func (*miniAppAgentWriteProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *miniAppAgentWriteProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	if p.calls == 1 {
		return &llm.Response{ToolCalls: []llm.ToolCall{{
			ID: "write-1", Name: "write", InputJSON: `{"path":"agent.txt","content":"created"}`,
		}}, StopReason: "tool_use"}, nil
	}
	onChunk(llm.StreamChunk{TextDelta: "written"})
	return &llm.Response{Content: "written", StopReason: "end_turn"}, nil
}

func (miniAppAgentTestProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	onChunk(llm.StreamChunk{TextDelta: "agent result"})
	return &llm.Response{Content: "agent result", StopReason: "end_turn"}, nil
}

func TestBuiltinToolExecutorAllowsReviewedWorkspaceWriteAndRejectsEscapes(t *testing.T) {
	workspace := t.TempDir()
	registry := tools.NewRegistryForEnvironment(&config.Config{}, platform.CurrentEnvironment())
	executor := NewBuiltinToolExecutor(registry, []string{"write"})
	_, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Tool: "write", Workspace: workspace,
		Arguments: map[string]any{"path": "output.txt", "content": "generated"},
	})
	if err != nil {
		t.Fatalf("reviewed write failed: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(workspace, "output.txt")); err != nil || string(raw) != "generated" {
		t.Fatalf("workspace output = %q, err=%v", raw, err)
	}

	_, err = executor.ExecuteTool(context.Background(), ToolRequest{
		Tool: "write", Workspace: workspace,
		Arguments: map[string]any{"path": "../outside.txt", "content": "blocked"},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("workspace escape error = %v, want escape rejection", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(workspace), "outside.txt")); !os.IsNotExist(err) {
		t.Fatalf("workspace escape created a file: %v", err)
	}

	_, err = executor.ExecuteTool(context.Background(), ToolRequest{Tool: "run_command", Workspace: workspace, Arguments: map[string]any{"command": "echo blocked"}})
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("undeclared command error = %v, want allowlist rejection", err)
	}
}

func TestBuiltinToolExecutorRejectsMoveAndSymlinkEscapes(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	registry := tools.NewRegistryForEnvironment(&config.Config{}, platform.CurrentEnvironment())
	executor := NewBuiltinToolExecutor(registry, []string{"mv", "write"})

	_, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Tool: "mv", Workspace: workspace,
		Arguments: map[string]any{"src": "inside.txt", "dst": filepath.Join(outside, "moved.txt")},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("move escape error = %v, want escape rejection", err)
	}

	link := filepath.Join(workspace, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err = executor.ExecuteTool(context.Background(), ToolRequest{
		Tool: "write", Workspace: workspace,
		Arguments: map[string]any{"path": filepath.Join("outside-link", "written.txt"), "content": "blocked"},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("symlink escape error = %v, want escape rejection", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "written.txt")); !os.IsNotExist(err) {
		t.Fatalf("symlink escape created outside file: %v", err)
	}
}

func TestReActAgentExecutorUsesEphemeralSessionAndBoundedProvider(t *testing.T) {
	cfg := miniAppModelTestConfig()
	executor := NewReActAgentExecutor(cfg)
	executor.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) {
		return miniAppAgentTestProvider{}, nil
	})
	result, err := executor.ExecuteAgent(context.Background(), AgentRequest{
		AppID: "app", RunID: "run", StepID: "agent", Prompt: "say hello",
		Tools: []string{}, MaxTurns: 1, Workspace: t.TempDir(),
		Binding: ModelBinding{ID: "binding", Selection: "fixed", Provider: ProviderIdentity{Type: "openai", BaseURL: "https://example.test"}, Model: "test/fake-model"},
	})
	if err != nil {
		t.Fatalf("agent execution failed: %v", err)
	}
	if result != "agent result" {
		t.Fatalf("agent result = %#v", result)
	}
}

func TestReActAgentExecutorAllowsReviewedWorkspaceWrite(t *testing.T) {
	workspace := t.TempDir()
	provider := &miniAppAgentWriteProvider{}
	cfg := miniAppModelTestConfig()
	executor := NewReActAgentExecutor(cfg)
	executor.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return provider, nil })
	result, err := executor.ExecuteAgent(context.Background(), AgentRequest{
		AppID: "app", RunID: "run", StepID: "agent", Prompt: "write the file",
		Tools: []string{"write"}, MaxTurns: 2, Workspace: workspace,
		Binding: ModelBinding{ID: "binding", Selection: "fixed", Provider: ProviderIdentity{Type: "openai", BaseURL: "https://example.test"}, Model: "test/fake-model"},
	})
	if err != nil {
		t.Fatalf("agent execution failed: %v", err)
	}
	if result != "written" {
		t.Fatalf("agent result = %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "agent.txt"))
	if err != nil || string(raw) != "created" {
		t.Fatalf("workspace output = %q, err=%v", raw, err)
	}
}

func TestModelBindingRejectsUnconfiguredProviderEndpoint(t *testing.T) {
	_, _, err := modelConfigForBinding(miniAppModelTestConfig(), ModelBinding{
		ID: "binding", Selection: "fixed", Model: "fake/fake-model",
		Provider: ProviderIdentity{Type: "openai", BaseURL: "http://127.0.0.1:8080"},
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured model endpoint error = %v", err)
	}
}

func miniAppModelTestConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{{Name: "test", Type: "openai", APIBase: "https://example.test"}},
		Models:    []config.ModelEntry{{Model: "test/fake-model"}},
		Agent:     config.Agent{Model: "test/fake-model"},
	}
}

func TestMiniAppAgentGuardRejectsEveryPermissionBearingTool(t *testing.T) {
	registry := tools.NewRegistryForEnvironment(&config.Config{}, platform.CurrentEnvironment())
	registry.Register(&tools.Tool{Definition: llm.ToolDefinition{Name: "dangerous_custom"}, RequiresPermission: true})
	workspace := t.TempDir()
	if err := guardMiniAppAgentToolCall(registry, "dangerous_custom", `{}`, workspace); err == nil || !strings.Contains(err.Error(), "interactive") {
		t.Fatalf("permission-bearing background tool error = %v", err)
	}
	if err := guardMiniAppAgentToolCall(registry, "write", `{"path":"inside.txt","content":"ok"}`, workspace); err != nil {
		t.Fatalf("reviewed confined write was rejected: %v", err)
	}
}
