package gitws

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/go-git/go-billy/v5"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// This file is the built-in backend of remotes.go: the remote branches, their
// refresh and a checkout tracking one of them, through go-git.

// builtinListRemotes names the configured remotes, sorted.
func builtinListRemotes(dir string) []string {
	repo, err := openRepo(dir)
	if err != nil {
		return nil
	}
	cfg, err := repo.Config()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Remotes))
	for name := range cfg.Remotes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// builtinListRefs returns the full names of the local and remote-tracking
// branches.
func builtinListRefs(dir string) []string {
	repo, err := openRepo(dir)
	if err != nil {
		return nil
	}
	iter, err := repo.References()
	if err != nil {
		return nil
	}
	var refs []string
	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		if name := ref.Name(); name.IsBranch() || name.IsRemote() {
			refs = append(refs, name.String())
		}
		return nil
	})
	sort.Strings(refs)
	return refs
}

// builtinBranchUpstream maps branch.<name>.merge through the remote's fetch refspec.
func builtinBranchUpstream(dir, branch string) string {
	repo, err := openRepo(dir)
	if err != nil {
		return ""
	}
	cfg, err := repo.Config()
	if err != nil {
		return ""
	}
	b, ok := cfg.Branches[branch]
	if !ok || b.Remote == "" || b.Remote == "." || !b.Merge.IsBranch() {
		return ""
	}
	remote := cfg.Remotes[b.Remote]
	if remote == nil {
		return ""
	}
	for _, spec := range remote.Fetch {
		if spec.Validate() == nil && spec.Match(b.Merge) {
			return spec.Dst(b.Merge).String()
		}
	}
	return ""
}

