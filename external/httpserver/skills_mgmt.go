//go:build http

package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
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
	s.mux.HandleFunc("POST /foxxycode/skills/sources/trust", s.foxxycodeSkillsSourceTrust)
	s.mux.HandleFunc("POST /foxxycode/skills/sources/untrust", s.foxxycodeSkillsSourceUntrust)
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

// foxxycodeSkillsGet lists all skills with their enabled/disabled state. ${CWD} in
// skills.dirs resolves against the session named by the optional
// X-FoxxyCode-Session-ID header, else the folder in the cwd query (Settings → Skills
// of a new chat), else the server default workspace, the same way
// /foxxycode/slash-commands does.
func (s *Server) foxxycodeSkillsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	cfg := s.activeCfg()
	installDir := cfg.Skills.ManagedDir(cfg.Paths.Home)
	loader := skills.NewLoader(cfg.Skills.SearchDirs())

	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	allLoaded, err := loader.LoadAll(cwd, cfg.Paths.Home)
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

// foxxycodeSkillsSyncPost fetches the skill sources in effect for the chat
// workspace (the session, else the cwd query of a picked folder, else the
// server default) and materializes them; a project entry the trust gate holds
// back is reported in held, not fetched.
func (s *Server) foxxycodeSkillsSyncPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	// Optional ?source=<src>: only that source, or only what is installed from
	// it when it is a marketplace; otherwise sync all.
	var res *skills.SyncResult
	var err error
	if src := strings.TrimSpace(r.URL.Query().Get("source")); src != "" {
		res, _, err = skills.UpdateSource(r.Context(), s.activeCfg(), cwd, src)
	} else {
		res, err = skills.Sync(r.Context(), s.activeCfg(), cwd)
	}
	if err != nil {
		// An entry the trust gate holds back is the caller's to approve, not
		// a failure of the server.
		status := http.StatusInternalServerError
		if errors.Is(err, skills.ErrHeld) {
			status = http.StatusBadRequest
		}
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), status)
		return
	}
	s.invalidateSlashCache()
	slog.Info("skills synced", "added", len(res.Added), "updated", len(res.Updated), "failed", len(res.Failed))
	w.Header().Set("Content-Type", "application/json")
	held := res.Held
	if held == nil {
		held = []skills.Declaration{}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"added":   res.Added,
		"updated": res.Updated,
		"failed":  res.Failed,
		"held":    held,
	})
}

type skillSourceRequest struct {
	Source string `json:"source"`
	Sync   bool   `json:"sync"`
	// Scope picks the file: "global" (default) <home>/marketplaces.json,
	// "local" the session workspace's .foxxycode/marketplaces.json.
	Scope string `json:"scope,omitempty"`
	// Kind is "source" (default, installed whole) or "marketplace" (a
	// catalog whose plugins are installed one by one).
	Kind string `json:"kind,omitempty"`
}

// foxxycodeSkillsSourcesPost declares a remote source (or a marketplace) in the
// marketplaces.json the scope names, and optionally syncs it. A local entry
// is approved for the workspace by the act of writing it.
func (s *Server) foxxycodeSkillsSourcesPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	cwd, ok := s.resolveSessionCWD(w, r)
	if !ok {
		return
	}
	var req skillSourceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
		return
	}
	cfg := s.activeCfg()
	var added bool
	var err error
	switch strings.TrimSpace(req.Kind) {
	case "", skills.KindSource:
		added, err = skills.AddSource(cfg, cwd, req.Source, req.Scope)
	case skills.KindMarketplace:
		if req.Sync {
			// Declaring a marketplace installs nothing; syncing its source
			// would install every plugin it lists and record them so.
			http.Error(w, `{"error":{"message":"a marketplace installs nothing on its own: install its plugins one by one (POST /foxxycode/skills/install, or plugin install <plugin>@<marketplace>), or add it with kind source to have every plugin installed"}}`, http.StatusBadRequest)
			return
		}
		var refreshed bool
		_, refreshed, err = skills.AddMarketplace(r.Context(), cfg, cwd, req.Source, req.Scope)
		added = !refreshed
	default:
		err = fmt.Errorf("unknown kind %q (use %q or %q)", req.Kind, skills.KindSource, skills.KindMarketplace)
	}
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}

	resp := map[string]interface{}{"ok": true, "added": added}
	if req.Sync {
		// The source just added, as documented; the others are refreshed by
		// POST /foxxycode/skills/sync.
		res, err := skills.SyncSource(r.Context(), s.activeCfg(), req.Source)
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
	// The skill is looked up in the workspace of the list it was picked from:
	// a project skill of the chat's folder is not in the server's default cwd.
	// The reach is the listing's (a session, else cwd, else the default) and
	// DeleteSkill still removes only a path inside a configured skills
	// directory of that workspace; for a skill linked in from elsewhere that
	// path is the link, so what it points at stays on disk.
	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	if err := skills.DeleteSkill(s.activeCfg(), cwd, name); err != nil {
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
// version is available in its marketplace source (performs network/git
// access). A source a project of the workspace declares and the trust gate
// holds back is not contacted.
func (s *Server) foxxycodeSkillsUpdatesGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	statuses, err := skills.CheckUpdates(r.Context(), s.activeCfg(), cwd)
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
	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	items, err := skills.AvailablePlugins(r.Context(), s.activeCfg(), cwd)
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

// foxxycodeSkillsInstallPost installs a single plugin from a marketplace source
// in effect for the chat workspace, or one the operator explicitly names.
func (s *Server) foxxycodeSkillsInstallPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	var req skillInstallRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
		return
	}
	res, err := skills.InstallPlugin(r.Context(), s.activeCfg(), cwd, req.Source, req.Plugin)
	if err == nil && len(res.Added)+len(res.Updated) == 0 && len(res.Failed) > 0 {
		// Nothing installed: an archive refused, a plugin without skills. The
		// client reports the reason instead of a success.
		err = errors.New("install " + req.Plugin + ": " + res.Failed[0].Error)
	}
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

// foxxycodeSkillsSourcesGet lists the skill sources and marketplaces declared
// for the chat workspace (the session, else the cwd query of a folder picked
// before its session, else the server default): entries carries each with its
// kind, origin, file and trust state; items are the sources installed whole in
// effect and system the subset FoxxyCode brings itself, which DELETE refuses.
func (s *Server) foxxycodeSkillsSourcesGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	cfg := s.activeCfg()
	decls, errs := skills.Declarations(cfg, cwd)
	if decls == nil {
		decls = []skills.Declaration{}
	}
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	system := make([]string, 0, len(skills.SystemSources))
	system = append(system, skills.SystemSources...)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":        "foxxycode.skills_sources",
		"workspace":     cwd,
		"project_trust": cfg.Skills.ResolvedProjectTrust(),
		"items":         skills.ListSources(cfg, cwd),
		"system":        system,
		"entries":       decls,
		"errors":        messages,
	})
}

