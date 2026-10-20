//go:build http && !miniapps

package httpserver

import (
	"encoding/json"
	"net/http"
)

// registerMiniAppsRoutes intentionally leaves the Mini Apps API absent from a
// lean HTTP build. The capability route remains available so clients can
// hide optional UI without probing every feature route.
func (s *Server) registerMiniAppsRoutes() {}

func (s *Server) miniAppsDrain() {}

func (s *Server) foxxycodeCapabilitiesGet(w http.ResponseWriter, _ *http.Request) {
	writeMiniAppsJSONStub(w, http.StatusOK, map[string]any{
		"object":       "foxxycode.capabilities",
		"capabilities": map[string]bool{"miniapps": false},
	})
}

func writeMiniAppsJSONStub(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