func trackingSource(remote *gitconfig.RemoteConfig, ref plumbing.ReferenceName) (plumbing.ReferenceName, error) {
	if remote != nil {
		for _, spec := range remote.Fetch {
			// Reverse a validated mapping with the force prefix removed. The
			// prefix applies to updates, and is not part of the source ref.
			spec = gitconfig.RefSpec(strings.TrimPrefix(spec.String(), "+"))
			if spec.Validate() != nil {
				continue
			}
			reverse := spec.Reverse()
			if reverse.Match(ref) {
				if source := reverse.Dst(ref); source.IsBranch() {
					return source, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no fetch refspec maps a remote branch to %s", ref)
}

// refuseUntrackedCollisions checks the paths checkout would introduce before
// go-git changes HEAD. Lstat also catches ignored files and symlinks.
func refuseUntrackedCollisions(worktree billy.Filesystem, current, next *object.Tree) error {
	changes, err := object.DiffTree(current, next)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if change.From.Name != "" || change.To.Name == "" {
			continue
		}
		name := change.To.Name
		var paths []string
		for candidate := name; candidate != "."; candidate = path.Dir(candidate) {
			paths = append(paths, candidate)
		}
	pathsCheck:
		for i := len(paths) - 1; i >= 0; i-- {
			candidate := paths[i]
			if entry, err := current.FindEntry(candidate); err == nil {
				if entry.Mode != filemode.Dir {
					// A clean tracked file can be replaced by a directory.
					break
				}
				if candidate == name {
					if err := checkTrackedDirectory(worktree, current, candidate); err != nil {
						return err
					}
				}
				continue
			}
			info, err := worktree.Lstat(candidate)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				// None of the descendants can exist either.
				break pathsCheck
			case err != nil:
				return fmt.Errorf("check checkout path %q: %w", candidate, err)
			case candidate == name || !info.IsDir():
				return untrackedCheckoutPath(candidate)
			}
		}
	}
	return nil
}

// refuseUncommitted refuses a checkout over tracked files that carry
// uncommitted changes, so nothing a checkout would discard is ever at risk;
// untracked files do not count.
func refuseUncommitted(wt *git.Worktree) error {
	status, err := wt.Status()
	if err != nil {
		return err
	}
	for _, fs := range status {
		if (fs.Staging != git.Unmodified && fs.Staging != git.Untracked) ||
			(fs.Worktree != git.Unmodified && fs.Worktree != git.Untracked) {
			return fmt.Errorf("refusing checkout: the working copy has uncommitted changes")
		}
	}
	return nil
}

func checkTrackedDirectory(worktree billy.Filesystem, current *object.Tree, dir string) error {
	entries, err := worktree.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range entries {
		name := path.Join(dir, info.Name())
		if _, err := current.FindEntry(name); err != nil {
			return untrackedCheckoutPath(name)
		}
		if info.IsDir() {
			if err := checkTrackedDirectory(worktree, current, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func untrackedCheckoutPath(name string) error {
	return fmt.Errorf("refusing checkout: untracked path %q would be overwritten", name)
}

// builtinCheckoutTracking creates target.Branch at the remote-tracking branch
// and records it as the branch's upstream, as `git checkout --track -b` does.
func builtinCheckoutTracking(dir string, target BranchTarget) error {
	repo, err := openRepo(dir)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := refuseUncommitted(wt); err != nil {
		return err
	}
	cfg, err := repo.Config()
	if err != nil {
		return err
	}
	ref := plumbing.ReferenceName(target.upstreamRef())
	source, err := trackingSource(cfg.Remotes[target.Remote], ref)
	if err != nil {
		return err
	}
	start, err := repo.Reference(ref, true)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", ref, err)
	}
	tip, err := repo.CommitObject(start.Hash())
	if err != nil {
		return err
	}
	next, err := tip.Tree()
	if err != nil {
		return err
	}
	current := &object.Tree{}
	if head, err := repo.Head(); err == nil {
		commit, err := repo.CommitObject(head.Hash())
		if err != nil {
			return err
		}
		current, err = commit.Tree()
		if err != nil {
			return err
		}
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return err
	}
	if err := refuseUntrackedCollisions(wt.Filesystem, current, next); err != nil {
		return err
	}
	if err := wt.Checkout(&git.CheckoutOptions{
		Hash:   start.Hash(),
		Branch: plumbing.NewBranchReferenceName(target.Branch),
		Create: true,
	}); err != nil {
		return err
	}
	cfg.Branches[target.Branch] = &gitconfig.Branch{
		Name:   target.Branch,
		Remote: target.Remote,
		Merge:  source,
	}
	return repo.SetConfig(cfg)
}

// builtinFetchRemotes refreshes each remote in-process, pruned and without tags.
func builtinFetchRemotes(ctx context.Context, dir string) ([]string, error) {
	installFileTransport()
	repo, err := openRepo(dir)
	if err != nil {
		return nil, err
	}
	cfg, err := repo.Config()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cfg.Remotes))
	settings := map[string]remoteSettings{}
	for name, rc := range cfg.Remotes {
		names = append(names, name)
		var s remoteSettings
		for _, spec := range rc.Fetch {
			s.refspecs = append(s.refspecs, spec.String())
		}
		if sub := cfg.Raw.Section("remote").Subsection(name); sub.HasOption("skipFetchAll") {
			s.skipFetchAll = gitBool(sub.Option("skipFetchAll"))
		}
		settings[name] = s
	}
	sort.Strings(names)
	remotes, problems := fetchPlan(names, settings)
	for _, name := range remotes {
		err := repo.FetchContext(ctx, &git.FetchOptions{
			RemoteName: name,
			Prune:      true,
			Tags:       git.NoTags,
		})
		if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
			continue
		}
		if ctx.Err() != nil {
			return remotes, fmt.Errorf("fetch remotes: %w", ctx.Err())
		}
		problems = append(problems, fmt.Sprintf("remote %s: %v", name, err))
	}
	if len(problems) > 0 {
		return remotes, errors.New(redactURLCredentials(strings.Join(problems, "; ")))
	}
	return remotes, nil
}
