//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func (s *Server) registerProvidersRoutes() {
	s.mux.HandleFunc("GET /foxxycode/providers/{name}/models", s.foxxycodeProviderModelsGet)
	s.mux.HandleFunc("POST /foxxycode/providers/models-probe", s.foxxycodeProviderModelsProbe)
	s.mux.HandleFunc("POST /foxxycode/providers/models", s.foxxycodeProviderModelsPost)
	s.registerCodexAuthRoutes()
	s.registerNeuralDeepAuthRoutes()
	s.registerProviderUsageRoutes()
}

// foxxycodeProviderModelsGet fetches the model list advertised by a configured
// provider's server. The provider is resolved from the active config by name, so
// its credentials (api_key / api_key_command / NAME_API_KEY env) and proxy apply
// without sending secrets over the wire. On a successful upstream call it returns
// {"ok":true,"models":[{"id","name","context_window"}]}; on failure it returns
// {"ok":false,"error":...,"models":[]} with HTTP 200 so the UI can fall back to
// manual model entry. An unknown provider name returns 404.
func (s *Server) foxxycodeProviderModelsGet(w http.ResponseWriter, r *http.Request) {
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
	s.writeProviderModels(w, r.Context(), c, prov)
}

// providerModelsRequest is the body of POST /foxxycode/providers/models: a provider
// row as the settings form edits it. The entry being edited has not necessarily
// been saved yet - the sign-in endpoints accept such providers for the same
// reason - so the row travels in the body rather than being looked up in the
// saved config.
type providerModelsRequest struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	APIBase       string `json:"api_base"`
	APIKey        string `json:"api_key"`
	APIKeyCommand string `json:"api_key_command"`
	Proxy         string `json:"proxy"`
}

// foxxycodeProviderModelsPost fetches the model list for a provider description
// posted in the request body. Fields the body leaves empty are inherited from
// the saved provider of the same name when there is one, so a sparse
// {"name": "..."} post resolves stored credentials without secrets travelling
// over the wire; fields the body carries override the saved row, so the form
// previews the provider as it is being edited. The credential pair
// (api_key / api_key_command) is one slot: it is inherited only when the body
// posts neither field, and only while the resolved api_base and proxy still
// match the saved row, so an overridden route never receives stored
// credentials. A
// posted api_key_command is executed server-side, exactly as it would be for a
// saved provider. Only an application/json body is accepted (415 otherwise),
// which keeps a cross-site page from driving this route through a browser.
// Responses follow the GET shape: {"ok":true,"models":[...]} on success,
// {"ok":false,"error":...} with HTTP 200 on an upstream failure, 400 for a
// malformed or invalid body.
func (s *Server) foxxycodeProviderModelsPost(w http.ResponseWriter, r *http.Request) {
	// Only a JSON body is read: an unauthenticated loopback server would
	// otherwise run the posted api_key_command, or send a stored key
	// upstream, for a page on another site (see requireJSONRequest).
	if !requireJSONRequest(w, r) {
		return
	}
	c := s.activeCfg()
	if c == nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	var req providerModelsRequest
	// A provider row is a few hundred bytes; nothing needs more than this.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	prov := config.ProviderConfig{
		Name:          strings.TrimSpace(req.Name),
		Type:          req.Type,
		APIBase:       req.APIBase,
		APIKey:        req.APIKey,
		APIKeyCommand: req.APIKeyCommand,
		Proxy:         req.Proxy,
	}
	if saved := c.FindProvider(prov.Name); saved != nil {
		if strings.TrimSpace(prov.Type) == "" {
			prov.Type = saved.Type
		}
		if strings.TrimSpace(prov.APIBase) == "" {
			prov.APIBase = saved.APIBase
		}
		if strings.TrimSpace(prov.Proxy) == "" {
			prov.Proxy = saved.Proxy
		}
		// The credential pair is one slot: it is inherited only when the body
		// posts neither field, and only while the request still takes the
		// saved route - the saved endpoint through the saved proxy. A caller
		// overriding either must post the credentials for it, otherwise the
		// stored key would go to a URL, or through a proxy, it was never
		// configured for.
		if strings.TrimSpace(prov.APIBase) == strings.TrimSpace(saved.APIBase) &&
			strings.TrimSpace(prov.Proxy) == strings.TrimSpace(saved.Proxy) &&
			strings.TrimSpace(prov.APIKey) == "" &&
			strings.TrimSpace(prov.APIKeyCommand) == "" {
			prov.APIKey = saved.APIKey
			prov.APIKeyCommand = saved.APIKeyCommand
		}
	}
	prov.Normalize()
	if err := prov.Validate(); err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeProviderModels(w, r.Context(), c, &prov)
}

