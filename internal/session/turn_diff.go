package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/gitws"
)

// ignoredDirs are skipped when walking the workspace.
var ignoredDirs = map[string]bool{
	".git": true, ".svn": true, "node_modules": true, "__pycache__": true,
	".venv": true, "venv": true, ".tox": true,
	"vendor": true, ".vendor": true,
	"dist": true, "build": true, ".next": true, "out": true,
	"target": true, ".gradle": true,
	".cache": true, ".sass-cache": true, ".mypy_cache": true,
}

const maxScanDepth = 20

// The snapshot limits are variables only so tests can shrink them.
var (
	// snapshotContentBudget bounds the bytes of file content one snapshot
	// holds. The content is what a rollback writes back; everything past the
	// budget is known by size, time and mode only. The old limit of 100 MB,
	// held twice per turn, ran IDE backends on large checkouts out of memory.
	snapshotContentBudget int64 = 32 * 1024 * 1024
	// snapshotFileContentCap is the largest file whose content is kept.
	snapshotFileContentCap int64 = 4 * 1024 * 1024
	// snapshotMaxFiles bounds how many files one snapshot tracks at all.
	snapshotMaxFiles = 200000
)

// readWorkspaceFile is os.ReadFile; tests replace it to see which files a
// snapshot or a diff reads.
var readWorkspaceFile = os.ReadFile

// WorkspaceFile holds the content and permissions of a single file.
type WorkspaceFile struct {
	Content []byte      `json:"content"` // binary content (encoding/json encodes as base64)
	Mode    fs.FileMode `json:"mode"`
}

// WorkspaceChange records what happened to one file during a turn.
type WorkspaceChange struct {
	Path   string         `json:"path"`
	Before *WorkspaceFile `json:"before,omitempty"` // nil and not BeforeUnavailable → file was created this turn
	After  *WorkspaceFile `json:"after,omitempty"`  // nil → file was deleted this turn
	// BeforeUnavailable marks a file that existed before the turn, or may
	// have, whose content the snapshot did not hold: too large for the
	// content budget, or outside a snapshot that tracked too many files. A
	// rollback leaves such a file as it is instead of deleting it.
	BeforeUnavailable bool `json:"before_unavailable,omitempty"`
}

// WorkspaceDiff is the set of file changes captured during one turn.
type WorkspaceDiff struct {
	Changes []WorkspaceChange `json:"changes"`
}

// WorkspaceSnapshot is a pre-turn snapshot used to compute the delta afterwards.
type WorkspaceSnapshot struct {
	files map[string]*snapshotEntry // relative path → state
	// truncated is set when the walk stopped at snapshotMaxFiles: a file
	// missing from files may then have existed before the turn.
	truncated bool
}

// snapshotEntry is what a snapshot knows about one file. Every tracked file
// has its metadata; hasContent says whether content holds its bytes.
type snapshotEntry struct {
	size       int64
	modTime    time.Time
	mode       fs.FileMode
	content    []byte
	hasContent bool
}

// walkWorkspaceFiles calls visit for every regular file under cwd that a turn
// snapshot covers, in lexical order, and stops early when visit returns false.
func walkWorkspaceFiles(cwd string, visit func(rel, abs string, info fs.FileInfo) bool) {
	_ = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			// The worktrees folder holds whole checkouts of other branches: a
			// turn here neither reads them nor rolls them back.
			if ignoredDirs[d.Name()] || gitws.IsWorktreesRoot(path) {
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
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(cwd, path)
		if !visit(rel, path, info) {
			return filepath.SkipAll
		}
		return nil
	})
}

// TakeWorkspaceSnapshot records the current state of the files under cwd:
// size, time and mode for every file it tracks, and the content of as many of
// them as fit snapshotContentBudget, smallest first, so a rollback can restore
// the most files for the memory it spends.
// Returns a non-nil snapshot even when cwd is empty (snapshot will be empty).
func TakeWorkspaceSnapshot(cwd string) *WorkspaceSnapshot {
	snap := &WorkspaceSnapshot{files: make(map[string]*snapshotEntry)}
	if cwd == "" {
		return snap
	}
	type candidate struct {
		abs   string
		entry *snapshotEntry
	}
	var candidates []candidate
	walkWorkspaceFiles(cwd, func(rel, abs string, info fs.FileInfo) bool {
		if len(snap.files) >= snapshotMaxFiles {
			snap.truncated = true
			return false
		}
		e := &snapshotEntry{size: info.Size(), modTime: info.ModTime(), mode: info.Mode()}
		snap.files[rel] = e
		if e.size <= snapshotFileContentCap {
			candidates = append(candidates, candidate{abs: abs, entry: e})
		}
		return true
	})
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].entry.size < candidates[j].entry.size })
	var held int64
	for _, c := range candidates {
		if held+c.entry.size > snapshotContentBudget {
			break
		}
		content, err := readWorkspaceFile(c.abs)
		if err != nil || int64(len(content)) != c.entry.size {
			// Unreadable, or changed while the snapshot was taken: keep the
			// metadata only rather than content that does not match it.
			continue
		}
		c.entry.content, c.entry.hasContent = content, true
		held += c.entry.size
	}
	return snap
}

