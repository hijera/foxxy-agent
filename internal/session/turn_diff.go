package session

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// toolStateDirs are the folders a tool keeps its own bookkeeping in: the
// editor's settings and the version control client's administrative area.
//
// Nothing in them is anyone's edit. An IDE rewrites .idea/workspace.xml when
// the caret moves; svn rewrites wc.db and a pristine copy of every file it
// touches, which is why one svn command buries a three-line edit under a
// hundred binary .svn-base blobs. They are excluded twice over - skipped when
// the workspace is walked, and dropped again when a recording is read back, so
// a session recorded before this rule is as clean as one recorded after.
var toolStateDirs = map[string]bool{
	".git": true, ".svn": true,
	".idea": true, ".vscode": true,
}

// ignoredDirs are skipped when walking the workspace on top of toolStateDirs.
//
// These hold generated content rather than bookkeeping: recording them would
// bury the edits, but a file under them is still a file, so - unlike tool state
// - they are only skipped at capture time. What a working copy reports under
// them stays visible.
var ignoredDirs = map[string]bool{
	"node_modules": true, "__pycache__": true,
	".venv": true, "venv": true, ".tox": true,
	"vendor": true, ".vendor": true,
	"dist": true, "build": true, ".next": true, "out": true,
	"target": true, ".gradle": true,
	".cache": true, ".sass-cache": true, ".mypy_cache": true,
}

// IsToolStatePath reports whether a path lies inside one of the tool state
// folders, at any depth.
//
// Matching whole segments keeps ordinary files - docs/idea.md, .ideas/plan.md,
// git-notes.txt - out of it. Both separators are levelled because a recording
// holds OS-shaped paths while a caller may well ask with forward slashes.
func IsToolStatePath(path string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if toolStateDirs[segment] {
			return true
		}
	}
	return false
}

const (
	maxFileSizeBytes  = 10 * 1024 * 1024  // skip files larger than 10 MB
	maxTotalSizeBytes = 100 * 1024 * 1024 // stop scanning after 100 MB total
	maxScanDepth      = 20
)

// WorkspaceFile holds the content and permissions of a single file.
type WorkspaceFile struct {
	Content []byte      `json:"content"` // binary content (encoding/json encodes as base64)
	Mode    fs.FileMode `json:"mode"`
}

// WorkspaceChange records what happened to one file during a turn.
type WorkspaceChange struct {
	Path   string         `json:"path"`
	Before *WorkspaceFile `json:"before,omitempty"` // nil → file was created this turn
	After  *WorkspaceFile `json:"after,omitempty"`  // nil → file was deleted this turn
}

// WorkspaceDiff is the set of file changes captured during one turn.
type WorkspaceDiff struct {
	Changes []WorkspaceChange `json:"changes"`
}

// WorkspaceSnapshot is a pre-turn snapshot used to compute the delta afterwards.
type WorkspaceSnapshot struct {
	files map[string]*WorkspaceFile // relative path → state
}

// TakeWorkspaceSnapshot records the current state of all files under cwd.
// Returns a non-nil snapshot even when cwd is empty (snapshot will be empty).
func TakeWorkspaceSnapshot(cwd string) *WorkspaceSnapshot {
	snap := &WorkspaceSnapshot{files: make(map[string]*WorkspaceFile)}
	if cwd == "" {
		return snap
	}
	var totalBytes int64
	_ = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if ignoredDirs[d.Name()] || toolStateDirs[d.Name()] {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(cwd, path)
			if strings.Count(rel, string(os.PathSeparator)) >= maxScanDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks, pipes, etc.
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxFileSizeBytes {
			return nil
		}
		if totalBytes+info.Size() > maxTotalSizeBytes {
			return filepath.SkipAll
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(cwd, path)
		snap.files[rel] = &WorkspaceFile{Content: content, Mode: info.Mode()}
		totalBytes += info.Size()
		return nil
	})
	return snap
}

