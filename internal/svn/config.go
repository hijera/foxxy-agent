// Package svn owns Subversion integration policies and workspace operations.
package svn

import (
	"context"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svnws"
)

// OptionsFor maps the vcs.svn config section onto svnws options. Exported so the
// HTTP layer describes working copies with exactly the same client settings.
func OptionsFor(cfg *config.Config) svnws.Options {
	if cfg == nil {
		return svnws.Options{TimeoutSeconds: config.SVNDefaultTimeoutSeconds}
	}
	return svnws.Options{
		Binary:         cfg.VCS.SVN.Binary,
		TimeoutSeconds: cfg.VCS.SVN.ResolvedTimeoutSeconds(),
		BranchLookup:   cfg.VCS.SVN.BranchLookupEnabled(),
	}
}

// DescribeFor inspects dir with the client the vcs.svn section configures. With
// Subversion support switched off (or no config at all) it reports a plain
// folder without running svn, so every surface hides SVN the same way.
func DescribeFor(ctx context.Context, cfg *config.Config, dir string) svnws.Info {
	if cfg == nil || !cfg.VCS.SVN.SVNEnabled() {
		return svnws.Info{Path: dir}
	}
	return svnws.Describe(ctx, dir, OptionsFor(cfg))
}

// BranchFor returns the branch of the working copy dir sits in ("trunk",
// "branches/feature-x"), or "" for a plain folder. It is the Subversion
// counterpart of gitws.Describe(dir).Branch for callers that only label
// something with the branch, such as a session export: the answer comes from
// the local working copy, so the repository's branch list - a server round
// trip - is never requested, whatever vcs.svn.branch_lookup says.
func BranchFor(ctx context.Context, cfg *config.Config, dir string) string {
	if cfg == nil || !cfg.VCS.SVN.SVNEnabled() || strings.TrimSpace(dir) == "" {
		return ""
	}
	opts := OptionsFor(cfg)
	opts.BranchLookup = false
	return svnws.Describe(ctx, dir, opts).Branch
}
