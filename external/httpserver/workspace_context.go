//go:build http

package httpserver

// Workspace context endpoints back the SPA composer chips: current folder,
// git branch, and worktree state, plus folder browsing and switching.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/gitws"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	toolsvn "github.com/hijera/foxxycode-agent/internal/tools/svn"
)

// workspaceContextPayload builds the JSON body shared by the context GET and
// the workspace switch POST. Git and Subversion are described independently, so
// a branch folder that also holds a git repository reports both.
func (s *Server) workspaceContextPayload(ctx context.Context, cwd string) map[string]interface{} {
	info := gitws.Describe(cwd)
	payload := map[string]interface{}{
		"object":      "foxxycode.workspace_context",
		"path":        info.Path,
		"name":        filepath.Base(info.Path),
		"is_git_repo": info.IsGitRepo,
		"is_worktree": info.IsWorktree,
	}
	if info.IsGitRepo {
		payload["repo_root"] = info.RepoRoot
		payload["branch"] = info.Branch
		payload["branches"] = info.Branches
		wts := make([]map[string]interface{}, 0, len(info.Worktrees))
		for _, wt := range info.Worktrees {
			wts = append(wts, map[string]interface{}{
				"path":   wt.Path,
				"branch": wt.Branch,
				"main":   wt.Main,
			})
		}
		payload["worktrees"] = wts
	}

	// With Subversion turned off in the settings the payload carries no svn
	// object at all, so the SPA hides both svn chips; with support on it always
	// reports at least `available`, which distinguishes "not installed" from
	// "not a working copy".
	svn := s.describeSVN(ctx, cwd)
	payload["is_svn_repo"] = svn.IsSVNRepo
	if s.svnEnabled() {
		payload["svn"] = map[string]interface{}{
			"available":       svn.Available,
			"wc_root":         svn.WCRoot,
			"url":             svn.URL,
			"relative_url":    svn.RelativeURL,
			"repository_root": svn.RepositoryRoot,
			"revision":        svn.Revision,
			"branch":          svn.Branch,
			"branches":        svn.Branches,
			"nested":          svn.Nested,
		}
	}
	return payload
}

// svnEnabled reports whether Subversion support is switched on in the config.
// Turning it off hides the SVN chip exactly like a missing client.
func (s *Server) svnEnabled() bool {
	cfg := s.activeCfg()
	return cfg != nil && cfg.VCS.SVN.SVNEnabled()
}

// describeSVN inspects cwd with the configured svn client. It returns an empty
// Info when Subversion support is disabled.
func (s *Server) describeSVN(ctx context.Context, cwd string) svnws.Info {
	if !s.svnEnabled() {
		return svnws.Info{Path: cwd}
	}
	return svnws.Describe(ctx, cwd, toolsvn.OptionsFor(s.activeCfg()))
}

// foxxycodeWorkspaceContextGet reports the workspace state for ?path= when given
// (pre-session preview), otherwise for the session in X-FoxxyCode-Session-ID
// (or the server default cwd without the header).
func (s *Server) foxxycodeWorkspaceContextGet(w http.ResponseWriter, r *http.Request) {
	cwd := strings.TrimSpace(r.URL.Query().Get("path"))
	if cwd != "" {
		abs, err := filepath.Abs(cwd)
		if err != nil {
			http.Error(w, `{"error":{"message":"invalid path"}}`, http.StatusBadRequest)
			return
		}
		fi, err := os.Stat(abs)
		if err != nil || !fi.IsDir() {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, "folder not found: "+abs), http.StatusBadRequest)
			return
		}
		cwd = abs
	} else {
		resolved, ok := s.resolveSlashListCWD(w, r)
		if !ok {
			return
		}
		cwd = resolved
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.workspaceContextPayload(r.Context(), cwd))
}

// foxxycodeWorkspaceFoldersGet lists subfolders of ?path= (default: session cwd)
// for the workspace folder picker. Hidden folders and node_modules are skipped.
func (s *Server) foxxycodeWorkspaceFoldersGet(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(r.URL.Query().Get("path"))
	if dir == "" {
		cwd, ok := s.resolveSlashListCWD(w, r)
		if !ok {
			return
		}
		dir = cwd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		http.Error(w, `{"error":{"message":"invalid path"}}`, http.StatusBadRequest)
		return
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.IsDir() {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, "folder not found: "+abs), http.StatusBadRequest)
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	folders := make([]map[string]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		folders = append(folders, map[string]string{
			"name": name,
			"path": filepath.Join(abs, name),
		})
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i]["name"] < folders[j]["name"] })
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":  "foxxycode.workspace_folders",
		"path":    abs,
		"parent":  filepath.Dir(abs),
		"folders": folders,
	})
}

