//go:build http

package httpserver

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/gitws"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/svn"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/workcopy"
)

type workingCopyChanges struct {
	dir, vcs string
	svnOpts  svnws.Options
}

func (s *Server) changesFor(ctx context.Context, cwd string) workingCopyChanges {
	wc := workingCopyChanges{dir: cwd}
	gitRoot := gitws.CheckoutRoot(cwd)
	if gitRoot != "" {
		wc.vcs = "git"
		if session.SameWorkspacePath(gitRoot, cwd) {
			return wc
		}
	}
	cfg := s.activeCfg()
	if cfg != nil && cfg.VCS.SVN.SVNEnabled() {
		wc.svnOpts = svn.OptionsFor(cfg)
		wc.svnOpts.BranchLookup = false
		info := svnws.Describe(ctx, cwd, wc.svnOpts)
		if info.IsSVNRepo && (gitRoot == "" || folderInside(gitRoot, info.WCRoot)) {
			wc.vcs = "svn"
		}
	}
	return wc
}

// Prefer the closest checkout when Git and SVN working copies are nested.
func folderInside(parent, child string) bool {
	rel, err := filepath.Rel(session.CanonicalWorkspacePath(parent), session.CanonicalWorkspacePath(child))
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

func (wc workingCopyChanges) list(ctx context.Context) ([]workcopy.Change, int, error) {
	switch wc.vcs {
	case "git":
		return gitws.WorktreeChanges(wc.dir)
	case "svn":
		return svnws.WorktreeChanges(ctx, wc.svnOpts, wc.dir)
	}
	return nil, 0, nil
}

func (wc workingCopyChanges) changeFor(ctx context.Context, path string) (*workcopy.Change, error) {
	switch wc.vcs {
	case "git":
		return gitws.WorktreeChangeFor(wc.dir, path)
	case "svn":
		return svnws.WorktreeChangeFor(ctx, wc.svnOpts, wc.dir, path)
	}
	return nil, nil
}

func (wc workingCopyChanges) read(ctx context.Context, path string) (*workcopy.Change, error) {
	// Try the reported name first: POSIX filenames can contain a backslash.
	change, err := wc.changeFor(ctx, path)
	other := filepath.FromSlash(strings.ReplaceAll(path, `\`, "/"))
	if err == nil && change == nil && other != path {
		return wc.changeFor(ctx, other)
	}
	return change, err
}

func (wc workingCopyChanges) discard(ctx context.Context, paths []string) error {
	if wc.vcs == "svn" {
		return svnws.Discard(ctx, wc.svnOpts, wc.dir, paths)
	}
	return gitws.Discard(wc.dir, paths)
}
