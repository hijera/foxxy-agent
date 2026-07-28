//go:build miniapps

package miniapps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type ProviderFactory func(llm.ProviderInput) (llm.Provider, error)

const neuralDeepProviderBaseURL = "https://api.neuraldeep.ru/v1"

type ConfigModelExecutor struct {
	cfg     *config.Config
	factory ProviderFactory
}

func NewConfigModelExecutor(cfg *config.Config, factory ProviderFactory) *ConfigModelExecutor {
	if factory == nil {
		factory = llm.NewProvider
	}
	return &ConfigModelExecutor{cfg: cfg, factory: factory}
}

func BindingForConfiguredModel(cfg *config.Config, modelRef, id string) (*ModelBinding, error) {
	if cfg == nil {
		return nil, errorsConfigUnavailable()
	}
	if strings.TrimSpace(modelRef) == "" {
		modelRef = cfg.Agent.Model
	}
	entry := cfg.FindModelEntry(modelRef)
	if entry == nil {
		return nil, fmt.Errorf("model %q is not configured", modelRef)
	}
	provider := cfg.FindProvider(entry.ProviderName())
	if provider == nil {
		return nil, fmt.Errorf("provider %q is not configured", entry.ProviderName())
	}
	if id == "" {
		id = "primary"
	}
	scope := "remote"
	if isLoopbackURL(provider.APIBase) {
		scope = "local"
	}
	return &ModelBinding{
		ID: id, Selection: "fixed",
		Provider: ProviderIdentity{
			Type: provider.Type, BaseURL: canonicalBaseURL(configuredProviderBaseURL(provider)), Scope: scope,
		},
		Model: entry.APIModel(), Credentials: CredentialBinding{Source: "matched_provider"},
	}, nil
}

func (e *ConfigModelExecutor) ExecuteModelStep(ctx context.Context, binding ModelBinding, prompt string) (any, error) {
	resolved, err := e.resolve(binding)
	if err != nil {
		return nil, err
	}
	if err := e.ensureLocalModel(ctx, binding, resolved); err != nil {
		return nil, err
	}
	provider, err := e.factory(llm.ProviderInput{
		Type: resolved.ProviderType, Model: resolved.Model, APIKey: resolved.APIKey,
		BaseURL: resolved.BaseURL, ProxyURL: resolved.ProxyURL,
		MaxTokens: resolved.MaxTokens, Temperature: resolved.Temperature,
	})
	if err != nil {
		return nil, err
	}
	response, err := provider.Complete(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: "Execute the reviewed mini-app step. Return only the declared operator result; never expose internal reasoning."},
		{Role: llm.RoleUser, Content: prompt},
	}, nil)
	if err != nil {
		return nil, err
	}
	return response.Content, nil
}

func (e *ConfigModelExecutor) ensureLocalModel(ctx context.Context, binding ModelBinding, resolved *config.ResolvedLLM) error {
	bootstrap := binding.LocalBootstrap
	if bootstrap == nil || !bootstrap.Connect {
		return nil
	}
	if binding.Provider.Scope != "local" || !isLoopbackURL(resolved.BaseURL) {
		return errorsConfigUnavailableForLocal("local model bootstrap requires a loopback provider")
	}
	timeout := bootstrap.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	available, probeErr := probeLocalModels(probeCtx, client, resolved)
	if probeErr == nil && available[resolved.Model] {
		return nil
	}
	if bootstrap.EnsureModel != "pull_if_missing" {
		if probeErr != nil {
			return fmt.Errorf("connect local model provider: %w", probeErr)
		}
		return fmt.Errorf("local model %q is not available", resolved.Model)
	}
	if !strings.EqualFold(binding.Provider.Adapter, "ollama") {
		return fmt.Errorf("local model %q is missing and adapter %q has no reviewed pull protocol",
			resolved.Model, binding.Provider.Adapter)
	}
	if err := pullOllamaModel(probeCtx, client, resolved); err != nil {
		return err
	}
	available, err := probeLocalModels(probeCtx, client, resolved)
	if err != nil {
		return fmt.Errorf("verify local model after pull: %w", err)
	}
	if !available[resolved.Model] {
		return fmt.Errorf("local model %q is unavailable after pull", resolved.Model)
	}
	return nil
}

