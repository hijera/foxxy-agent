//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func (s *Server) registerOnboardingRoutes() {
	s.mux.HandleFunc("GET /foxxycode/onboarding/status", s.foxxycodeOnboardingStatusGet)
}

type onboardingStatusDTO struct {
	FirstRun        bool     `json:"first_run"`
	HasConfig       bool     `json:"has_config"`
	HasProviders    bool     `json:"has_providers"`
	HasModels       bool     `json:"has_models"`
	HasAgentModel   bool     `json:"has_agent_model"`
	MissingAPIKeys  []string `json:"missing_api_keys"`
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
		FirstRun:       firstRun,
		HasConfig:      hasConfig,
		HasProviders:   hasProviders,
		HasModels:      hasModels,
		HasAgentModel:  hasAgentModel,
		MissingAPIKeys: missing,
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

func missingProviderAPIKeys(c *config.Config) []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, p := range c.Providers {
		key := strings.TrimSpace(p.APIKey)
		if key == "~" {
			continue
		}
		if strings.HasPrefix(key, "${") && strings.HasSuffix(key, "}") {
			continue
		}
		if key != "" {
			continue
		}
		if strings.TrimSpace(p.APIKeyCommand) != "" {
			continue
		}
		out = append(out, p.Name)
	}
	return out
}
