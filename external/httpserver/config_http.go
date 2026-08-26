//go:build http

package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// Sentinel failures of the PUT transaction, mapped to distinct HTTP replies.
var (
	errFoxxyCodeConfigUnavailable = errors.New("foxxycode config unavailable")
	errFoxxyCodeConfigParse       = errors.New("foxxycode config parse failed")
	errFoxxyCodeConfigSerialize   = errors.New("foxxycode config serialize failed")
	errFoxxyCodeConfigBackup      = errors.New("foxxycode config backup failed")
	errFoxxyCodeConfigWrite       = errors.New("foxxycode config write failed")
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
	// Reflect the live auth state (config token plus any --auth-token / FOXXYCODE_HTTP_TOKEN).
	dto.HTTPServer.AuthConfigured = s.authPolicyNow().enabled
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
	if _, err := config.ParseConfigJSONPreservingSecrets(body, c.Paths, c); err != nil {
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeFoxxyCodeConfigErr(w, http.StatusBadRequest, "read body")
		return
	}
	// The whole transaction - reading the current config the secret-preserving
	// parse merges into, backup, write, reload, and installing the result into
	// the server and session manager - runs under the process-wide config file
	// lock shared with the agent's config_commit / config_rollback tools.
	// Anything less lets two writers interleave and install runtime state that
	// no longer matches the file.
	var cfgPath string
	txErr := config.WithConfigFileLock(func() error {
		c := s.activeCfg()
		if c == nil {
			return errFoxxyCodeConfigUnavailable
		}
		paths := c.Paths
		cfgPath = paths.ConfigPath
		newCfg, err := config.ParseConfigJSONPreservingSecrets(body, paths, c)
		if err != nil {
			return fmt.Errorf("%w: %s", errFoxxyCodeConfigParse, err.Error())
		}
		yb, err := config.MarshalConfigYAML(newCfg)
		if err != nil {
			return errFoxxyCodeConfigSerialize
		}
		if err := config.BackupCurrent(cfgPath); err != nil {
			s.log.Error("foxxycode config backup", "error", err)
			return errFoxxyCodeConfigBackup
		}
		if err := config.AtomicWriteConfigYAML(cfgPath, yb); err != nil {
			s.log.Error("foxxycode config write", "error", err)
			return errFoxxyCodeConfigWrite
		}
		reloaded, err := config.LoadWithPaths(paths)
		if err != nil {
			s.log.Error("foxxycode config reload after write", "error", err)
			if bak, er2 := os.ReadFile(config.BackupPath(cfgPath)); er2 == nil {
				if er3 := config.AtomicWriteConfigYAML(cfgPath, bak); er3 != nil {
					s.log.Error("foxxycode config rollback", "error", er3)
				}
			}
			return err
		}
		s.ReplaceConfig(reloaded)
		s.mgr.ReplaceConfig(reloaded)
		return nil
	})
	switch {
	case errors.Is(txErr, errFoxxyCodeConfigUnavailable):
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "config unavailable")
		return
	case errors.Is(txErr, errFoxxyCodeConfigParse):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": strings.TrimPrefix(txErr.Error(), errFoxxyCodeConfigParse.Error()+": "),
		})
		return
	case errors.Is(txErr, errFoxxyCodeConfigSerialize):
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "serialize yaml")
		return
	case errors.Is(txErr, errFoxxyCodeConfigBackup):
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "backup failed")
		return
	case errors.Is(txErr, errFoxxyCodeConfigWrite):
		writeFoxxyCodeConfigErr(w, http.StatusInternalServerError, "write failed")
		return
	case txErr != nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": txErr.Error(),
		})
		return
	}
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
