//go:build http

package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/EvilFreelancer/coddy-agent/internal/skills"
)

func (s *Server) registerSkillsManagementRoutes() {
	s.mux.HandleFunc("GET /coddy/skills", s.coddySkillsGet)
	s.mux.HandleFunc("POST /coddy/skills/{name}/enable", s.coddySkillsEnablePost)
	s.mux.HandleFunc("POST /coddy/skills/{name}/disable", s.coddySkillsDisablePost)
}

type skillRowResponse struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	FilePath    string   `json:"file_path"`
	AlwaysApply bool     `json:"always_apply"`
	Globs       []string `json:"globs,omitempty"`
	Disabled    bool     `json:"disabled"`
}

// coddySkillsGet lists all skills with their enabled/disabled state.
func (s *Server) coddySkillsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	cfg := s.activeCfg()
	installDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	loader := skills.NewLoader(cfg.Skills.Dirs)

	allLoaded, err := loader.LoadAll(s.defaultCWD, cfg.Paths.Home)
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
			Disabled:    skills.IsDisabled(disabled, sum.Name),
		}
		if sk != nil {
			row.FilePath = sk.FilePath
			row.AlwaysApply = sk.AlwaysApply
			row.Globs = sk.Globs
		}
		rows = append(rows, row)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "coddy.skills_list",
		"items":  rows,
	})
}

// coddySkillsEnablePost removes a skill from the disabled list.
func (s *Server) coddySkillsEnablePost(w http.ResponseWriter, r *http.Request) {
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

// coddySkillsDisablePost adds a skill to the disabled list.
func (s *Server) coddySkillsDisablePost(w http.ResponseWriter, r *http.Request) {
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

