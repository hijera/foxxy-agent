package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EffectiveSessionCWD resolves the filesystem working directory for a new session.
// If clientCWD is empty or whitespace, defaultCWD is used. Result is absolute.
func EffectiveSessionCWD(clientCWD, defaultCWD string) (string, error) {
	s := strings.TrimSpace(clientCWD)
	if s == "" {
		s = strings.TrimSpace(defaultCWD)
	}
	if s == "" {
		return "", fmt.Errorf("session cwd is empty")
	}
	abs, err := filepath.Abs(s)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	return abs, nil
}

// CWDInScope reports whether cwd is root itself or a directory beneath it.
// An empty root matches everything (no scope requested); an empty cwd never matches.
// Both sides are compared in their canonical form (CanonicalWorkspacePath:
// absolute, cleaned, symlinks resolved), case-insensitively where the default
// filesystem folds case, so a session stored under one spelling of a project is
// in scope for any other spelling of it.
func CWDInScope(cwd, root string) bool {
	return NewWorkspaceScope(root)(cwd)
}

// NewWorkspaceScope returns the CWDInScope test for one root, resolving the
// root once and each distinct cwd once. A listing asks it for every stored
// session, and resolving symlinks costs a filesystem call per path component,
// so the IDE History poll does not pay that per row.
func NewWorkspaceScope(root string) func(cwd string) bool {
	r := CanonicalWorkspacePath(filepath.FromSlash(root))
	if r == "" {
		return func(string) bool { return true }
	}
	if caseInsensitivePaths {
		r = strings.ToLower(r)
	}
	prefix := r
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	seen := map[string]bool{}
	return func(cwd string) bool {
		key := strings.TrimSpace(cwd)
		if key == "" {
			return false
		}
		if in, ok := seen[key]; ok {
			return in
		}
		c := CanonicalWorkspacePath(filepath.FromSlash(key))
		if caseInsensitivePaths {
			c = strings.ToLower(c)
		}
		in := c == r || strings.HasPrefix(c, prefix)
		seen[key] = in
		return in
	}
}

// SetSessionWorkspace switches the session working directory and re-derives
// cwd-scoped state (skills, project rules, slash commands). The target must
// be an existing directory.
func (m *Manager) SetSessionWorkspace(st *State, dir string) error {
	abs, err := EffectiveSessionCWD(dir, "")
	if err != nil {
		return err
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("workspace folder not found: %s", abs)
	}

	if m.SessionTurnActiveInProcess(st.GetID()) {
		return ErrSessionTurnBusy
	}
	unlock, err := m.acquirePromptTurnLock(st.GetID(), st)
	if err != nil {
		return err
	}
	defer unlock()

	// The cwd switch is committed here, synchronously: the caller reports the new workspace as
	// soon as this returns. Only reconnecting the new workspace's MCP servers moves to the
	// background - it re-arms the readiness gate, so a prompt sent right after the switch waits
	// for the servers it is actually going to use. Same reasoning as
	// connectConfiguredMCPServers: this runs under the turn lock, on a request goroutine.
	settle := st.beginConfiguredMCPConnect()
	st.SetCWD(abs)
	go func() {
		defer settle()
		st.replaceConfiguredMCPClients(m.configuredMCPClients(context.Background(), abs))
	}()

	cfg := m.activeCfg()
	loadedSkills, err := m.loadSkills(abs, cfg)
	if err != nil {
		m.log.Warn("failed to load skills on workspace switch", "error", err)
	}
	st.ReplaceSkills(loadedSkills)
	st.ReplaceRulesCatalog(DiscoverRules(cfg, abs))
	m.sendAvailableSlashCommands(st.GetID(), st)
	return nil
}

// caseInsensitivePaths marks the platforms whose default filesystems fold
// case, so two spellings of a folder that differ only in case are one folder.
var caseInsensitivePaths = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// CanonicalWorkspacePath returns the form two spellings of one folder share:
// absolute, cleaned, with symlinks resolved when the folder exists. Sessions
// keep the cwd as the client gave it (the console stores the logical $PWD of
// a symlinked checkout, an editor sends the physical path, a Windows client
// may differ in the drive letter's case), so every workspace filter compares
// this form rather than the stored string.
func CanonicalWorkspacePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(abs)
}

// SameWorkspacePath reports whether two paths name the same folder.
func SameWorkspacePath(a, b string) bool {
	return matchesWorkspace(CanonicalWorkspacePath(a), b)
}

// matchesWorkspace compares an already canonical filter with a stored path.
func matchesWorkspace(canonical, stored string) bool {
	c := CanonicalWorkspacePath(stored)
	if c == canonical {
		return true
	}
	return caseInsensitivePaths && strings.EqualFold(c, canonical)
}