// writeProviderModels lists the models a provider advertises and writes the
// {"ok","models"} answer shared by the GET and POST routes.
func (s *Server) writeProviderModels(w http.ResponseWriter, ctx context.Context, c *config.Config, prov *config.ProviderConfig) {
	models, err := llm.ListModels(ctx, llm.ProviderInput{
		Name:     prov.Name,
		Type:     prov.Type,
		APIKey:   prov.EffectiveAPIKeyContext(ctx),
		BaseURL:  prov.APIBase,
		ProxyURL: prov.Proxy,
		AuthPath: config.ProviderAuthPath(c.Paths.Home, prov.Name, prov.Type),
		// An unsaved row counts toward the rows of its type too, so the
		// preview uses the account a saved row would.
		NoCLILogin: !c.ProviderMayUseCLILogin(prov.Name, prov.Type),
	})
	writeProviderModelsResult(w, models, err)
}

// foxxycodeProviderModelsProbe fetches the model list for a provider that is not
// saved in the config yet (onboarding): API credentials arrive in the request
// body instead of being resolved by provider name. Codex is the exception:
// provider_name resolves its server-side OAuth file, so the browser never handles
// the token. Response shape matches the GET variant:
// {"ok":true,"models":[...]} or {"ok":false,"error":...,"models":[]} with HTTP
// 200 on upstream failure so the UI can fall back to manual entry.
func (s *Server) foxxycodeProviderModelsProbe(w http.ResponseWriter, r *http.Request) {
	// The body names an api_base the server then contacts, so this route starts
	// work on the server exactly as the sign-in ones do (see requireJSONRequest).
	if !requireJSONRequest(w, r) {
		return
	}
	var in struct {
		Type         string `json:"type"`
		ProviderName string `json:"provider_name"`
		APIBase      string `json:"api_base"`
		APIKey       string `json:"api_key"`
		Proxy        string `json:"proxy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if _, ok := config.AllowedLLMProviderTypes[in.Type]; !ok {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "type must be \"openai\", \"anthropic\", \"neuraldeep\", or \"codex\"")
		return
	}

	providerInput := llm.ProviderInput{
		Type:     in.Type,
		APIKey:   in.APIKey,
		BaseURL:  in.APIBase,
		ProxyURL: in.Proxy,
	}
	// Both managed-credential types are probed before the provider exists in
	// config.yaml: onboarding signs in first and lists models second, so the
	// catalog has to be readable from the stored credential alone. A neuraldeep
	// probe that carries its own api_key needs no stored login, and therefore no
	// provider name either - the explicit key wins in NewProvider as well.
	if in.Type == "codex" || (in.Type == "neuraldeep" && strings.TrimSpace(in.APIKey) == "") {
		probe := config.ProviderConfig{Name: strings.TrimSpace(in.ProviderName), Type: in.Type}
		probe.Normalize()
		if err := probe.Validate(); err != nil {
			writeFoxxyCodeConfigErr(w, http.StatusBadRequest, err.Error())
			return
		}
		c := s.activeCfg()
		if c == nil || strings.TrimSpace(c.Paths.Home) == "" {
			writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config home unavailable")
			return
		}
		providerInput.AuthPath = config.ProviderAuthPath(c.Paths.Home, probe.Name, probe.Type)
		providerInput.NoCLILogin = !c.ProviderMayUseCLILogin(probe.Name, probe.Type)
	}

	models, err := llm.ListModels(r.Context(), providerInput)
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