// foxxycodeSessionWorkspacePost switches the session workspace: {"path": dir}
// changes the folder, {"branch": b} checks the branch out in place, and
// {"branch": b, "worktree": true} ensures a dedicated worktree for it.
// {"vcs": "svn"} routes the branch switch to Subversion, where "worktree" means
// checking the branch out into its own folder.
func (s *Server) foxxycodeSessionWorkspacePost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	var body struct {
		Path     string `json:"path"`
		Branch   string `json:"branch"`
		Worktree bool   `json:"worktree"`
		VCS      string `json:"vcs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	st, err := s.mgr.EnsureHTTPSession(r.Context(), id, s.defaultCWD)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	// Folder, branch, and worktree are fixed at session start: once the
	// conversation has messages, the workspace no longer moves under it.
	if len(st.GetMessages()) > 0 {
		http.Error(w, `{"error":{"message":"workspace is locked once the conversation starts"}}`, http.StatusConflict)
		return
	}

	switch {
	case strings.TrimSpace(body.Path) != "":
		if err := s.mgr.SetSessionWorkspace(st, body.Path); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
			return
		}
	case strings.TrimSpace(body.Branch) != "":
		switch strings.ToLower(strings.TrimSpace(body.VCS)) {
		case "", "git":
			if status, err := s.applyBranchSwitch(st, body.Branch, body.Worktree); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), status)
				return
			}
		case "svn":
			if status, err := s.applySVNBranchSwitch(r.Context(), st, body.Branch, body.Worktree); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), status)
				return
			}
		default:
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`,
				"unsupported vcs: "+body.VCS), http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, `{"error":{"message":"path or branch required"}}`, http.StatusBadRequest)
		return
	}

	payload := s.workspaceContextPayload(r.Context(), st.GetCWD())
	payload["id"] = id
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// applyBranchSwitch moves the session to branch. A branch already checked out
// in another worktree (including the main one) switches the session cwd there;
// otherwise it is either checked out in place or opened in a new worktree
// under <home>/worktrees/<repo>/. Returns the HTTP status for errors.
func (s *Server) applyBranchSwitch(st *session.State, branch string, useWorktree bool) (int, error) {
	cwd := st.GetCWD()
	info := gitws.Describe(cwd)
	if !info.IsGitRepo {
		return http.StatusBadRequest, fmt.Errorf("workspace is not a git repository: %s", cwd)
	}
	branch = strings.TrimSpace(branch)
	if branch == info.Branch {
		return 0, nil
	}
	for _, wt := range info.Worktrees {
		if wt.Branch == branch {
			if err := s.mgr.SetSessionWorkspace(st, wt.Path); err != nil {
				return http.StatusBadRequest, err
			}
			return 0, nil
		}
	}
	if useWorktree {
		root := filepath.Join(info.RepoRoot, ".foxxycode", "worktrees")
		if cfg := s.activeCfg(); cfg != nil && strings.TrimSpace(cfg.Paths.Home) != "" {
			root = filepath.Join(cfg.Paths.Home, "worktrees", filepath.Base(info.RepoRoot))
		}
		path, _, err := gitws.EnsureWorktree(info.RepoRoot, branch, root)
		if err != nil {
			return http.StatusConflict, err
		}
		if err := s.mgr.SetSessionWorkspace(st, path); err != nil {
			return http.StatusBadRequest, err
		}
		return 0, nil
	}
	if err := gitws.Checkout(cwd, branch); err != nil {
		return http.StatusConflict, err
	}
	return 0, nil
}

// svnWorkingCopyAt reports whether dir itself is an existing svn working copy
// root (not merely a folder below one).
func svnWorkingCopyAt(ctx context.Context, dir string, opts svnws.Options) bool {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return false
	}
	info := svnws.Describe(ctx, dir, opts)
	return info.IsSVNRepo && !info.Nested
}

// applySVNBranchSwitch moves the session to an SVN branch. Subversion has no
// worktrees, so separateFolder checks the branch out into its own folder under
// <home>/worktrees/<wc>/ and moves the session there - the branch-folder
// workflow. Otherwise the working copy is switched in place.
func (s *Server) applySVNBranchSwitch(ctx context.Context, st *session.State, branch string, separateFolder bool) (int, error) {
	cwd := st.GetCWD()
	if !s.svnEnabled() {
		return http.StatusConflict, fmt.Errorf("subversion support is disabled (vcs.svn.enabled)")
	}
	opts := toolsvn.OptionsFor(s.activeCfg())
	info := svnws.Describe(ctx, cwd, opts)
	if !info.Available {
		return http.StatusConflict, fmt.Errorf("svn client not found; install Subversion or set vcs.svn.binary")
	}
	if !info.IsSVNRepo {
		return http.StatusBadRequest, fmt.Errorf("workspace is not an svn working copy: %s", cwd)
	}
	branch = strings.TrimSpace(branch)
	if branch == info.Branch && !separateFolder {
		return 0, nil
	}
	url, err := svnws.BranchURL(info, branch)
	if err != nil {
		return http.StatusBadRequest, err
	}

	if separateFolder {
		dest := filepath.Join(info.WCRoot, ".foxxycode", "branches", svnws.BranchDirName(branch))
		if cfg := s.activeCfg(); cfg != nil && strings.TrimSpace(cfg.Paths.Home) != "" {
			dest = filepath.Join(cfg.Paths.Home, "worktrees",
				filepath.Base(info.WCRoot), svnws.BranchDirName(branch))
		}
		// An existing checkout of that branch is reused instead of re-fetched.
		// The folder has to be a working copy in its own right: Describe would
		// otherwise resolve an enclosing working copy above it.
		if !svnWorkingCopyAt(ctx, dest, opts) {
			if _, err := svnws.Checkout(ctx, opts, url, dest, ""); err != nil {
				return http.StatusConflict, err
			}
		}
		if err := s.mgr.SetSessionWorkspace(st, dest); err != nil {
			return http.StatusBadRequest, err
		}
		return 0, nil
	}
	if _, err := svnws.Switch(ctx, cwd, opts, url); err != nil {
		return http.StatusConflict, err
	}
	return 0, nil
}
