//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/configapi"
)

// registerConfigRoutes mounts the settings form's routes (internal/configapi,
// shared with the swarm relay) and the reasoning levels lookup it uses.
func (s *Server) registerConfigRoutes() {
	s.mux.HandleFunc("GET /foxxycode/config/reasoning-levels", s.foxxycodeConfigReasoningLevelsGet)
	backend := &configapi.Backend{
		Live: s.activeCfg,
		// ReplaceConfig on the manager reaches this server through the config
		// observer registered in New, which is also how a reload from the
		// agent's own config_commit tool or from the console gets here.
		Install: func(c *config.Config) error {
			s.mgr.ReplaceConfig(c)
			return nil
		},
		Schema:    configSchemaForBuild,
		Decorate:  s.decorateConfigDocument,
		Revisions: s.served,
		Log:       s.log,
	}
	backend.Register(s.mux)
}

func configSchemaForBuild() ([]byte, error) {
	doc := config.UISchemaMap()
	markMissingBuildTags(doc)
	return json.Marshal(doc)
}

func (s *Server) foxxycodeConfigSchemaGet(w http.ResponseWriter, r *http.Request) {
	data, err := configSchemaForBuild()
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "schema generation failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// markMissingBuildTags keeps the schema independent of the serving binary's tags.
func markMissingBuildTags(doc map[string]interface{}) {
	props, _ := doc["properties"].(map[string]interface{})
	compiled := map[string]bool{config.BrowserBuildTag: config.BrowserToolCompiled}
	for _, raw := range props {
		section, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := section[config.SchemaRequiresBuildTag].(string)
		if have, known := compiled[tag]; known && !have {
			section[config.SchemaBuildTagMissing] = true
		}
	}
}

// decorateConfigDocument reports the effective auth state (YAML token or
// out-of-band --auth-token / FOXXYCODE_HTTP_TOKEN), not just the config-file token,
// so the UI can reflect that auth is on regardless of source.
func (s *Server) decorateConfigDocument(dto *config.ConfigJSON) {
	pol := s.authPolicyNow()
	dto.HTTPServer.AuthConfigured = len(pol.tokens) > 0
	// The sign-in form is reported the same way and for the same reason: the
	// account may come from the environment, which is nowhere in this document.
	dto.HTTPServer.LoginConfigured = pol.login.enabled && !pol.login.broken
	if dto.HTTPServer.LoginConfigured {
		dto.HTTPServer.LoginSource = pol.login.account.source
	}
	// login.user stays whatever the file says, even when the live account came
	// from the environment: this document is what a save writes back, and an
	// environment credential must never end up in it. Who is signed in is
	// GET /foxxycode/auth/me's answer, not this one's.
}

// writeFoxxyCodeConfigErr answers with the error document of the config routes.
func writeFoxxyCodeConfigErr(w http.ResponseWriter, code int, msg string) {
	configapi.WriteError(w, code, msg)
}
