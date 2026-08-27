//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// ConfigSource resolves the configuration a Mini App step should run against.
// The HTTP transport passes the server's live accessor, so a provider, key,
// model, or MCP change saved in Settings reaches the next step instead of the
// next restart. A nil source, or one that returns nil, is treated as "no
// configuration" rather than panicking mid-run.
type ConfigSource func() *config.Config

func staticConfigSource(cfg *config.Config) ConfigSource {
	return func() *config.Config { return cfg }
}

func (source ConfigSource) resolve() *config.Config {
	if source == nil {
		return nil
	}
	return source()
}

// BuiltinToolExecutor adapts the registered FoxxyCode tools to deterministic
// Mini App steps. The allowlist is always enforced, including when a caller
// accidentally constructs it with a nil slice. Permission-bearing tools are
// rejected because a background Mini App run has no interactive ACP approval.
type BuiltinToolExecutor struct {
	// registry pins one prebuilt registry; it stays nil for the live variant,
	// which rebuilds from source on every call.
	registry        *tooling.Registry
	source          ConfigSource
	allowlist       map[string]struct{}
	allowRestricted bool
}

func NewBuiltinToolExecutor(registry *tooling.Registry, allowlist []string) *BuiltinToolExecutor {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		name = strings.TrimSpace(name)
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	return &BuiltinToolExecutor{registry: registry, allowlist: allowed, allowRestricted: true}
}

func NewConfiguredBuiltinToolExecutor(cfg *config.Config, allowlist []string) *BuiltinToolExecutor {
	return NewBuiltinToolExecutor(tools.NewRegistryForEnvironment(cfg, platform.CurrentEnvironment()), allowlist)
}

// NewLiveBuiltinToolExecutor builds the tool registry from the currently active
// configuration on every call. Its allowlist is that live registry, which is
// exactly what the HTTP transport used to enumerate and pass in by hand; the
// per-Mini-App reviewed permission list still gates each individual call.
func NewLiveBuiltinToolExecutor(source ConfigSource) *BuiltinToolExecutor {
	return &BuiltinToolExecutor{source: source}
}

func (e *BuiltinToolExecutor) currentRegistry() *tooling.Registry {
	if e == nil {
		return nil
	}
	if e.registry != nil {
		return e.registry
	}
	cfg := e.source.resolve()
	if cfg == nil {
		return nil
	}
	return tools.NewRegistryForEnvironment(cfg, platform.CurrentEnvironment())
}

func (e *BuiltinToolExecutor) ValidateMiniAppCapabilities(app MiniApp) error {
	registry := e.currentRegistry()
	if registry == nil {
		return errors.New("builtin tool registry is unavailable")
	}
	for _, name := range app.Permissions.Tools {
		if _, allowed := e.allowlist[name]; e.allowRestricted && !allowed {
			return fmt.Errorf("tool %q is not enabled for Mini Apps", name)
		}
		tool, found := registry.Get(name)
		if !found || tool == nil {
			return fmt.Errorf("tool %q is not registered", name)
		}
		if tool.RequiresPermission || builtinToolNeedsInteractivePermission(name) {
			return fmt.Errorf("tool %q requires interactive permission", name)
		}
	}
	return nil
}

func (e *BuiltinToolExecutor) ExecuteTool(ctx context.Context, req ToolRequest) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	registry := e.currentRegistry()
	if registry == nil {
		return nil, errors.New("builtin tool registry is unavailable")
	}
	name := strings.TrimSpace(req.Tool)
	if name == "" {
		return nil, errors.New("tool name is required")
	}
	if e.allowRestricted {
		if _, ok := e.allowlist[name]; !ok {
			return nil, fmt.Errorf("tool %q is not declared in Mini App allowlist", name)
		}
	}
	tool, ok := registry.Get(name)
	if !ok || tool == nil {
		return nil, fmt.Errorf("unknown builtin tool %q", name)
	}
	if tool.RequiresPermission || builtinToolNeedsInteractivePermission(name) {
		return nil, fmt.Errorf("tool %q requires interactive permission and is unavailable to Mini App execution", name)
	}
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		return nil, errors.New("workspace is required for a Mini App tool step")
	}
	argsJSON, err := miniAppArgumentsJSON(req.Arguments)
	if err != nil {
		return nil, err
	}
	if err := validateBuiltinToolPaths(name, req.Arguments, workspace); err != nil {
		return nil, err
	}
	env := &tooling.Env{
		CWD:            workspace,
		PermissionMode: config.PermModeAsk,
		SessionID:      req.RunID,
	}
	result, err := registry.Execute(ctx, name, argsJSON, env)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// builtinToolNeedsInteractivePermission names the tools a background Mini App