// ComputeWorkspaceDiff compares the current state of cwd against the before snapshot
// and returns a WorkspaceDiff describing what changed. Returns nil diff if nothing changed.
func ComputeWorkspaceDiff(cwd string, before *WorkspaceSnapshot) (*WorkspaceDiff, error) {
	after := TakeWorkspaceSnapshot(cwd)
	beforeFiles := make(map[string]*WorkspaceFile)
	if before != nil {
		beforeFiles = before.files
	}

	var changes []WorkspaceChange
	seen := make(map[string]bool)

	// Files that exist after the turn.
	for rel, af := range after.files {
		seen[rel] = true
		bf := beforeFiles[rel]
		if bf == nil {
			// New file.
			changes = append(changes, WorkspaceChange{Path: rel, After: af})
		} else if string(bf.Content) != string(af.Content) || bf.Mode != af.Mode {
			// Modified file.
			changes = append(changes, WorkspaceChange{Path: rel, Before: bf, After: af})
		}
	}

	// Files that existed before but are gone now.
	for rel, bf := range beforeFiles {
		if !seen[rel] {
			changes = append(changes, WorkspaceChange{Path: rel, Before: bf})
		}
	}

	if len(changes) == 0 {
		return nil, nil
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return &WorkspaceDiff{Changes: changes}, nil
}

// StoreWorkspaceDiff writes the diff to <sessionDir>/diffs/turn_<n>.json atomically.
// If diff is nil (no changes), no file is written.
func StoreWorkspaceDiff(sessionDir string, turnN int, diff *WorkspaceDiff) error {
	if diff == nil || len(diff.Changes) == 0 {
		return nil
	}
	dir := TurnDiffsDir(sessionDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(diff)
	if err != nil {
		return fmt.Errorf("marshal workspace diff: %w", err)
	}
	p := filepath.Join(dir, fmt.Sprintf("turn_%d.json", turnN))
	return writeBytesAtomic(p, data)
}

// LoadWorkspaceDiff reads a stored turn diff. Returns nil if the file does not exist.
func LoadWorkspaceDiff(sessionDir string, turnN int) (*WorkspaceDiff, error) {
	p := filepath.Join(TurnDiffsDir(sessionDir), fmt.Sprintf("turn_%d.json", turnN))
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var diff WorkspaceDiff
	if err := json.Unmarshal(data, &diff); err != nil {
		return nil, fmt.Errorf("parse workspace diff: %w", err)
	}
	return &diff, nil
}

// ListStoredTurnDiffs returns turn indices that have stored diff files, sorted descending.
func ListStoredTurnDiffs(sessionDir string) ([]int, error) {
	dir := TurnDiffsDir(sessionDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var nums []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "turn_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		s := strings.TrimPrefix(name, "turn_")
		s = strings.TrimSuffix(s, ".json")
		if n, err := strconv.Atoi(s); err == nil {
			nums = append(nums, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(nums)))
	return nums, nil
}

// RestoreWorkspaceFiles restores workspace files by reversing all turn diffs with
// turn number > afterTurn. Pure Go, no external tools required.
func RestoreWorkspaceFiles(cwd, sessionDir string, afterTurn int) (string, error) {
	if cwd == "" {
		return "no workspace cwd; file rollback skipped", nil
	}
	all, err := ListStoredTurnDiffs(sessionDir)
	if err != nil {
		return "", err
	}

	var applied, skipped int
	var msgs []string

	for _, n := range all {
		if n <= afterTurn {
			break
		}
		diff, err := LoadWorkspaceDiff(sessionDir, n)
		if err != nil || diff == nil || len(diff.Changes) == 0 {
			skipped++
			continue
		}
		restored, err := reverseWorkspaceDiff(cwd, diff)
		if err != nil {
			skipped++
			msgs = append(msgs, fmt.Sprintf("turn %d partial rollback: %v", n, err))
		} else {
			applied++
			// Counted from what was written rather than from what was recorded:
			// tool state is skipped below and never was the user's edit.
			msgs = append(msgs, fmt.Sprintf("reversed turn %d (%d file(s))", n, restored))
		}
	}

	if len(msgs) == 0 {
		return "no file changes to roll back", nil
	}
	return strings.Join(msgs, "; ") + fmt.Sprintf(" (%d applied, %d skipped)", applied, skipped), nil
}

// reverseWorkspaceDiff restores each file to its Before state (or removes if Before is nil),
// and reports how many it wrote.
func reverseWorkspaceDiff(cwd string, diff *WorkspaceDiff) (int, error) {
	var errs []string
	restored := 0
	for _, ch := range diff.Changes {
		if IsToolStatePath(ch.Path) {
			// Recorded by an older build. Putting wc.db or .git/index back to
			// what it held before the session would leave the client describing
			// a tree that is no longer there - a rollback must not break the
			// working copy it was asked to clean up.
			continue
		}
		absPath := filepath.Join(cwd, ch.Path)
		if ch.Before == nil {
			// File was created during the turn; delete it.
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("remove %s: %v", ch.Path, err))
			} else {
				restored++
			}
		} else {
			// File was modified or deleted; restore original content.
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				errs = append(errs, fmt.Sprintf("mkdir for %s: %v", ch.Path, err))
				continue
			}
			mode := ch.Before.Mode
			if mode == 0 {
				mode = 0o644
			}
			if err := os.WriteFile(absPath, ch.Before.Content, mode); err != nil {
				errs = append(errs, fmt.Sprintf("restore %s: %v", ch.Path, err))
			} else {
				restored++
			}
		}
	}
	if len(errs) > 0 {
		return restored, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return restored, nil
}
