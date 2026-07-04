//go:build http

package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func (s *Server) registerConfigRoutes() {
	s.mux.HandleFunc("GET /foxxycode/config/schema", s.foxxycodeConfigSchemaGet)
	s.mux.HandleFunc("GET /foxxycode/config", s.foxxycodeConfigGet)
	s.mux.HandleFunc("POST /foxxycode/config/validate", s.foxxycodeConfigValidatePost)
	s.mux.HandleFunc("PUT /foxxycode/config", s.foxxycodeConfigPut)
}

func (s *Server) foxxycodeConfigSchemaGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	data, err := config.UISchemaJSON()
	if err != nil {
		s.log.Error("foxxycode config schema", "error", err)
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "schema generation failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Server) foxxycodeConfigGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	c := s.activeCfg()
	if c == nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	dto := config.ConfigToJSONDTO(c)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dto); err != nil {
		s.log.Error("foxxycode config get encode", "error", err)
	}
}

func (s *Server) foxxycodeConfigValidatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	c := s.activeCfg()
	if c == nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "read body")
		return
	}
	if _, err := config.ParseAndValidateConfigJSON(body, c.Paths); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) foxxycodeConfigPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.NotFound(w, r)
		return
	}
	c := s.activeCfg()
	if c == nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config unavailable")
		return
	}
	paths := c.Paths
	cfgPath := paths.ConfigPath
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "read body")
		return
	}
	newCfg, err := config.ParseAndValidateConfigJSON(body, paths)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	yb, err := config.MarshalConfigYAML(newCfg)
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "serialize yaml")
		return
	}
	if err := config.BackupCurrent(cfgPath); err != nil {
		s.log.Error("foxxycode config backup", "error", err)
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "backup failed")
		return
	}
	if err := config.AtomicWriteConfigYAML(cfgPath, yb); err != nil {
		s.log.Error("foxxycode config write", "error", err)
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "write failed")
		return
	}
	reloaded, err := config.LoadWithPaths(paths)
	if err != nil {
		s.log.Error("foxxycode config reload after write", "error", err)
		if bak, er2 := os.ReadFile(config.BackupPath(cfgPath)); er2 == nil {
			if er3 := config.AtomicWriteConfigYAML(cfgPath, bak); er3 != nil {
				s.log.Error("foxxycode config rollback", "error", er3)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	s.ReplaceConfig(reloaded)
	s.mgr.ReplaceConfig(reloaded)
	s.log.Info("config updated", "path", cfgPath)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func writeFoxxyCodeConfigErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    false,
		"error": msg,
	})
}
