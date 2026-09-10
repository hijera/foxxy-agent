package session

import (
	"path/filepath"
	"testing"
)

func file(content string) *WorkspaceFile {
	return &WorkspaceFile{Content: []byte(content), Mode: 0o644}
}

func storeTurn(t *testing.T, dir string, turn int, changes ...WorkspaceChange) {
	t.Helper()
	if err := StoreWorkspaceDiff(dir, turn, &WorkspaceDiff{Changes: changes}); err != nil {
		t.Fatalf("StoreWorkspaceDiff(turn %d): %v", turn, err)
	}
}

func TestAggregateSessionChangesEmpty(t *testing.T) {
	got, err := AggregateSessionChanges(t.TempDir())
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want no changes, got %d", len(got))
	}
}

func TestAggregateSessionChangesKinds(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1,
		WorkspaceChange{Path: "b.txt", Before: file("old\n"), After: file("new\n")},
		WorkspaceChange{Path: "a.txt", After: file("fresh\n")},
		WorkspaceChange{Path: "c.txt", Before: file("gone\n")},
	)

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 changes, got %d: %+v", len(got), got)
	}
	// Sorted by path so the card lists files in a stable order.
	if got[0].Path != "a.txt" || got[1].Path != "b.txt" || got[2].Path != "c.txt" {
		t.Fatalf("unsorted: %s %s %s", got[0].Path, got[1].Path, got[2].Path)
	}
	if got[0].Kind != FileAdded || got[0].Before != nil {
		t.Fatalf("a.txt should be added with no before: %+v", got[0])
	}
	if got[1].Kind != FileModified || string(got[1].Before) != "old\n" || string(got[1].After) != "new\n" {
		t.Fatalf("b.txt wrong: %+v", got[1])
	}
	if got[2].Kind != FileDeleted || got[2].After != nil {
		t.Fatalf("c.txt should be deleted with no after: %+v", got[2])
	}
}

// The card reports the net effect of the whole session, so a file touched by
// several turns keeps the content it had before the first one and after the last.
func TestAggregateSessionChangesSpansTurns(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1, WorkspaceChange{Path: "x.txt", Before: file("v1\n"), After: file("v2\n")})
	storeTurn(t, dir, 2, WorkspaceChange{Path: "x.txt", Before: file("v2\n"), After: file("v3\n")})
	storeTurn(t, dir, 3, WorkspaceChange{Path: "x.txt", Before: file("v3\n"), After: file("v4\n")})

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 change, got %d", len(got))
	}
	if string(got[0].Before) != "v1\n" || string(got[0].After) != "v4\n" {
		t.Fatalf("want v1 -> v4, got %q -> %q", got[0].Before, got[0].After)
	}
	if got[0].Kind != FileModified {
		t.Fatalf("want modified, got %q", got[0].Kind)
	}
}

// Turn numbers are read from file names, so a two-digit turn must not sort
// before a one-digit one.
func TestAggregateSessionChangesOrdersTurnsNumerically(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 2, WorkspaceChange{Path: "x.txt", Before: file("second\n"), After: file("third\n")})
	storeTurn(t, dir, 10, WorkspaceChange{Path: "x.txt", Before: file("third\n"), After: file("last\n")})

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if string(got[0].Before) != "second\n" || string(got[0].After) != "last\n" {
		t.Fatalf("want second -> last, got %q -> %q", got[0].Before, got[0].After)
	}
}

func TestAggregateSessionChangesDropsNetNoOps(t *testing.T) {
	dir := t.TempDir()
	// Created then removed again, and edited then edited back.
	storeTurn(t, dir, 1,
		WorkspaceChange{Path: "scratch.txt", After: file("temp\n")},
		WorkspaceChange{Path: "kept.txt", Before: file("same\n"), After: file("touched\n")},
	)
	storeTurn(t, dir, 2,
		WorkspaceChange{Path: "scratch.txt", Before: file("temp\n")},
		WorkspaceChange{Path: "kept.txt", Before: file("touched\n"), After: file("same\n")},
	)

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want no net changes, got %+v", got)
	}
}

// A file created early and edited later is still an addition, not a modification.
func TestAggregateSessionChangesAddThenEditStaysAdded(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1, WorkspaceChange{Path: "n.txt", After: file("draft\n")})
	storeTurn(t, dir, 2, WorkspaceChange{Path: "n.txt", Before: file("draft\n"), After: file("final\n")})

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if len(got) != 1 || got[0].Kind != FileAdded {
		t.Fatalf("want one added file, got %+v", got)
	}
	if string(got[0].After) != "final\n" {
		t.Fatalf("want final content, got %q", got[0].After)
	}
}

func TestAggregateSessionChangesMarksBinary(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1, WorkspaceChange{
		Path:  "logo.png",
		After: &WorkspaceFile{Content: []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02}, Mode: 0o644},
	})

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if len(got) != 1 || !got[0].Binary {
		t.Fatalf("want one binary change, got %+v", got)
	}
}

func TestAggregateSessionChangesIgnoresUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1, WorkspaceChange{Path: "x.txt", After: file("hi\n")})
	// A stray file in the diffs directory must not break the read.
	if err := writeBytesAtomic(filepath.Join(TurnDiffsDir(dir), "notes.md"), []byte("hello")); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 change, got %d", len(got))
	}
}

// The viewer's "last turn" scope reports only what the newest turn did, so an
// earlier turn's files must not leak into it.
func TestAggregateTurnChangesIsolatesTheTurn(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1, WorkspaceChange{Path: "first.txt", After: file("one\n")})
	storeTurn(t, dir, 2,
		WorkspaceChange{Path: "second.txt", Before: file("old\n"), After: file("new\n")},
	)

	latest, err := LatestTurnNumber(dir)
	if err != nil {
		t.Fatalf("LatestTurnNumber: %v", err)
	}
	if latest != 2 {
		t.Fatalf("want turn 2, got %d", latest)
	}

	got, err := AggregateTurnChanges(dir, latest)
	if err != nil {
		t.Fatalf("AggregateTurnChanges: %v", err)
	}
	if len(got) != 1 || got[0].Path != "second.txt" {
		t.Fatalf("want only second.txt, got %+v", got)
	}
	if got[0].Kind != FileModified || string(got[0].Before) != "old\n" {
		t.Fatalf("second.txt wrong: %+v", got[0])
	}
}

// A turn that edited a file back to its previous content changed nothing, and
// the same no-op rule the session scope applies must hold per turn.
func TestAggregateTurnChangesDropsNoOps(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1,
		WorkspaceChange{Path: "same.txt", Before: file("x\n"), After: file("x\n")},
		WorkspaceChange{Path: "real.txt", Before: file("a\n"), After: file("b\n")},
	)

	got, err := AggregateTurnChanges(dir, 1)
	if err != nil {
		t.Fatalf("AggregateTurnChanges: %v", err)
	}
	if len(got) != 1 || got[0].Path != "real.txt" {
		t.Fatalf("want only real.txt, got %+v", got)
	}
}

// No stored diffs at all: the scope is empty rather than an error, so the
// viewer can render "nothing changed" the same way every other scope does.
func TestLatestTurnNumberEmpty(t *testing.T) {
	latest, err := LatestTurnNumber(t.TempDir())
	if err != nil {
		t.Fatalf("LatestTurnNumber: %v", err)
	}
	if latest != 0 {
		t.Fatalf("want 0 for a session with no turns, got %d", latest)
	}
	got, err := AggregateTurnChanges(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("AggregateTurnChanges: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want no changes, got %+v", got)
	}
}