// run must never reach, on top of the registry's own RequiresPermission flag.
// Names are matched as the registry actually registers them, so the foxxycode_
// prefix the browser family carries is normalized away first.
func builtinToolNeedsInteractivePermission(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	// Namespaced MCP calls are never Mini App builtins.
	if strings.Contains(name, "__") {
		return true
	}
	bare := strings.TrimPrefix(name, "foxxycode_")
	switch bare {
	case "run_command", "ssh_run_command":
		return true
	}
	// The whole browser family drives the operator's shared browser session, so
	// even its read-only members stay out of an unattended run.
	if strings.HasPrefix(bare, "browser_") {
		return true
	}
	return strings.HasSuffix(bare, "_create") || strings.HasSuffix(bare, "_update") || strings.HasSuffix(bare, "_delete")
}

func validateBuiltinToolPaths(name string, args any, workspace string) error {
	root, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve Mini App workspace: %w", err)
	}
	var value any
	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode tool arguments: %w", err)
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode tool arguments: %w", err)
	}
	var walk func(any, string) error
	walk = func(item any, key string) error {
		switch typed := item.(type) {
		case map[string]any:
			for childKey, child := range typed {
				if err := walk(child, childKey); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range typed {
				if err := walk(child, key); err != nil {
					return err
				}
			}
		case string:
			if !isFilesystemPathKey(key) {
				return nil
			}
			candidate := typed
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(root, candidate)
			}
			candidate, err := canonicalPathForContainment(candidate)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, candidate)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return fmt.Errorf("tool %q path %q escapes its run workspace", name, typed)
			}
		}
		return nil
	}
	return walk(value, "")
}

func canonicalPathForContainment(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	current := abs
	for {
		if _, statErr := os.Lstat(current); statErr == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", resolveErr
			}
			remainder, relErr := filepath.Rel(current, abs)
			if relErr != nil {
				return "", relErr
			}
			return filepath.Join(resolved, remainder), nil
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		current = parent
	}
}

func isFilesystemPathKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "path", "paths", "file", "files", "directory", "dir", "destination", "target", "cwd", "working_directory", "src", "dst", "source":
		return true
	default:
		return false
	}
}

func miniAppArgumentsJSON(value any) (string, error) {
	if value == nil {
		return "{}", nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		if !json.Valid(raw) {
			return "", errors.New("tool arguments are not valid JSON")
		}
		return string(raw), nil
	}
	if text, ok := value.(string); ok && json.Valid([]byte(text)) {
		return text, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode tool arguments: %w", err)
	}
	return string(raw), nil
}

// ProviderModelExecutor is the bounded tool-free model adapter used by llm
// steps and model-backed success checks.
type ProviderModelExecutor struct {
	source  ConfigSource
	factory func(llm.ProviderInput) (llm.Provider, error)
}

func NewProviderModelExecutor(cfg *config.Config) *ProviderModelExecutor {
	return NewLiveProviderModelExecutor(staticConfigSource(cfg))
}

// NewLiveProviderModelExecutor resolves the configuration on every step, so a
// model or key edited in Settings applies to the next llm step.
func NewLiveProviderModelExecutor(source ConfigSource) *ProviderModelExecutor {
	return &ProviderModelExecutor{source: source, factory: llm.NewProvider}
}

