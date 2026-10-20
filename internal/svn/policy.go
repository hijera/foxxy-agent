package svn

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Policy describes display and mutation effects, never permission approval.
type Policy struct {
	Kind    string
	Mutates bool
}

// ToolPolicy resolves historical tool semantics even when SVN is disabled.
func ToolPolicy(name string) (Policy, bool) {
	for _, candidate := range ReadOnlyToolNames() {
		if name == candidate {
			return Policy{Kind: "other"}, true
		}
	}
	for _, candidate := range mutatingToolNames() {
		if name == candidate {
			return Policy{Kind: "write", Mutates: true}, true
		}
	}
	return Policy{}, false
}

// WriteTargets reports paths whose previous contents became stale.
func WriteTargets(name, argsJSON, cwd string) ([]string, bool) {
	policy, ok := ToolPolicy(name)
	if !ok {
		return nil, false
	}
	if !policy.Mutates {
		return nil, true
	}
	return writeTargets(name, argsJSON, cwd), true
}

func absPath(path, cwd string) string {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

// svnWriteTargets returns what a successful Subversion mutation invalidates.
//
// Only tools that rewrite working-copy content make an earlier read stale.
// svn_add merely schedules a path and svn_commit only ships what is already on
// disk, so treating those as writes would collapse the entire read history at
// the very end of a task, when the model most needs it. The rest do rewrite
// files, so they invalidate their target paths - or the whole workspace when the
// call names none, which for update/revert/resolve means "everything", and for
// switch/merge/checkout is the only safe answer since they can replace an
// arbitrary subtree.
func writeTargets(toolName, argsJSON, cwd string) []string {
	switch toolName {
	case "svn_add", "svn_commit":
		return nil
	case "svn_update", "svn_revert", "svn_resolve":
		var a struct {
			Paths []string `json:"paths"`
		}
		if json.Unmarshal([]byte(argsJSON), &a) != nil {
			return []string{absPath(cwd, cwd)}
		}
		var out []string
		for _, p := range a.Paths {
			if strings.TrimSpace(p) != "" {
				out = append(out, absPath(p, cwd))
			}
		}
		if len(out) == 0 {
			return []string{absPath(cwd, cwd)}
		}
		return out
	default:
		return []string{absPath(cwd, cwd)}
	}
}
