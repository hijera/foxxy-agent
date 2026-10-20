//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// ReActAgentExecutor runs one bounded real FoxxyCode ReAct turn in an
// ephemeral session. Only non-interactive registered tools are eligible for
// guarded one-time approval, so the reviewed allowlist cannot bypass policy.
type ReActAgentExecutor struct {
	source  ConfigSource
	factory func(llm.ProviderInput) (llm.Provider, error)
}

func NewReActAgentExecutor(cfg *config.Config) *ReActAgentExecutor {
	return NewLiveReActAgentExecutor(staticConfigSource(cfg))
}

// NewLiveReActAgentExecutor resolves the configuration on every agent step, so
// the tool registry and model binding follow what Settings currently holds.
func NewLiveReActAgentExecutor(source ConfigSource) *ReActAgentExecutor {
	return &ReActAgentExecutor{source: source, factory: llm.NewProvider}
}

// NewAgentExecutor is a concise alias for callers wiring the runtime.
func NewAgentExecutor(cfg *config.Config) *ReActAgentExecutor {
	return NewReActAgentExecutor(cfg)
}

func (e *ReActAgentExecutor) SetProviderFactory(factory func(llm.ProviderInput) (llm.Provider, error)) {
	if e != nil && factory != nil {
		e.factory = factory
	}
}

func (e *ReActAgentExecutor) ValidateMiniAppCapabilities(app MiniApp) error {
	if e == nil {
		return errors.New("agent configuration is unavailable")
	}
	cfg := e.source.resolve()
	if cfg == nil {
		return errors.New("agent configuration is unavailable")
	}
	registry := tools.NewRegistryForEnvironment(cfg, platform.CurrentEnvironment())
	for _, binding := range app.Requirements.ModelBindings {
		if _, _, err := modelConfigForBinding(cfg, binding); err != nil {
			return err
		}
	}
	for _, step := range flattenMiniAppSteps(app.Workflow) {
		if step.Kind != "agent" {
			continue
		}
		for _, name := range step.Tools {
			tool, found := registry.Get(name)
			if !found || tool == nil {
				return fmt.Errorf("agent tool %q is not registered", name)
			}
			if tool.RequiresPermission || builtinToolNeedsInteractivePermission(name) {
				return fmt.Errorf("agent tool %q requires interactive permission", name)
			}
		}
	}
	return nil
}

func flattenMiniAppSteps(steps []Step) []Step {
	flat := make([]Step, 0, len(steps))
	for _, step := range steps {
		flat = append(flat, step)
		flat = append(flat, flattenMiniAppSteps(step.Then)...)
		flat = append(flat, flattenMiniAppSteps(step.Else)...)
	}
	return flat
}

func (e *ReActAgentExecutor) ExecuteAgent(ctx context.Context, req AgentRequest) (any, error) {
	if e == nil {
		return nil, errors.New("agent configuration is unavailable")
	}
	cfg := e.source.resolve()
	if cfg == nil {
		return nil, errors.New("agent configuration is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		return nil, errors.New("workspace is required for a Mini App agent step")
	}
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return nil, fmt.Errorf("create agent workspace: %w", err)
	}
	modelCfg, modelRef, err := modelConfigForBinding(cfg, req.Binding)
	if err != nil {
		return nil, err
	}
	modelCfg.Agent.Model = modelRef
	maxTurns := req.MaxTurns
	if maxTurns <= 0 {
		maxTurns = modelCfg.Agent.MaxTurns
	}
	if maxTurns <= 0 {
		maxTurns = 8
	}
	if maxTurns > 64 {
		maxTurns = 64
	}

	state := &session.State{
		ID:   "miniapp-" + req.RunID + "-" + req.StepID,
		CWD:  workspace,
		Mode: session.ModeAgent,
	}
	sender := allowingMiniAppSender{}
	allowedTools := make([]string, len(req.Tools))
	copy(allowedTools, req.Tools)
	registry := tools.NewRegistryForEnvironment(modelCfg, platform.CurrentEnvironment())
	ag := agent.NewAgentWithOptions(modelCfg, state, sender, nil, agent.AgentOptions{
		BuiltinToolAllowlist: allowedTools,
		MaxTurns:             maxTurns,
		ToolCallGuard: func(name, argsJSON, cwd string) error {
			return guardMiniAppAgentToolCall(registry, name, argsJSON, cwd)
		},
	})
	if e.factory != nil {
		ag.SetProviderFactory(e.factory)
	}
	stop, err := ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: req.Prompt}})
	if err != nil {
		return nil, err
	}
	if stop == string(acp.StopReasonCancelled) {
		return nil, context.Canceled
	}
	if stop == string(acp.StopReasonMaxTurns) {
		return nil, fmt.Errorf("agent step exceeded max turns (%d)", maxTurns)
	}
	if stop != string(acp.StopReasonEndTurn) && stop != "" {
		return nil, fmt.Errorf("agent step stopped with %s", stop)
	}
	content := ""
	msgs := state.GetMessages()
	for index := len(msgs) - 1; index >= 0; index-- {
		if msgs[index].Role == llm.RoleAssistant {
			content = strings.TrimSpace(msgs[index].Content)
			break
		}
	}
	if req.OutputSchema == nil {
		return content, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return nil, fmt.Errorf("agent output is not JSON: %w", err)
	}
	if err := validateJSONType(decoded, req.OutputSchema); err != nil {
		return nil, err
	}
	return decoded, nil
}

func guardMiniAppAgentToolCall(registry *tools.Registry, name, argsJSON, cwd string) error {
	if registry == nil {
		return errors.New("tool registry is unavailable")
	}
	tool, found := registry.Get(name)
	if !found || tool == nil {
		return errors.New("tool is not a registered Mini App builtin")
	}
	if tool.RequiresPermission || builtinToolNeedsInteractivePermission(name) {
		return errors.New("interactive, command, and MCP tools are unavailable to Mini App agent steps")
	}
	var args any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Errorf("decode tool arguments: %w", err)
	}
	return validateBuiltinToolPaths(name, args, cwd)
}

// allowingMiniAppSender acknowledges permission prompts only after the Agent's
// per-call guard has confined the exact allowlisted operation to the run
// workspace. Interactive, command, and MCP tools fail before this boundary.
type allowingMiniAppSender struct{}

func (allowingMiniAppSender) SendSessionUpdate(string, interface{}) error { return nil }

func (allowingMiniAppSender) RequestPermission(_ context.Context, params acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	for _, option := range params.Options {
		if option.Kind == "allow_once" {
			return &acp.PermissionResult{Outcome: "selected", OptionID: option.OptionID}, nil
		}
	}
	return nil, errors.New("permission request has no one-time approval option")
}

func (allowingMiniAppSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return nil, errors.New("questions are unavailable during Mini App agent execution")
}
