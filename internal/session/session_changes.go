package session

import (
	"bytes"
	"os"
	"sort"

	"github.com/hijera/foxxycode-agent/internal/textenc"
)

// FileChangeKind is what happened to a file over the whole session.
type FileChangeKind string

const (
	FileAdded    FileChangeKind = "added"
	FileModified FileChangeKind = "modified"
	FileDeleted  FileChangeKind = "deleted"
)

// FileChange is the net effect of a session on one file: the content it had
// before the first turn that touched it, and after the last one.
type FileChange struct {
	Path string
	Kind FileChangeKind
	// Before is nil when the file did not exist before the session.
	Before []byte
	// After is nil when the file no longer exists.
	After  []byte
	Binary bool
}

// aggregate accumulates one file's endpoints across turns.
type aggregate struct {
	beforeSeen   bool
	beforeExists bool
	before       []byte
	afterExists  bool
	after        []byte
}

// AggregateSessionChanges collapses every stored per-turn workspace diff of a
// session into one net change per file.
//
// The per-turn diffs are the source of truth for what the agent touched: they
// are captured by snapshotting the workspace around each turn, so an edit made
// by a shell command counts exactly like one made by the edit tool. Collapsing
// them here means the card, the SPA viewer and both IDE plugins all describe
// the same change set.
func AggregateSessionChanges(sessionDir string) ([]FileChange, error) {
	turns, err := ListStoredTurnDiffs(sessionDir)
	if err != nil {
		return nil, err
	}
	if len(turns) == 0 {
		return nil, nil
	}
	// ListStoredTurnDiffs sorts newest first; walk oldest first so "before"
	// comes from the earliest turn that touched each file.
	sort.Ints(turns)
	return aggregateTurns(sessionDir, turns)
}

// LatestTurnNumber returns the newest turn that stored a workspace diff, or 0
// when the session stored none. The viewer's "last turn" scope resolves to this
// before asking for that turn's changes.
func LatestTurnNumber(sessionDir string) (int, error) {
	turns, err := ListStoredTurnDiffs(sessionDir)
	if err != nil {
		return 0, err
	}
	if len(turns) == 0 {
		return 0, nil
	}
	// ListStoredTurnDiffs sorts newest first.
	return turns[0], nil
}

// AggregateTurnChanges reports what one turn did, in the same shape as the
// whole-session fold, so every scope of the viewer renders from one type. A
// turn number with no stored diff yields no changes rather than an error.
func AggregateTurnChanges(sessionDir string, turnN int) ([]FileChange, error) {
	return aggregateTurns(sessionDir, []int{turnN})
}

// aggregateTurns folds the given turns into one net change per file. Turns are
// walked in the order given, so callers pass them oldest first for "before" to
// come from the earliest turn that touched each file.
func aggregateTurns(sessionDir string, turns []int) ([]FileChange, error) {
	byPath := make(map[string]*aggregate)
	for _, n := range turns {
		diff, err := LoadWorkspaceDiff(sessionDir, n)
		if err != nil {
			return nil, err
		}
		if diff == nil {
			continue
		}
		for _, ch := range diff.Changes {
			agg := byPath[ch.Path]
			if agg == nil {
				agg = &aggregate{}
				byPath[ch.Path] = agg
			}
			if !agg.beforeSeen {
				agg.beforeSeen = true
				if ch.Before != nil {
					agg.beforeExists = true
					agg.before = ch.Before.Content
				}
			}
			if ch.After != nil {
				agg.afterExists = true
				agg.after = ch.After.Content
			} else {
				agg.afterExists = false
				agg.after = nil
			}
		}
	}

	out := make([]FileChange, 0, len(byPath))
	for path, agg := range byPath {
		switch {
		case !agg.beforeExists && !agg.afterExists:
			// Created and removed again within the session: nothing to show.
			continue
		case agg.beforeExists && agg.afterExists && bytes.Equal(agg.before, agg.after):
			// Edited and edited back: the session left no trace in this file.
			continue
		}
		change := FileChange{Path: path, Binary: looksBinary(agg.before, agg.after)}
		switch {
		case !agg.beforeExists:
			change.Kind = FileAdded
			change.After = agg.after
		case !agg.afterExists:
			change.Kind = FileDeleted
			change.Before = agg.before
		default:
			change.Kind = FileModified
			change.Before = agg.before
			change.After = agg.after
		}
		out = append(out, change)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// looksBinary reports whether either side is binary, in which case there is no
// line diff to render. textenc owns this decision for the whole tree.
func looksBinary(before, after []byte) bool {
	return textenc.LooksBinary(before) || textenc.LooksBinary(after)
}

// ClearStoredTurnDiffs removes every recorded turn diff of a session.
//
// It is called after a full session rollback: the workspace is back at its
// pre-session state, so the stored diffs no longer describe anything true, and
// leaving them would keep the changed-files card reporting undone work.
func ClearStoredTurnDiffs(sessionDir string) error {
	err := os.RemoveAll(TurnDiffsDir(sessionDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
