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

// Settings reach a running server through ReplaceConfig, so every executor has
// to resolve the configuration per call. Capturing it once pinned each run to
// whatever happened to be live when the first Mini Apps request arrived, which
// meant a changed provider, key, or model only applied after a restart.
func TestExecutorsResolveConfigurationPerCall(t *testing.T) {
	active := &config.Config{}
	source := ConfigSource(func() *config.Config { return active })

	app := coreValidApp()
	app.Requirements.ModelBindings = []ModelBinding{{
		ID: "binding", Selection: "fixed", Model: "test/fake-model",
		Provider: ProviderIdentity{Type: "openai", BaseURL: "https://example.test"},
	}}

	model := NewLiveProviderModelExecutor(source)
	agentExecutor := NewLiveReActAgentExecutor(source)
	if err := model.ValidateMiniAppCapabilities(app); err == nil {
		t.Fatal("model binding validated against a configuration without the provider")
	}
	if err := agentExecutor.ValidateMiniAppCapabilities(app); err == nil {
		t.Fatal("agent binding validated against a configuration without the provider")
	}

	active = miniAppModelTestConfig() // the operator saves the provider in Settings

	if err := model.ValidateMiniAppCapabilities(app); err != nil {
		t.Fatalf("model executor kept the stale configuration: %v", err)
	}
	if err := agentExecutor.ValidateMiniAppCapabilities(app); err != nil {
		t.Fatalf("agent executor kept the stale configuration: %v", err)
	}
}

func TestLiveBuiltinToolExecutorResolvesRegistryPerCall(t *testing.T) {
	var active *config.Config
	executor := NewLiveBuiltinToolExecutor(func() *config.Config { return active })

	app := coreValidApp()
	app.Permissions = Permissions{Tools: []string{"write"}}
	if err := executor.ValidateMiniAppCapabilities(app); err == nil {
		t.Fatal("tool executor validated without a configuration")
	}

	active = miniAppModelTestConfig()

	if err := executor.ValidateMiniAppCapabilities(app); err != nil {
		t.Fatalf("tool executor kept the stale registry: %v", err)
	}
	workspace := t.TempDir()
	if _, err := executor.ExecuteTool(context.Background(), ToolRequest{
		RunID: "run", Tool: "write", Workspace: workspace,
		Arguments: map[string]any{"path": "live.txt", "content": "ok"},
	}); err != nil {
		t.Fatalf("reviewed write against the live registry failed: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(workspace, "live.txt")); err != nil || string(raw) != "ok" {
		t.Fatalf("workspace output = %q, err=%v", raw, err)
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

// The registry namespaces the browser family as foxxycode_browser_*, so matching
// the bare names left this guard inert for every tool it was written to stop.
// The whole family is out of reach: it drives the operator's shared browser
// session, which an unattended run must not touch even to read.
func TestBuiltinToolNeedsInteractivePermissionMatchesRegisteredNames(t *testing.T) {
	unavailable := []string{
		"run_command", "ssh_run_command",
		"foxxycode_browser_click", "foxxycode_browser_fill", "foxxycode_browser_evaluate",
		"foxxycode_browser_hover", "foxxycode_browser_navigate", "foxxycode_browser_scroll",
		"foxxycode_browser_screenshot", "foxxycode_browser_close",
		"mcp__github__create_issue", "job_create", "job_update", "job_delete",
	}
	for _, name := range unavailable {
		if !builtinToolNeedsInteractivePermission(name) {
			t.Errorf("%q must be unavailable to unattended Mini App execution", name)
		}
	}
	for _, name := range []string{"read", "write", "edit", "glob", "grep", "print_tree", "mkdir"} {
		if builtinToolNeedsInteractivePermission(name) {
			t.Errorf("%q must stay available to a reviewed Mini App step", name)
		}
	}
}
