//go:build http

package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/skills"
)

func (s *Server) registerSkillsManagementRoutes() {
	s.mux.HandleFunc("GET /foxxycode/skills", s.foxxycodeSkillsGet)
	s.mux.HandleFunc("POST /foxxycode/skills/{name}/enable", s.foxxycodeSkillsEnablePost)
	s.mux.HandleFunc("POST /foxxycode/skills/{name}/disable", s.foxxycodeSkillsDisablePost)
}

type skillRowResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
	Enabled     bool   `json:"enabled"`
}

// foxxycodeSkillsGet lists all skills with their enabled/disabled state.
func (s *Server) foxxycodeSkillsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	cfg := s.activeCfg()
	installDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	loader := skills.NewLoader(cfg.Skills.Dirs)

	allLoaded, err := loader.LoadAll(s.sessionDefaultCWD(), cfg.Paths.Home)
	if err != nil {
		http.Error(w, `{"error":{"message":"failed to load skills"}}`, http.StatusInternalServerError)
		return
	}
	disabled := skills.ReadDisabled(installDir)
	sums := skills.ListSkills(allLoaded)

	byName := make(map[string]*skills.Skill, len(allLoaded))
	for _, sk := range allLoaded {
		n := skills.CanonicalCommandName(sk)
		if _, ok := byName[n]; !ok {
			byName[n] = sk
		}
	}

	rows := make([]skillRowResponse, 0, len(sums))
	for _, sum := range sums {
		sk := byName[sum.Name]
		row := skillRowResponse{
			Name:        sum.Name,
			Description: sum.Description,
			Enabled:     !skills.IsDisabled(disabled, sum.Name),
		}
		if sk != nil {
			row.FilePath = sk.FilePath
		}
		rows = append(rows, row)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "foxxycode.skills_list",
		"items":  rows,
	})
}

// foxxycodeSkillsEnablePost removes a skill from the disabled list.
func (s *Server) foxxycodeSkillsEnablePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if err := skills.Enable(s.activeCfg(), name); err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	slog.Info("skill enabled", "name", name)
	s.slashMu.Lock()
	s.slashCache = make(map[string]slashListCacheEntry)
	s.slashMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// foxxycodeSkillsDisablePost adds a skill to the disabled list.
func (s *Server) foxxycodeSkillsDisablePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if err := skills.Disable(s.activeCfg(), name); err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	slog.Info("skill disabled", "name", name)
	s.slashMu.Lock()
	s.slashCache = make(map[string]slashListCacheEntry)
	s.slashMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