func probeLocalModels(ctx context.Context, client *http.Client, resolved *config.ResolvedLLM) (map[string]bool, error) {
	endpoint, err := modelEndpoint(resolved.BaseURL)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if resolved.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+resolved.APIKey)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("model probe returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, model := range payload.Data {
		out[model.ID] = true
	}
	for _, model := range payload.Models {
		out[model.ID] = true
		out[model.Name] = true
		out[model.Model] = true
	}
	return out, nil
}

func pullOllamaModel(ctx context.Context, client *http.Client, resolved *config.ResolvedLLM) error {
	parsed, err := url.Parse(resolved.BaseURL)
	if err != nil {
		return err
	}
	parsed.Path = "/api/pull"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	body, _ := json.Marshal(map[string]any{"model": resolved.Model, "stream": false})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("pull local Ollama model: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("pull local Ollama model returned HTTP %d: %s",
			response.StatusCode, strings.TrimSpace(string(message)))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8<<20))
	return nil
}

func modelEndpoint(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid local provider base URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/models"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (e *ConfigModelExecutor) resolve(binding ModelBinding) (*config.ResolvedLLM, error) {
	if e == nil || e.cfg == nil {
		return nil, errorsConfigUnavailable()
	}
	if binding.Selection == "capability" {
		return e.cfg.ResolveLLM(e.cfg.Agent.Model)
	}
	if binding.Selection != "fixed" {
		return nil, fmt.Errorf("unsupported model selection %q", binding.Selection)
	}
	wantURL := canonicalBaseURL(binding.Provider.BaseURL)
	for _, entry := range e.cfg.Models {
		if entry.APIModel() != binding.Model {
			continue
		}
		provider := e.cfg.FindProvider(entry.ProviderName())
		if provider == nil || canonicalBaseURL(configuredProviderBaseURL(provider)) != wantURL {
			continue
		}
		if requiresProtocolMatch(binding.Provider.Type) &&
			!strings.EqualFold(provider.Type, binding.Provider.Type) {
			continue
		}
		if binding.Provider.Adapter != "" && !strings.EqualFold(binding.Provider.Adapter, provider.Type) {
			continue
		}
		return e.cfg.ResolveLLM(entry.Model)
	}
	return nil, fmt.Errorf("fixed provider/model is unavailable: %s %s at %s",
		binding.Provider.Type, binding.Model, wantURL)
}

func configuredProviderBaseURL(provider *config.ProviderConfig) string {
	if provider != nil && strings.EqualFold(provider.Type, "neuraldeep") {
		return neuralDeepProviderBaseURL
	}
	if provider == nil {
		return ""
	}
	return provider.APIBase
}

func canonicalBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if isLoopbackHost(host) {
		host = "localhost"
	}
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else {
		parsed.Host = host
	}
	parsed.Path = strings.TrimRight(path.Clean("/"+parsed.Path), "/")
	if parsed.Path == "." || parsed.Path == "/" {
		parsed.Path = ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func isLoopbackURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && isLoopbackHost(strings.ToLower(parsed.Hostname()))
}

func isLoopbackHost(host string) bool {
	if host == "localhost" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requiresProtocolMatch(providerType string) bool {
	return strings.EqualFold(providerType, "openai") || strings.EqualFold(providerType, "anthropic")
}

func errorsConfigUnavailable() error {
	return fmt.Errorf("FoxxyCode provider configuration is unavailable")
}

func errorsConfigUnavailableForLocal(message string) error {
	return fmt.Errorf("FoxxyCode local provider configuration is unavailable: %s", message)
}
