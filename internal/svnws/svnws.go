// Package svnws inspects and manipulates Subversion working copies. It is the
// SVN counterpart of package gitws: detection for the workspace chips plus the
// operations backing the svn_* tools. Like gitws it shells out to the client
// binary and imports no other internal package.
package svnws

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/platform"
)

// DefaultTimeoutSeconds bounds a single svn invocation when Options leaves it unset.
const DefaultTimeoutSeconds = 120

// Options configures how the svn client is invoked. Callers build it from the
// vcs.svn config section; svnws itself never reads configuration.
type Options struct {
	// Binary is an explicit path to the svn client. Empty resolves "svn" on PATH.
	Binary string
	// TimeoutSeconds bounds each invocation. Zero uses DefaultTimeoutSeconds.
	TimeoutSeconds int
	// BranchLookup allows Describe to list repository branches, which contacts
	// the server. Off keeps Describe purely local.
	BranchLookup bool
}

// Info describes the Subversion state of a workspace folder.
type Info struct {
	Path           string   `json:"path"`
	Available      bool     `json:"available"`
	IsSVNRepo      bool     `json:"is_svn_repo"`
	WCRoot         string   `json:"wc_root,omitempty"`
	URL            string   `json:"url,omitempty"`
	RelativeURL    string   `json:"relative_url,omitempty"`
	RepositoryRoot string   `json:"repository_root,omitempty"`
	UUID           string   `json:"uuid,omitempty"`
	Revision       int      `json:"revision,omitempty"`
	Branch         string   `json:"branch,omitempty"`
	Branches       []string `json:"branches,omitempty"`
	// Nested reports that the working copy root sits above Path: the folder
	// itself is not versioned (a git clone dropped inside a branch folder is
	// the common case), but an enclosing directory is an SVN working copy.
	Nested bool `json:"nested,omitempty"`
}

func (o Options) binary() string {
	if b := strings.TrimSpace(o.Binary); b != "" {
		return b
	}
	return "svn"
}

func (o Options) timeout() time.Duration {
	if o.TimeoutSeconds > 0 {
		return time.Duration(o.TimeoutSeconds) * time.Second
	}
	return DefaultTimeoutSeconds * time.Second
}

// Available reports whether the configured svn client can be resolved.
func Available(o Options) bool {
	_, err := exec.LookPath(o.binary())
	return err == nil
}

// run executes the svn client in dir and returns its stdout. Every invocation is
// non-interactive so a missing credential fails fast instead of blocking a turn.
func run(ctx context.Context, o Options, dir string, args ...string) (string, error) {
	if !Available(o) {
		return "", fmt.Errorf("svn client not found (%s); install Subversion or set vcs.svn.binary", o.binary())
	}
	ctx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()

	full := append([]string{"--non-interactive"}, args...)
	cmd := exec.CommandContext(ctx, o.binary(), full...)
	cmd.Dir = dir
	platform.HideConsoleWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimRight(stdout.String(), "\r\n")
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if ctx.Err() != nil {
			return out, fmt.Errorf("svn %s timed out after %s: %s",
				strings.Join(args, " "), o.timeout(), detail)
		}
		return out, fmt.Errorf("svn %s: %w: %s", strings.Join(args, " "), err, detail)
	}
	return out, nil
}

// infoXML mirrors the parts of `svn info --xml` this package consumes.
type infoXML struct {
	Entries []struct {
		Revision    string `xml:"revision,attr"`
		URL         string `xml:"url"`
		RelativeURL string `xml:"relative-url"`
		Repository  struct {
			Root string `xml:"root"`
			UUID string `xml:"uuid"`
		} `xml:"repository"`
		WCInfo struct {
			WCRoot string `xml:"wcroot-abspath"`
		} `xml:"wc-info"`
		Commit struct {
			Revision string `xml:"revision,attr"`
		} `xml:"commit"`
	} `xml:"entry"`
}

// listXML mirrors `svn list --xml`.
type listXML struct {
	Lists []struct {
		Entries []struct {
			Kind string `xml:"kind,attr"`
			Name string `xml:"name"`
		} `xml:"entry"`
	} `xml:"list"`
}

