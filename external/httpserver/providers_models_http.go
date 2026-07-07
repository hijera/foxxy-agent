//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func (s *Server) registerProvidersRoutes() {
	s.mux.HandleFunc("GET /foxxycode/providers/{name}/models", s.foxxycodeProviderModelsGet)
	s.mux.HandleFunc("POST /foxxycode/providers/models-probe", s.foxxycodeProviderModelsProbe)
}

// foxxycodeProviderModelsGet fetches the model list advertised by a configured
// provider's server. The provider is resolved from the active config by name, so
// its credentials (api_key / api_key_command / NAME_API_KEY env) and proxy apply
// without sending secrets over the wire. On a successful upstream call it returns
// {"ok":true,"models":[{"id","name"}]}; on failure it returns
// {"ok":false,"error":...,"models":[]} with HTTP 200 so the UI can fall back to
// manual model entry. An unknown provider name returns 404.
func (s *Server) foxxycodeProviderModelsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	c := s.activeCfg()
	if c == nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	name := r.PathValue("name")
	var prov *config.ProviderConfig
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			prov = &c.Providers[i]
			break
		}
	}
	if prov == nil {
		writeFoxxyCodeConfigErr(w, http.StatusNotFound, "unknown provider")
		return
	}

	models, err := llm.ListModels(r.Context(), llm.ProviderInput{
		Type:     prov.Type,
		APIKey:   prov.EffectiveAPIKey(),
		BaseURL:  prov.APIBase,
		ProxyURL: prov.Proxy,
	})
	writeProviderModelsResult(w, models, err)
}

// foxxycodeProviderModelsProbe fetches the model list for a provider that is not
// saved in the config yet (onboarding): credentials arrive in the request body
// instead of being resolved by provider name. Response shape matches the GET
// variant: {"ok":true,"models":[...]} or {"ok":false,"error":...,"models":[]}
// with HTTP 200 on upstream failure so the UI can fall back to manual entry.
// A malformed body or unsupported provider type returns 400.
func (s *Server) foxxycodeProviderModelsProbe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type    string `json:"type"`
		APIBase string `json:"api_base"`
		APIKey  string `json:"api_key"`
		Proxy   string `json:"proxy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if in.Type != "openai" && in.Type != "anthropic" {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "type must be \"openai\" or \"anthropic\"")
		return
	}

	models, err := llm.ListModels(r.Context(), llm.ProviderInput{
		Type:     in.Type,
		APIKey:   in.APIKey,
		BaseURL:  in.APIBase,
		ProxyURL: in.Proxy,
	})
	writeProviderModelsResult(w, models, err)
}

// writeProviderModelsResult encodes the shared model-listing response shape.
func writeProviderModelsResult(w http.ResponseWriter, models []llm.ModelEntry, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     false,
			"error":  err.Error(),
			"models": []llm.ModelEntry{},
		})
		return
	}
	if models == nil {
		models = []llm.ModelEntry{}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"models": models,
	})
}