func (e *ProviderModelExecutor) SetProviderFactory(factory func(llm.ProviderInput) (llm.Provider, error)) {
	if e != nil && factory != nil {
		e.factory = factory
	}
}

func (e *ProviderModelExecutor) ValidateMiniAppCapabilities(app MiniApp) error {
	if e == nil {
		return errors.New("model configuration is unavailable")
	}
	cfg := e.source.resolve()
	if cfg == nil {
		return errors.New("model configuration is unavailable")
	}
	for _, binding := range app.Requirements.ModelBindings {
		if _, _, err := modelConfigForBinding(cfg, binding); err != nil {
			return err
		}
	}
	return nil
}

func (e *ProviderModelExecutor) ExecuteModel(ctx context.Context, req ModelRequest) (any, error) {
	if e == nil {
		return nil, errors.New("model configuration is unavailable")
	}
	cfg := e.source.resolve()
	if cfg == nil {
		return nil, errors.New("model configuration is unavailable")
	}
	modelCfg, modelRef, err := modelConfigForBinding(cfg, req.Binding)
	if err != nil {
		return nil, err
	}
	resolved, err := modelCfg.ResolveLLM(modelRef)
	if err != nil {
		return nil, err
	}
	factory := e.factory
	if factory == nil {
		factory = llm.NewProvider
	}
	provider, err := factory(llm.ProviderInput{
		Type: resolved.ProviderType, Model: resolved.Model, APIKey: resolved.APIKey,
		BaseURL: resolved.BaseURL, ProxyURL: resolved.ProxyURL, AuthPath: resolved.AuthPath,
		MaxTokens: resolved.MaxTokens, Temperature: resolved.Temperature,
	})
	if err != nil {
		return nil, err
	}
	response, err := provider.Complete(ctx, []llm.Message{{Role: llm.RoleUser, Content: req.Prompt}}, nil)
	if err != nil {
		return nil, fmt.Errorf("model step: %w", err)
	}
	if response == nil {
		return nil, errors.New("model step returned no response")
	}
	content := strings.TrimSpace(response.Content)
	if req.OutputSchema == nil {
		return content, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return nil, fmt.Errorf("model output is not JSON: %w", err)
	}
	if err := validateJSONType(decoded, req.OutputSchema); err != nil {
		return nil, err
	}
	return decoded, nil
}

func modelConfigForBinding(base *config.Config, binding ModelBinding) (*config.Config, string, error) {
	if base == nil {
		return nil, "", errors.New("model configuration is unavailable")
	}
	modelRef := strings.TrimSpace(binding.Model)
	if modelRef == "" && binding.Selection == "capability" {
		modelRef = strings.TrimSpace(base.Agent.Model)
	}
	if modelRef == "" {
		return nil, "", fmt.Errorf("model binding %q has no model", binding.ID)
	}
	entry := base.FindModelEntry(modelRef)
	if entry == nil {
		return nil, "", fmt.Errorf("model binding %q references model %q that is not configured", binding.ID, modelRef)
	}
	provider := base.FindProvider(entry.ProviderName())
	if provider == nil {
		return nil, "", fmt.Errorf("model binding %q references unavailable provider %q", binding.ID, entry.ProviderName())
	}
	if want := strings.TrimSpace(binding.Provider.Type); want != "" && want != provider.Type {
		return nil, "", fmt.Errorf("model binding %q provider type does not match configured provider", binding.ID)
	}
	if want := strings.TrimSpace(binding.Provider.BaseURL); want != "" && strings.TrimRight(want, "/") != strings.TrimRight(provider.APIBase, "/") {
		return nil, "", fmt.Errorf("model binding %q provider URL does not match configured provider", binding.ID)
	}
	clone := *base
	clone.Providers = append([]config.ProviderConfig(nil), base.Providers...)
	clone.Models = append([]config.ModelEntry(nil), base.Models...)
	clone.Agent.Model = modelRef
	return &clone, modelRef, nil
}