// Describe inspects dir. It never fails on plain folders: a non-working-copy dir
// (or a missing svn client) yields Info{IsSVNRepo: false}. Detection is entirely
// independent of git, so a folder can be both a git repository and an SVN
// working copy.
func Describe(ctx context.Context, dir string, o Options) Info {
	info := Info{Path: dir}
	if abs, err := filepath.Abs(dir); err == nil {
		info.Path = abs
	}
	if !Available(o) {
		return info
	}
	info.Available = true

	target, out, nested := resolveWCDir(ctx, info.Path, o)
	if target == "" || strings.TrimSpace(out) == "" {
		return info
	}
	var parsed infoXML
	if err := xml.Unmarshal([]byte(out), &parsed); err != nil || len(parsed.Entries) == 0 {
		return info
	}
	entry := parsed.Entries[0]

	info.IsSVNRepo = true
	info.Nested = nested
	info.URL = strings.TrimSpace(entry.URL)
	info.RepositoryRoot = strings.TrimSpace(entry.Repository.Root)
	info.UUID = strings.TrimSpace(entry.Repository.UUID)
	info.WCRoot = strings.TrimSpace(entry.WCInfo.WCRoot)
	if info.WCRoot == "" {
		info.WCRoot = target
	}
	info.RelativeURL = relativeURL(entry.RelativeURL, info.URL, info.RepositoryRoot)
	info.Revision = parseRevision(entry.Revision, entry.Commit.Revision)
	info.Branch = BranchFromRelativeURL(info.RelativeURL)
	if o.BranchLookup {
		info.Branches = listBranches(ctx, target, o, info.RepositoryRoot)
	}
	return info
}

// resolveWCDir returns the directory whose `svn info --xml` describes dir, along
// with that output. When dir itself is unversioned it walks up to the nearest
// ancestor holding a .svn administrative directory; the bool reports that the
// working copy root sits above dir.
func resolveWCDir(ctx context.Context, dir string, o Options) (target, out string, nested bool) {
	if out, err := run(ctx, o, dir, "info", "--xml"); err == nil {
		return dir, out, false
	}
	current := dir
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", false
		}
		current = parent
		if fi, err := os.Stat(filepath.Join(current, ".svn")); err == nil && fi.IsDir() {
			if out, err := run(ctx, o, current, "info", "--xml"); err == nil {
				return current, out, true
			}
			return "", "", false
		}
	}
}

// relativeURL prefers the client-provided value (svn 1.8+) and otherwise derives
// it by trimming the repository root off the entry URL.
func relativeURL(raw, url, root string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return v
	}
	url = strings.TrimSpace(url)
	root = strings.TrimSpace(root)
	if url == "" || root == "" || !strings.HasPrefix(url, root) {
		return ""
	}
	return "^" + strings.TrimSuffix(strings.TrimPrefix(url, root), "/")
}

// parseRevision picks the working copy revision, falling back to the last
// committed revision when the entry carries no revision attribute.
func parseRevision(entry, commit string) int {
	for _, raw := range []string{entry, commit} {
		n := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// BranchFromRelativeURL maps a repository-relative URL to the branch it belongs
// to: "^/branches/feature-x/sub" becomes "branches/feature-x", "^/trunk/sub"
// becomes "trunk". An unrecognised layout yields an empty string.
func BranchFromRelativeURL(rel string) string {
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "^")
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return ""
	}
	parts := strings.Split(rel, "/")
	switch parts[0] {
	case "trunk":
		return "trunk"
	case "branches", "tags":
		if len(parts) >= 2 && parts[1] != "" {
			return parts[0] + "/" + parts[1]
		}
		return parts[0]
	}
	return ""
}

// listBranches reports the branch-like paths of the repository: trunk when
// present plus every directory under branches/. Failures degrade to nil, since
// the lookup contacts the server.
func listBranches(ctx context.Context, dir string, o Options, root string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	var out []string
	for _, name := range listDirNames(ctx, dir, o, root) {
		if name == "trunk" {
			out = append(out, "trunk")
		}
	}
	for _, name := range listDirNames(ctx, dir, o, root+"/branches") {
		out = append(out, "branches/"+name)
	}
	return out
}

// listDirNames returns the directory entry names of an svn URL.
func listDirNames(ctx context.Context, dir string, o Options, url string) []string {
	if err := guardArg("url", url); err != nil {
		return nil
	}
	out, err := run(ctx, o, dir, "list", "--xml", "--", url)
	if err != nil {
		return nil
	}
	var parsed listXML
	if err := xml.Unmarshal([]byte(out), &parsed); err != nil {
		return nil
	}
	var names []string
	for _, list := range parsed.Lists {
		for _, e := range list.Entries {
			if e.Kind != "dir" {
				continue
			}
			if name := strings.Trim(strings.TrimSpace(e.Name), "/"); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// BranchURL resolves a branch name ("trunk", "branches/x") against the
// repository root of info.
func BranchURL(info Info, branch string) (string, error) {
	branch = strings.Trim(strings.TrimSpace(branch), "/")
	if branch == "" {
		return "", fmt.Errorf("empty branch name")
	}
	if err := guardArg("branch", branch); err != nil {
		return "", err
	}
	root := strings.TrimSuffix(strings.TrimSpace(info.RepositoryRoot), "/")
	if root == "" {
		return "", fmt.Errorf("repository root unknown for %s", info.Path)
	}
	return root + "/" + branch, nil
}

// BranchDirName maps a branch name to a filesystem-safe directory name, matching
// the git worktree folder naming.
func BranchDirName(branch string) string {
	mapped := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', ' ', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, strings.TrimSpace(branch))
	return strings.Trim(mapped, "-.")
}
