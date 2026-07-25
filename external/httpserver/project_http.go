//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/project"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// FolderPickerFunc opens a native folder dialog and returns the chosen
// path, cancelled=true when the user dismissed it, or an error.
type FolderPickerFunc func(ctx context.Context) (path string, cancelled bool, err error)

// AttachProjectStore wires the persisted current-project / recent-projects
// store. Without it PUT /foxxycode/project responds 503 and GET falls back
// to the process default cwd.
func (s *Server) AttachProjectStore(ps *project.Store) {
	s.projects = ps
}

// SetFolderPicker installs the native folder dialog hook (desktop mode).
// A nil hook keeps POST /foxxycode/project/pick-folder at 501.
func (s *Server) SetFolderPicker(fn FolderPickerFunc) {
	s.folderPicker = fn
}

// sessionDefaultCWD is the working directory for sessions that do not
// carry their own: the current project when one is set, else the process
// default cwd resolved at startup.
func (s *Server) sessionDefaultCWD() string {
	if s.projects != nil {
		if p := s.projects.Current(); p != "" {
			return p
		}
	}
	return s.defaultCWD
}

func (s *Server) registerProjectRoutes() {
	s.mux.HandleFunc("GET /foxxycode/project", s.foxxycodeProjectGet)
	s.mux.HandleFunc("PUT /foxxycode/project", s.foxxycodeProjectPut)
	s.mux.HandleFunc("GET /foxxycode/projects/recent", s.foxxycodeProjectsRecentGet)
	s.mux.HandleFunc("POST /foxxycode/project/pick-folder", s.foxxycodeProjectPickFolder)
	s.mux.HandleFunc("GET /foxxycode/project/last-session", s.foxxycodeProjectLastSessionGet)
	s.mux.HandleFunc("PUT /foxxycode/project/last-session", s.foxxycodeProjectLastSessionPut)
}

func (s *Server) writeProjectDTO(w http.ResponseWriter) {
	path := s.defaultCWD
	source := "default"
	if s.projects != nil {
		if p := s.projects.Current(); p != "" {
			path = p
			source = "project"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":        "foxxycode.project",
		"path":          path,
		"source":        source,
		"native_picker": s.folderPicker != nil,
	})
}

func (s *Server) foxxycodeProjectGet(w http.ResponseWriter, r *http.Request) {
	s.writeProjectDTO(w)
}

func (s *Server) foxxycodeProjectPut(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		http.Error(w, `{"error":{"message":"project store unavailable"}}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	if err := s.projects.SetCurrent(body.Path); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	s.log.Info("project switched", "path", s.projects.Current())
	s.writeProjectDTO(w)
}

func (s *Server) foxxycodeProjectsRecentGet(w http.ResponseWriter, r *http.Request) {
	type recentDTO struct {
		Path         string `json:"path"`
		Name         string `json:"name"`
		LastOpenedAt string `json:"last_opened_at"`
		Exists       bool   `json:"exists"`
	}
	data := make([]recentDTO, 0)
	if s.projects != nil {
		for _, e := range s.projects.Recent() {
			exists := false
			if info, err := os.Stat(e.Path); err == nil && info.IsDir() {
				exists = true
			}
			data = append(data, recentDTO{
				Path:         e.Path,
				Name:         filepath.Base(e.Path),
				LastOpenedAt: e.LastOpenedAt,
				Exists:       exists,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   data,
	})
}

// writeLastSessionDTO reports the session an editor plugin should reopen for
// the current project. The id is validated against the store on the way out:
// a deleted session, or one that has since moved to another project, would
// otherwise route the SPA to an empty chat with no error.
func (s *Server) writeLastSessionDTO(w http.ResponseWriter) {
	scope := s.sessionDefaultCWD()
	id := ""
	if s.projects != nil {
		id = strings.TrimSpace(s.projects.LastSession(scope))
	}
	if id != "" && !s.lastSessionUsable(id, scope) {
		id = ""
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":     "foxxycode.project_last_session",
		"path":       scope,
		"session_id": id,
	})
}

// lastSessionUsable reports whether id still names a persisted, non-scheduler
// session whose working directory belongs to scope.
func (s *Server) lastSessionUsable(id, scope string) bool {
	if session.ValidateFolderSessionID(id) != nil {
		return false
	}
	fs := s.mgr.FileStore()
	if fs == nil || fs.Root == "" {
		return false
	}
	meta, err := fs.ReadMeta(id)
	if err != nil {
		return false
	}
	if meta.ExcludedFromComposerSessionList(id) {
		return false
	}
	return session.CWDInScope(meta.CWD, scope)
}

func (s *Server) foxxycodeProjectLastSessionGet(w http.ResponseWriter, r *http.Request) {
	s.writeLastSessionDTO(w)
}

func (s *Server) foxxycodeProjectLastSessionPut(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		http.Error(w, `{"error":{"message":"project store unavailable"}}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	// An empty id clears the record: the user went back to a new chat.
	id := strings.TrimSpace(body.SessionID)
	if id != "" {
		if err := session.ValidateFolderSessionID(id); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
			return
		}
	}
	if err := s.projects.SetLastSession(s.sessionDefaultCWD(), id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	s.writeLastSessionDTO(w)
}

func (s *Server) foxxycodeProjectPickFolder(w http.ResponseWriter, r *http.Request) {
	picker := s.folderPicker
	if picker == nil {
		http.Error(w, `{"error":{"message":"native folder picker not available"}}`, http.StatusNotImplemented)
		return
	}
	if !s.pickerBusy.CompareAndSwap(false, true) {
		http.Error(w, `{"error":{"message":"folder dialog already open"}}`, http.StatusConflict)
		return
	}
	defer s.pickerBusy.Store(false)
	path, cancelled, err := picker(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.project_pick",
		"cancelled": cancelled,
		"path":      path,
	})
}