// ComputeWorkspaceDiff compares the current state of cwd against the before snapshot
// and returns a WorkspaceDiff describing what changed. Returns nil diff if nothing changed.
//
// It walks the tree again but reads only what it must: a file the snapshot
// holds content for is compared byte for byte, one file at a time, so an edit
// that keeps the size and the time is still seen; any other file is compared
// by size, time and mode and read only when those moved.
func ComputeWorkspaceDiff(cwd string, before *WorkspaceSnapshot) (*WorkspaceDiff, error) {
	if before == nil {
		before = &WorkspaceSnapshot{files: map[string]*snapshotEntry{}}
	}
	var changes []WorkspaceChange
	seen := make(map[string]bool)
	var afterHeld int64

	// afterFile carries a changed file's new content when it fits what is left
	// of the budget. Nothing reads it back today, so past the budget only the
	// mode is recorded rather than holding the tree in memory again.
	afterFile := func(abs string, info fs.FileInfo, content []byte, haveContent bool) *WorkspaceFile {
		af := &WorkspaceFile{Mode: info.Mode()}
		if info.Size() > snapshotFileContentCap || afterHeld+info.Size() > snapshotContentBudget {
			return af
		}
		if !haveContent {
			data, err := readWorkspaceFile(abs)
			if err != nil {
				return af
			}
			content = data
		}
		af.Content = content
		afterHeld += int64(len(content))
		return af
	}

	if cwd != "" {
		walked := 0
		walkWorkspaceFiles(cwd, func(rel, abs string, info fs.FileInfo) bool {
			if walked >= snapshotMaxFiles {
				return false
			}
			walked++
			seen[rel] = true
			be := before.files[rel]
			switch {
			case be == nil:
				// Past a truncated snapshot there is no telling whether the
				// file was there before the turn.
				changes = append(changes, WorkspaceChange{
					Path:              rel,
					After:             afterFile(abs, info, nil, false),
					BeforeUnavailable: before.truncated,
				})
			case be.hasContent:
				bf := &WorkspaceFile{Content: be.content, Mode: be.mode}
				if info.Size() != be.size || info.Mode() != be.mode {
					changes = append(changes, WorkspaceChange{Path: rel, Before: bf, After: afterFile(abs, info, nil, false)})
					return true
				}
				data, err := readWorkspaceFile(abs)
				if err != nil || bytes.Equal(data, be.content) {
					return true
				}
				changes = append(changes, WorkspaceChange{Path: rel, Before: bf, After: afterFile(abs, info, data, true)})
			default:
				if info.Size() == be.size && info.Mode() == be.mode && info.ModTime().Equal(be.modTime) {
					return true
				}
				changes = append(changes, WorkspaceChange{
					Path:              rel,
					After:             afterFile(abs, info, nil, false),
					BeforeUnavailable: true,
				})
			}
			return true
		})
	}

	// Files the snapshot tracked that the walk did not see. A walk cut short
	// by the file limit does not prove a file is gone, so ask the file system.
	for rel, be := range before.files {
		if seen[rel] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(cwd, rel)); !os.IsNotExist(err) {
			continue
		}
		ch := WorkspaceChange{Path: rel}
		if be.hasContent {
			ch.Before = &WorkspaceFile{Content: be.content, Mode: be.mode}
		} else {
			ch.BeforeUnavailable = true
		}
		changes = append(changes, ch)
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
		left, err := reverseWorkspaceDiff(cwd, diff)
		if err != nil {
			skipped++
			msgs = append(msgs, fmt.Sprintf("turn %d partial rollback: %v", n, err))
		} else {
			applied++
			msgs = append(msgs, fmt.Sprintf("reversed turn %d (%d file(s))", n, len(diff.Changes)-len(left)))
		}
		if len(left) > 0 {
			msgs = append(msgs, fmt.Sprintf("turn %d left as is, too large to snapshot: %s", n, strings.Join(left, ", ")))
		}
	}

	if len(msgs) == 0 {
		return "no file changes to roll back", nil
	}
	return strings.Join(msgs, "; ") + fmt.Sprintf(" (%d applied, %d skipped)", applied, skipped), nil
}

// reverseWorkspaceDiff restores each file to its Before state (or removes it
// when the turn created it). A change whose Before content the snapshot did
// not hold cannot be undone and is left alone; its path is returned.
func reverseWorkspaceDiff(cwd string, diff *WorkspaceDiff) ([]string, error) {
	var errs, left []string
	for _, ch := range diff.Changes {
		absPath := filepath.Join(cwd, ch.Path)
		switch {
		case ch.BeforeUnavailable:
			left = append(left, ch.Path)
		case ch.Before == nil:
			// File was created during the turn; delete it.
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("remove %s: %v", ch.Path, err))
			}
		default:
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
			}
		}
	}
	if len(errs) > 0 {
		return left, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return left, nil
}
