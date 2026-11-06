package svn

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svnws"
)

// SwitchResult leaves session admission, relocation and catalog reloads to the caller.
type SwitchResult struct {
	Path   string
	Reload bool
}

// WorkspaceError distinguishes invalid selections from operation conflicts.
type WorkspaceError struct {
	Err      error
	Conflict bool
}

func (e *WorkspaceError) Error() string { return e.Err.Error() }
func (e *WorkspaceError) Unwrap() error { return e.Err }
func conflict(err error) error          { return &WorkspaceError{Err: err, Conflict: true} }
func invalid(err error) error           { return &WorkspaceError{Err: err} }

// svnWorkingCopyAt reports whether dir itself is an existing svn working copy
// root (not merely a folder below one).
func svnWorkingCopyAt(ctx context.Context, dir string, opts svnws.Options) bool {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return false
	}
	info := svnws.Describe(ctx, dir, opts)
	return info.IsSVNRepo && !info.Nested
}

// SwitchBranch reuses or creates a branch folder, or switches the working copy in place.
func SwitchBranch(ctx context.Context, cfg *config.Config, cwd, branch string, separateFolder bool) (SwitchResult, error) {
	if cfg == nil || !cfg.VCS.SVN.SVNEnabled() {
		return SwitchResult{}, conflict(fmt.Errorf("subversion support is disabled (vcs.svn.enable)"))
	}
	opts := OptionsFor(cfg)
	info := svnws.Describe(ctx, cwd, opts)
	if !info.Available {
		return SwitchResult{}, conflict(fmt.Errorf("svn client not found; install Subversion or set vcs.svn.binary"))
	}
	if !info.IsSVNRepo {
		return SwitchResult{}, invalid(fmt.Errorf("workspace is not an svn working copy: %s", cwd))
	}
	branch = strings.TrimSpace(branch)
	if branch == info.Branch && !separateFolder {
		return SwitchResult{}, nil
	}
	url, err := svnws.BranchURL(info, branch)
	if err != nil {
		return SwitchResult{}, invalid(err)
	}

	if separateFolder {
		dest := filepath.Join(info.WCRoot, ".foxxycode", "branches", svnws.BranchDirName(branch))
		if cfg != nil && strings.TrimSpace(cfg.Paths.Home) != "" {
			dest = filepath.Join(cfg.Paths.Home, "worktrees",
				filepath.Base(info.WCRoot), svnws.BranchDirName(branch))
		}
		// An existing checkout of that branch is reused instead of re-fetched.
		// The folder has to be a working copy in its own right: Describe would
		// otherwise resolve an enclosing working copy above it.
		if !svnWorkingCopyAt(ctx, dest, opts) {
			if _, err := svnws.Checkout(ctx, opts, url, dest, ""); err != nil {
				return SwitchResult{}, conflict(err)
			}
		}
		return SwitchResult{Path: dest}, nil
	}
	if _, err := svnws.Switch(ctx, cwd, opts, url); err != nil {
		return SwitchResult{}, conflict(err)
	}
	return SwitchResult{Reload: true}, nil
}
