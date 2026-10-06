//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func (s *Server) registerOnboardingRoutes() {
	s.mux.HandleFunc("GET /foxxycode/onboarding/status", s.foxxycodeOnboardingStatusGet)
}

type onboardingStatusDTO struct {
	FirstRun      bool `json:"first_run"`
	HasConfig     bool `json:"has_config"`
	HasProviders  bool `json:"has_providers"`
	HasModels     bool `json:"has_models"`
	HasAgentModel bool `json:"has_agent_model"`
	// HasAgentCredentials is true when the provider named by agent.model has at
	// least one credential source (see providerHasCredentials). This is what the
	// SPA gates the provider picker on: a keyless provider the agent does not use
	// must not reopen onboarding on every save and start.
	HasAgentCredentials bool `json:"has_agent_credentials"`
	// MissingAPIKeys lists every provider with no credential source at all.
	// Informational; the picker is not gated on it.
	MissingAPIKeys    []string               `json:"missing_api_keys"`
	SuggestedDefaults map[string]interface{} `json:"suggested_defaults,omitempty"`
}

func (s *Server) foxxycodeOnboardingStatusGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	c := s.activeCfg()
	if c == nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	cfgPath := strings.TrimSpace(c.Paths.ConfigPath)
	_, statErr := os.Stat(cfgPath)
	hasConfig := statErr == nil
	firstRun := !hasConfig

	hasProviders := len(c.Providers) > 0
	hasModels := len(c.Models) > 0
	hasAgentModel := strings.TrimSpace(c.Agent.Model) != ""

	missing := missingProviderAPIKeys(c)

	dto := onboardingStatusDTO{
		FirstRun:            firstRun,
		HasConfig:           hasConfig,
		HasProviders:        hasProviders,
		HasModels:           hasModels,
		HasAgentModel:       hasAgentModel,
		HasAgentCredentials: agentProviderHasCredentials(c),
		MissingAPIKeys:      missing,
		SuggestedDefaults: map[string]interface{}{
			"provider_name": "openai",
			"provider_type": "openai",
			"model":         "openai/gpt-4o",
			"max_tokens":    8192,
			"temperature":   0.2,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dto); err != nil {
		s.log.Error("foxxycode onboarding status encode", "error", err)
	}
}

// missingProviderAPIKeys lists the providers that have no credential source at
// all. Informational only; see providerHasCredentials for what counts.
func missingProviderAPIKeys(c *config.Config) []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, p := range c.Providers {
		if providerHasCredentials(c.Paths.Home, p) {
			continue
		}
		out = append(out, p.Name)
	}
	return out
}

// agentProviderHasCredentials reports whether the provider named by agent.model
// exists and has a credential source. False when agent.model is empty or names
// a provider that is not configured: either way the agent cannot run a turn and
// the provider picker is the right thing to show.
func agentProviderHasCredentials(c *config.Config) bool {
	if c == nil {
		return false
	}
	name, _, err := config.SplitModelRef(c.Agent.Model)
	if err != nil {
		return false
	}
	prov := c.FindProvider(name)
	if prov == nil {
		return false
	}
	return providerHasCredentials(c.Paths.Home, *prov)
}

// providerHasCredentials mirrors the resolution order of
// config.ProviderConfig.EffectiveAPIKey plus the managed OAuth logins, but
// without executing anything: a non-empty api_key (a literal, "~", or a
// "${VAR}" reference that failed to expand all count - the user did configure
// one), an api_key_command (never run here - the status is read on every UI
// start and must not spawn a shell), the conventional NAME_API_KEY variable, or
// a stored hub / ChatGPT login for the neuraldeep and codex provider types.
func providerHasCredentials(home string, p config.ProviderConfig) bool {
	if strings.TrimSpace(p.APIKey) != "" {
		return true
	}
	if strings.TrimSpace(p.APIKeyCommand) != "" {
		return true
	}
	if env := config.ProviderAPIKeyEnvVarName(p.Name); env != "" && strings.TrimSpace(os.Getenv(env)) != "" {
		return true
	}
	switch strings.TrimSpace(p.Type) {
	case "neuraldeep":
		st, err := llm.InspectNeuralDeepAuth(config.NeuralDeepAuthPath(home, p.Name))
		return err == nil && st.Connected
	case "codex":
		st, err := llm.InspectCodexAuth(config.CodexAuthPath(home, p.Name))
		return err == nil && st.Connected
	}
	return false
}
