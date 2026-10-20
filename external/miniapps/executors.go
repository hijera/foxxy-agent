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

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	"github.com/hijera/foxxycode-agent/internal/tools"
	toolcmd "github.com/hijera/foxxycode-agent/internal/tools/cmdprofile"
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
		// A document-embedded command profile is its own declaration: validate
		// the spec instead of requiring a registry entry.
		if profile, declared := commandProfileByToolName(app.Requirements.Commands, name); declared {
			if err := profile.Validate(); err != nil {
				return fmt.Errorf("command profile %q: %w", profile.Name, err)
			}
			if profile.ResolvedPermission() != cmdprofile.PermissionAllow {
				return fmt.Errorf("command profile %q must declare permission: allow", profile.Name)
			}
			continue
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
	// A document-embedded profile is executed from its own declaration; the
	// registry only serves ordinary builtins and config-declared profiles.
	if req.CommandProfile != nil {
		return e.executeCommandProfile(ctx, req, argsJSON, workspace)
	}
	tool, ok := registry.Get(name)
	if !ok || tool == nil {
		return nil, fmt.Errorf("unknown builtin tool %q", name)
	}
	if tool.RequiresPermission || builtinToolNeedsInteractivePermission(name) {
		return nil, fmt.Errorf("tool %q requires interactive permission and is unavailable to Mini App execution", name)
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

// executeCommandProfile runs a document-embedded command profile. The order of
// the gates matters: an ask-profile is a hard error (a Mini App runs
// unattended, so a prompt could never be answered), a missing binary is an
// actionable install error, and an untrusted profile pauses the run through
// the existing waiting-for-confirmation flow. Trust binds the profile content
// hash to the exact resolved binary path; a profile whose hash matches an
// operator-declared config profile is implicitly trusted.
func (e *BuiltinToolExecutor) executeCommandProfile(ctx context.Context, req ToolRequest, argsJSON, workspace string) (any, error) {
	profile := req.CommandProfile.Clone()
	// When the operator's config declares this same profile (the document form
	// is the portable, bare-name spelling of it), execute the config
	// declaration instead: it knows where the binary actually lives on this
	// machine, and it is the operator's own trust anchor.
	if declared, ok := e.configProfileForDocument(profile); ok {
		profile = declared
	}
	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("command profile %q: %w", profile.Name, err)
	}
	if profile.ResolvedPermission() != cmdprofile.PermissionAllow {
		return nil, fmt.Errorf("command profile %q must declare permission: allow to run inside a Mini App", profile.Name)
	}
	resolved, err := cmdprofile.ResolveBinary(profile, workspace)
	if err != nil {
		var missing *cmdprofile.BinaryNotFoundError
		if errors.As(err, &missing) {
			return nil, fmt.Errorf("command %q is not installed on this machine%s", profile.Binary, commandInstallHint(profile))
		}
		return nil, err
	}
	hash, err := cmdprofile.CanonicalHash(profile)
	if err != nil {
		return nil, err
	}
	if !e.commandProfileTrusted(hash, resolved) {
		return nil, fmt.Errorf(
			"command profile %q wants to run %s and is not trusted on this machine: %w",
			profile.Name, resolved, errWaitingForConfirmation)
	}
	tool := toolcmd.Tool(profile)
	out, err := tool.Execute(ctx, argsJSON, &tooling.Env{
		CWD: workspace, PermissionMode: config.PermModeAsk, SessionID: req.RunID,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// configProfileForDocument finds the config-declared profile whose portable
// form equals the document profile's content.
func (e *BuiltinToolExecutor) configProfileForDocument(document cmdprofile.ProfileSpec) (cmdprofile.ProfileSpec, bool) {
	cfg := e.source.resolve()
	if cfg == nil {
		return cmdprofile.ProfileSpec{}, false
	}
	documentHash, err := cmdprofile.CanonicalHash(document)
	if err != nil {
		return cmdprofile.ProfileSpec{}, false
	}
	for _, declared := range cfg.Commands {
		if portableHash, err := cmdprofile.CanonicalHash(declared.Portable()); err == nil && portableHash == documentHash {
			return declared.Clone(), true
		}
		if declaredHash, err := cmdprofile.CanonicalHash(declared); err == nil && declaredHash == documentHash {
			return declared.Clone(), true
		}
	}
	return cmdprofile.ProfileSpec{}, false
}

// commandProfileTrusted reports whether the profile may run without pausing:
// either its hash matches an operator-declared config profile (in declared or
// portable form), or the trust store holds an approval for this hash bound to
// this resolved binary path.
func (e *BuiltinToolExecutor) commandProfileTrusted(hash, resolved string) bool {
	cfg := e.source.resolve()
	if cfg != nil {
		for _, declared := range cfg.Commands {
			if declaredHash, err := cmdprofile.CanonicalHash(declared); err == nil && declaredHash == hash {
				return true
			}
			if portableHash, err := cmdprofile.CanonicalHash(declared.Portable()); err == nil && portableHash == hash {
				return true
			}
		}
	}
	home := ""
	if cfg != nil {
		home = strings.TrimSpace(cfg.Paths.Home)
	}
	return cmdprofile.NewTrustStore(home).Trusted(hash, resolved)
}

// commandInstallHint appends the exact install command(s) for the detected
// package managers.
func commandInstallHint(profile cmdprofile.ProfileSpec) string {
	managers := cmdprofile.DetectManagers(profile)
	if len(managers) == 0 {
		return "; install it and ensure it is on PATH"
	}
	var hints []string
	for _, manager := range managers {
		hints = append(hints, strings.Join(manager.Argv, " "))
	}
	return "; install it with: " + strings.Join(hints, "  |  ")
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
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "path", "paths", "file", "files", "directory", "dir", "destination", "target", "cwd", "working_directory", "src", "dst", "source":
		return true
	}
	// Command-profile file params are named *_path/_file/_dir by construction,
	// and the suffix rule hardens every builtin argument spelled that way too.
	return strings.HasSuffix(key, "_path") || strings.HasSuffix(key, "_file") || strings.HasSuffix(key, "_dir")
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

func (e *ProviderModelExecutor) providerForBinding(binding ModelBinding) (llm.Provider, error) {
	if e == nil {
		return nil, errors.New("model configuration is unavailable")
	}
	cfg := e.source.resolve()
	if cfg == nil {
		return nil, errors.New("model configuration is unavailable")
	}
	modelCfg, modelRef, err := modelConfigForBinding(cfg, binding)
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
	return provider, nil
}

func (e *ProviderModelExecutor) ExecuteModel(ctx context.Context, req ModelRequest) (any, error) {
	provider, err := e.providerForBinding(req.Binding)
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
