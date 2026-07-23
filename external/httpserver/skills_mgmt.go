//go:build http

package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/skills"
)

func (s *Server) registerSkillsManagementRoutes() {
	s.mux.HandleFunc("GET /foxxycode/skills", s.foxxycodeSkillsGet)
	s.mux.HandleFunc("GET /foxxycode/skills/updates", s.foxxycodeSkillsUpdatesGet)
	s.mux.HandleFunc("GET /foxxycode/skills/available", s.foxxycodeSkillsAvailableGet)
	s.mux.HandleFunc("GET /foxxycode/skills/sources", s.foxxycodeSkillsSourcesGet)
	s.mux.HandleFunc("POST /foxxycode/skills/install", s.foxxycodeSkillsInstallPost)
	s.mux.HandleFunc("POST /foxxycode/skills/{name}/enable", s.foxxycodeSkillsEnablePost)
	s.mux.HandleFunc("POST /foxxycode/skills/{name}/disable", s.foxxycodeSkillsDisablePost)
	s.mux.HandleFunc("POST /foxxycode/skills/{name}/update", s.foxxycodeSkillsUpdatePost)
	s.mux.HandleFunc("POST /foxxycode/skills/sync", s.foxxycodeSkillsSyncPost)
	s.mux.HandleFunc("POST /foxxycode/skills/sources", s.foxxycodeSkillsSourcesPost)
	s.mux.HandleFunc("DELETE /foxxycode/skills/sources", s.foxxycodeSkillsSourcesDelete)
	s.mux.HandleFunc("DELETE /foxxycode/skills/{name}", s.foxxycodeSkillsDelete)
}

type skillRowResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
	Enabled     bool   `json:"enabled"`
	Version     string `json:"version,omitempty"` // installed version, when known
	Source      string `json:"source,omitempty"`  // configured source string when remote-synced
	Readonly    bool   `json:"readonly"`          // bundled skills cannot be deleted
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
	remote := skills.RemoteSources(cfg)
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
			Version:     skills.InstalledVersion(remote, sum.Name, sk),
		}
		if sk != nil {
			row.FilePath = sk.FilePath
			row.Readonly = skills.SkillReadonly(sk)
		}
		if ent, ok := remote[sum.Name]; ok {
			row.Source = ent.Source
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

// foxxycodeSkillsSyncPost fetches all configured skill sources and materializes them.
func (s *Server) foxxycodeSkillsSyncPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	// Optional ?source=<src>: sync only that marketplace; otherwise sync all.
	var res *skills.SyncResult
	var err error
	if src := strings.TrimSpace(r.URL.Query().Get("source")); src != "" {
		res, err = skills.SyncSource(r.Context(), s.activeCfg(), src)
	} else {
		res, err = skills.Sync(r.Context(), s.activeCfg())
	}
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	s.invalidateSlashCache()
	slog.Info("skills synced", "added", len(res.Added), "updated", len(res.Updated), "failed", len(res.Failed))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"added":   res.Added,
		"updated": res.Updated,
		"failed":  res.Failed,
	})
}

type skillSourceRequest struct {
	Source string `json:"source"`
	Sync   bool   `json:"sync"`
}

// foxxycodeSkillsSourcesPost adds a remote source to skills.sources (and optionally syncs).
func (s *Server) foxxycodeSkillsSourcesPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req skillSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
		return
	}
	cfg := s.activeCfg()
	added, err := skills.AddSource(cfg, req.Source)
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	// AddSource persisted config.yaml; reload so the running server sees it.
	s.reloadConfigFromDisk()

	resp := map[string]interface{}{"ok": true, "added": added}
	if req.Sync {
		res, err := skills.Sync(r.Context(), s.activeCfg())
		if err != nil {
			body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
			http.Error(w, string(body), http.StatusInternalServerError)
			return
		}
		s.invalidateSlashCache()
		resp["sync"] = map[string]interface{}{"added": res.Added, "updated": res.Updated, "failed": res.Failed}
	}
	slog.Info("skill source added", "source", req.Source, "added", added)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// foxxycodeSkillsDelete removes a remote (synced) skill by name.