type skillSourceTrustRequest struct {
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// foxxycodeSkillsSourceTrust approves a project entry of the session workspace's
// .foxxycode/marketplaces.json. The optional fingerprint names the entry the
// operator was shown: one the checkout rewrote since is refused with 409.
func (s *Server) foxxycodeSkillsSourceTrust(w http.ResponseWriter, r *http.Request) {
	cwd, ok := s.resolveSessionCWD(w, r)
	if !ok {
		return
	}
	var req skillSourceTrustRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Key) == "" {
		http.Error(w, `{"error":{"message":"body must name the entry: {\"key\": \"<marketplace | source>\"}"}}`, http.StatusBadRequest)
		return
	}
	d, err := skills.ApproveShown(s.activeCfg(), cwd, req.Key, req.Fingerprint)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, skills.ErrDeclarationChanged) {
			status = http.StatusConflict
		}
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), status)
		return
	}
	slog.Info("project skill source approved", "key", req.Key, "workspace", cwd)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "fingerprint": d.Fingerprint})
}

// foxxycodeSkillsSourceUntrust withdraws the approval of a project entry.
func (s *Server) foxxycodeSkillsSourceUntrust(w http.ResponseWriter, r *http.Request) {
	cwd, ok := s.resolveSessionCWD(w, r)
	if !ok {
		return
	}
	var req skillSourceTrustRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Key) == "" {
		http.Error(w, `{"error":{"message":"body must name the entry: {\"key\": \"<marketplace | source>\"}"}}`, http.StatusBadRequest)
		return
	}
	removed, err := skills.Revoke(s.activeCfg(), cwd, req.Key)
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "removed": removed})
}

// foxxycodeSkillsUpdatePost re-syncs the source that provides {name}, installing the
// version that source currently declares.
func (s *Server) foxxycodeSkillsUpdatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	cwd, ok := s.resolveListingCWD(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	res, err := skills.UpdateSkill(r.Context(), s.activeCfg(), cwd, name)
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

// foxxycodeSkillsSourcesDelete takes the sources and marketplaces ?source= names
// (a marketplace by name, either kind by source) out of the operator's
// marketplaces.json and the session workspace's, or only out of the one
// ?origin= names.
func (s *Server) foxxycodeSkillsSourcesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.NotFound(w, r)
		return
	}
	cwd, ok := s.resolveSessionCWD(w, r)
	if !ok {
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source == "" {
		http.Error(w, `{"error":{"message":"missing source query parameter"}}`, http.StatusBadRequest)
		return
	}
	// origin (home or project) limits the removal to the file of the row a
	// client showed; without it both files are edited.
	removed, err := skills.RemoveSource(s.activeCfg(), cwd, source, r.URL.Query().Get("origin"))
	var notice skills.RemovalNotice
	if errors.As(err, &notice) {
		// The entry was dropped; the notice only says the built-in source stays.
		slog.Info("skill source removed", "source", source, "notice", notice.Msg)
		err = nil
	}
	if err != nil {
		body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": err.Error()}})
		http.Error(w, string(body), http.StatusBadRequest)
		return
	}
	slog.Info("skill source removed", "source", source, "removed", removed)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "removed": removed})
}

func (s *Server) invalidateSlashCache() {
	s.slashMu.Lock()
	s.slashCache = make(map[string]slashListCacheEntry)
	s.slashMu.Unlock()
}

// reloadConfigFromDisk re-reads config.yaml (after a route persisted it) and
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
	// The same overrides every other install applies; a document they refuse
	// leaves the live configuration in place.
	if err := s.mgr.ReplaceConfigChecked(reloaded); err != nil {
		s.log.Error("skills config reload", "error", err)
		return
	}
	s.ReplaceConfig(reloaded)
}
