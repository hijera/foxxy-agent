package session

import (
	"os"
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

// An IDE rewrites its own settings folder constantly - .idea/workspace.xml moves
// on every caret change - so recording it would put churn nobody asked about at
// the top of every review.
func TestWorkspaceSnapshotSkipsEditorSettings(t *testing.T) {
	cwd := t.TempDir()
	for _, rel := range []string{
		filepath.Join(".idea", "workspace.xml"),
		filepath.Join(".vscode", "settings.json"),
		filepath.Join("src", ".idea", "nested.xml"),
		"real.txt",
	} {
		p := filepath.Join(cwd, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	before := TakeWorkspaceSnapshot(cwd)
	for _, rel := range []string{
		filepath.Join(".idea", "workspace.xml"),
		filepath.Join(".vscode", "settings.json"),
		filepath.Join("src", ".idea", "nested.xml"),
		"real.txt",
	} {
		if err := os.WriteFile(filepath.Join(cwd, rel), []byte("after\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	diff, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatalf("ComputeWorkspaceDiff: %v", err)
	}
	if diff == nil || len(diff.Changes) != 1 {
		t.Fatalf("want only real.txt, got %+v", diff)
	}
	if diff.Changes[0].Path != "real.txt" {
		t.Fatalf("unexpected change: %+v", diff.Changes[0])
	}
}

// .svn is to Subversion what .git is to git: an administrative area the client
// rewrites on its own. A turn where the agent runs any svn command would
// otherwise fill the card with wc.db and pristine copies.
func TestWorkspaceSnapshotSkipsSVNAdminDir(t *testing.T) {
	cwd := t.TempDir()
	for _, rel := range []string{
		filepath.Join(".svn", "wc.db"),
		filepath.Join(".svn", "pristine", "ab", "abc.svn-base"),
		"real.js",
	} {
		p := filepath.Join(cwd, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	before := TakeWorkspaceSnapshot(cwd)
	for _, rel := range []string{
		filepath.Join(".svn", "wc.db"),
		filepath.Join(".svn", "pristine", "ab", "abc.svn-base"),
		"real.js",
	} {
		if err := os.WriteFile(filepath.Join(cwd, rel), []byte("after\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	diff, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatalf("ComputeWorkspaceDiff: %v", err)
	}
	if diff == nil || len(diff.Changes) != 1 || diff.Changes[0].Path != "real.js" {
		t.Fatalf("want only real.js, got %+v", diff)
	}
}

// A session recorded before the snapshot learned to skip a version control
// client's administrative folder still carries it, and in a Subversion working
// copy the pristine blobs are the bulk of such a recording. The aggregate is
// what every viewer and the rollback read, so it is where they stop.
func TestAggregateSessionChangesSkipsToolState(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1,
		WorkspaceChange{Path: filepath.Join(".svn", "pristine", "3b", "3b7f.svn-base"), After: file("blob\n")},
		WorkspaceChange{Path: filepath.Join(".svn", "wc.db"), Before: file("db1"), After: file("db2")},
		WorkspaceChange{Path: filepath.Join(".idea", "workspace.xml"), Before: file("one"), After: file("two")},
		WorkspaceChange{Path: filepath.Join(".vscode", "settings.json"), After: file("{}")},
		WorkspaceChange{Path: filepath.Join(".git", "index"), Before: file("i1"), After: file("i2")},
		// Names that merely look like the folders above stay: the match is on
		// whole path segments.
		WorkspaceChange{Path: "git-notes.txt", After: file("kept\n")},
		WorkspaceChange{Path: filepath.Join("docs", "idea.md"), After: file("kept\n")},
		WorkspaceChange{Path: "app.js", Before: file("a\n"), After: file("b\n")},
	)

	got, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatalf("AggregateSessionChanges: %v", err)
	}
	var paths []string
	for _, c := range got {
		paths = append(paths, filepath.ToSlash(c.Path))
	}
	want := []string{"app.js", "docs/idea.md", "git-notes.txt"}
	if len(paths) != len(want) {
		t.Fatalf("want %v, got %v", want, paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("want %v, got %v", want, paths)
		}
	}
}

// Rolling a session back must not reach into a working copy's bookkeeping.
// Restoring .svn/wc.db to what it held three turns ago leaves Subversion
// describing a tree that no longer exists - the rollback would break the
// working copy it was asked to clean up.
func TestRestoreWorkspaceFilesSkipsToolState(t *testing.T) {
	cwd := t.TempDir()
	dir := t.TempDir()

	writeAt := func(rel, content string) {
		t.Helper()
		p := filepath.Join(cwd, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	readAt := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(cwd, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	writeAt("app.js", "edited\n")
	writeAt(filepath.Join(".svn", "wc.db"), "current")
	writeAt(filepath.Join(".svn", "pristine", "3b", "3b7f.svn-base"), "blob")

	storeTurn(t, dir, 1,
		WorkspaceChange{Path: "app.js", Before: file("original\n"), After: file("edited\n")},
		WorkspaceChange{Path: filepath.Join(".svn", "wc.db"), Before: file("stale"), After: file("current")},
		// No Before: a plain rollback would delete this one.
		WorkspaceChange{Path: filepath.Join(".svn", "pristine", "3b", "3b7f.svn-base"), After: file("blob")},
	)

	if _, err := RestoreWorkspaceFiles(cwd, dir, 0); err != nil {
		t.Fatalf("RestoreWorkspaceFiles: %v", err)
	}
	if got := readAt("app.js"); got != "original\n" {
		t.Fatalf("app.js not rolled back: %q", got)
	}
	if got := readAt(filepath.Join(".svn", "wc.db")); got != "current" {
		t.Fatalf("svn bookkeeping was rewritten: %q", got)
	}
	if got := readAt(filepath.Join(".svn", "pristine", "3b", "3b7f.svn-base")); got != "blob" {
		t.Fatalf("svn pristine copy was rewritten: %q", got)
	}
}
