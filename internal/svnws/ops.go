package svnws

// Working copy operations backing the svn_* tools. Every argument that reaches
// the client is guarded: a value starting with "-" would be parsed as an option,
// so it is rejected rather than allowed to inject flags (same rule as gitws).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AcceptValues are the conflict resolutions accepted by Resolve.
var AcceptValues = []string{
	"working", "base", "mine-full", "theirs-full", "mine-conflict", "theirs-conflict",
}

// guardArg rejects empty values and values that would be parsed as an option.
func guardArg(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("empty %s", name)
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("refusing %s that looks like an option: %q", name, value)
	}
	return nil
}

// guardPaths validates a positional path list, returning the trimmed values.
// An empty list means "the whole working copy" and is left to the caller.
func guardPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := guardArg("path", p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// guardRevision accepts revision numbers, keywords (HEAD, BASE, PREV,
// COMMITTED), ranges ("100:200") and date specifiers ("{2026-01-31}").
func guardRevision(rev string) error {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return nil
	}
	if strings.HasPrefix(rev, "-") {
		return fmt.Errorf("refusing revision that looks like an option: %q", rev)
	}
	for _, r := range rev {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == ':' || r == '{' || r == '}' || r == '-' || r == '.' || r == '+':
		default:
			return fmt.Errorf("unsupported revision specifier: %q", rev)
		}
	}
	return nil
}

// withPaths appends "--" and the positional paths when any were given.
func withPaths(args []string, paths []string) []string {
	if len(paths) == 0 {
		return args
	}
	args = append(args, "--")
	return append(args, paths...)
}

// Status reports local modifications. Empty paths status the whole working copy.
func Status(ctx context.Context, dir string, o Options, paths []string) (string, error) {
	clean, err := guardPaths(paths)
	if err != nil {
		return "", err
	}
	return run(ctx, o, dir, withPaths([]string{"status"}, clean)...)
}

// Diff shows working copy changes, optionally limited to paths and a revision
// (or revision range).
func Diff(ctx context.Context, dir string, o Options, paths []string, revision string) (string, error) {
	clean, err := guardPaths(paths)
	if err != nil {
		return "", err
	}
	if err := guardRevision(revision); err != nil {
		return "", err
	}
	args := []string{"diff"}
	if r := strings.TrimSpace(revision); r != "" {
		args = append(args, "--revision", r)
	}
	return run(ctx, o, dir, withPaths(args, clean)...)
}

// Log lists recent revisions for target (a path or URL); limit caps the entries.
func Log(ctx context.Context, dir string, o Options, target string, limit int) (string, error) {
	args := []string{"log"}
	if limit > 0 {
		args = append(args, "--limit", fmt.Sprintf("%d", limit))
	}
	args = append(args, "-v")
	var paths []string
	if t := strings.TrimSpace(target); t != "" {
		if err := guardArg("target", t); err != nil {
			return "", err
		}
		paths = []string{t}
	}
	return run(ctx, o, dir, withPaths(args, paths)...)
}

// List lists repository entries at target (a path or URL).
func List(ctx context.Context, dir string, o Options, target string) (string, error) {
	var paths []string
	if t := strings.TrimSpace(target); t != "" {
		if err := guardArg("target", t); err != nil {
			return "", err
		}
		paths = []string{t}
	}
	return run(ctx, o, dir, withPaths([]string{"list"}, paths)...)
}

// Add schedules paths for addition. Parent directories are added as needed.
func Add(ctx context.Context, dir string, o Options, paths []string) (string, error) {
	clean, err := guardPaths(paths)
	if err != nil {
		return "", err
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("svn add requires at least one path")
	}
	return run(ctx, o, dir, withPaths([]string{"add", "--parents"}, clean)...)
}

// Revert restores paths to their base revision. Recursive reverts directories.
func Revert(ctx context.Context, dir string, o Options, paths []string, recursive bool) (string, error) {
	clean, err := guardPaths(paths)
	if err != nil {
		return "", err
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("svn revert requires at least one path")
	}
	args := []string{"revert"}
	if recursive {
		args = append(args, "--recursive")
	}
	return run(ctx, o, dir, withPaths(args, clean)...)
}

// Update brings the working copy (or the given paths) to revision, defaulting to HEAD.
func Update(ctx context.Context, dir string, o Options, paths []string, revision string) (string, error) {
	clean, err := guardPaths(paths)
	if err != nil {
		return "", err
	}
	if err := guardRevision(revision); err != nil {
		return "", err
	}
	args := []string{"update"}
	if r := strings.TrimSpace(revision); r != "" {
		args = append(args, "--revision", r)
	}
	return run(ctx, o, dir, withPaths(args, clean)...)
}

// Commit sends changes to the server. Paths are always explicit so unrelated
// parts of the tree (for example a nested git clone) are never swept in.
func Commit(ctx context.Context, dir string, o Options, message string, paths []string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("svn commit requires a message")
	}
	clean, err := guardPaths(paths)
	if err != nil {
		return "", err
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("svn commit requires at least one path")
	}
	args := []string{"commit", "--message", message}
	return run(ctx, o, dir, withPaths(args, clean)...)
}

// Switch repoints the working copy at dir to url (branch switch in place).
func Switch(ctx context.Context, dir string, o Options, url string) (string, error) {
	if err := guardArg("url", url); err != nil {
		return "", err
	}
	return run(ctx, o, dir, "switch", "--", strings.TrimSpace(url))
}

// Checkout creates a new working copy of url at dest. dest must not already be
// a working copy; svn reports that itself.
func Checkout(ctx context.Context, o Options, url, dest, revision string) (string, error) {
	if err := guardArg("url", url); err != nil {
		return "", err
	}
	if err := guardArg("destination", dest); err != nil {
		return "", err
	}
	if err := guardRevision(revision); err != nil {
		return "", err
	}
	args := []string{"checkout"}
	if r := strings.TrimSpace(revision); r != "" {
		args = append(args, "--revision", r)
	}
	dest = strings.TrimSpace(dest)
	args = append(args, "--", strings.TrimSpace(url), dest)
	// Run from the parent so a relative dest resolves predictably; create it so
	// checking a branch out into a fresh folder tree works in one step.
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("checkout destination: %w", err)
	}
	return run(ctx, o, parent, args...)
}

// Merge merges source (a branch URL or path) into the working copy at dir.
func Merge(ctx context.Context, dir string, o Options, source, revision string) (string, error) {
	if err := guardArg("source", source); err != nil {
		return "", err
	}
	if err := guardRevision(revision); err != nil {
		return "", err
	}
	args := []string{"merge"}
	if r := strings.TrimSpace(revision); r != "" {
		args = append(args, "--revision", r)
	}
	args = append(args, "--", strings.TrimSpace(source))
	return run(ctx, o, dir, args...)
}

// Resolve marks conflicts on paths as resolved using the given accept strategy.
func Resolve(ctx context.Context, dir string, o Options, paths []string, accept string) (string, error) {
	clean, err := guardPaths(paths)
	if err != nil {
		return "", err
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("svn resolve requires at least one path")
	}
	accept = strings.TrimSpace(accept)
	if accept == "" {
		accept = "working"
	}
	if !allowedAccept(accept) {
		return "", fmt.Errorf("unsupported accept value %q (want one of %s)",
			accept, strings.Join(AcceptValues, ", "))
	}
	args := []string{"resolve", "--accept", accept}
	return run(ctx, o, dir, withPaths(args, clean)...)
}

func allowedAccept(value string) bool {
	for _, v := range AcceptValues {
		if v == value {
			return true
		}
	}
	return false
}