func (s *Server) foxxycodeSkillsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if err := skills.DeleteSkill(s.activeCfg(), s.sessionDefaultCWD(), name); err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	s.invalidateSlashCache()
	slog.Info("skill deleted", "name", name)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// foxxycodeSkillsUpdatesGet reports, per installed remote skill, whether a newer
// version is available in its marketplace source (performs network/git access).
func (s *Server) foxxycodeSkillsUpdatesGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	statuses, err := skills.CheckUpdates(r.Context(), s.activeCfg())
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "foxxycode.skills_updates",
		"items":  statuses,
	})
}

// foxxycodeSkillsAvailableGet lists installable plugins advertised by the configured
// marketplaces (network / git access), each flagged with whether it is already
// installed. Backs the browse/filter install control.
func (s *Server) foxxycodeSkillsAvailableGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	items, err := skills.AvailablePlugins(r.Context(), s.activeCfg(), s.sessionDefaultCWD())
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "foxxycode.skills_available",
		"items":  items,
	})
}

type skillInstallRequest struct {
	Source string `json:"source"`
	Plugin string `json:"plugin"`
}

// foxxycodeSkillsInstallPost installs a single plugin from a marketplace source.
func (s *Server) foxxycodeSkillsInstallPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req skillInstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
		return
	}
	res, err := skills.InstallPlugin(r.Context(), s.activeCfg(), req.Source, req.Plugin)
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	s.invalidateSlashCache()
	slog.Info("plugin installed", "source", req.Source, "plugin", req.Plugin, "added", len(res.Added), "updated", len(res.Updated))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"added":   res.Added,
		"updated": res.Updated,
		"failed":  res.Failed,
	})
}

// foxxycodeSkillsSourcesGet lists configured remote skill sources.
func (s *Server) foxxycodeSkillsSourcesGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "foxxycode.skills_sources",
		"items":  skills.ListSources(s.activeCfg()),
	})
}

// foxxycodeSkillsUpdatePost re-syncs the source that provides {name}, installing the
// version that source currently declares.
func (s *Server) foxxycodeSkillsUpdatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	res, err := skills.UpdateSkill(r.Context(), s.activeCfg(), name)
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	s.invalidateSlashCache()
	slog.Info("skill updated", "name", name, "added", len(res.Added), "updated", len(res.Updated), "failed", len(res.Failed))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"added":   res.Added,
		"updated": res.Updated,
		"failed":  res.Failed,
	})
}

// foxxycodeSkillsSourcesDelete removes a source from skills.sources (query ?source=).
func (s *Server) foxxycodeSkillsSourcesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.NotFound(w, r)
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source == "" {
		http.Error(w, `{"error":{"message":"missing source query parameter"}}`, http.StatusBadRequest)
		return
	}
	removed, err := skills.RemoveSource(s.activeCfg(), source)
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	s.reloadConfigFromDisk()
	slog.Info("skill source removed", "source", source, "removed", removed)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "removed": removed})
}

func (s *Server) invalidateSlashCache() {
	s.slashMu.Lock()
	s.slashCache = make(map[string]slashListCacheEntry)
	s.slashMu.Unlock()
}

// reloadConfigFromDisk re-reads config.yaml (after AddSource persisted it) and
// swaps it into the running server and session manager.
func (s *Server) reloadConfigFromDisk() {
	c := s.activeCfg()
	if c == nil {
		return
	}
	reloaded, err := config.LoadWithPaths(c.Paths)
	if err != nil {
		s.log.Error("skills config reload", "error", err)
		return
	}
	s.ReplaceConfig(reloaded)
	s.mgr.ReplaceConfig(reloaded)
}
