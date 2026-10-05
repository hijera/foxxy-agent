package session

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func writeWorkspaceFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return p
}

// diffShape flattens a diff into "path:before->after" lines for comparison.
func diffShape(d *WorkspaceDiff) []string {
	if d == nil {
		return nil
	}
	side := func(f *WorkspaceFile) string {
		if f == nil {
			return "<none>"
		}
		return string(f.Content)
	}
	var out []string
	for _, c := range d.Changes {
		out = append(out, filepath.ToSlash(c.Path)+":"+side(c.Before)+"->"+side(c.After))
	}
	sort.Strings(out)
	return out
}

// The changes card, opened while a turn runs, asks what the turn has done after
// every tool call, so the live comparison has to give the answer the stored
// diff will give when the turn ends.
func TestLiveWorkspaceDiffAgreesWithTheStoredDiff(t *testing.T) {
	cwd := t.TempDir()
	writeWorkspaceFile(t, cwd, "keep.txt", "same\n")
	writeWorkspaceFile(t, cwd, "edit.txt", "one\n")
	writeWorkspaceFile(t, cwd, "gone.txt", "bye\n")
	writeWorkspaceFile(t, cwd, filepath.Join("src", "deep.js"), "a\n")
	before := TakeWorkspaceSnapshot(cwd)

	writeWorkspaceFile(t, cwd, "edit.txt", "one, then two\n")
	writeWorkspaceFile(t, cwd, "new.txt", "hello\n")
	if err := os.Remove(filepath.Join(cwd, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, cwd, filepath.Join("src", "deep.js"), "a\nb\n")
	// Tool state stays out of both.
	writeWorkspaceFile(t, cwd, filepath.Join(".svn", "wc.db"), "db")

	live, err := LiveWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatalf("LiveWorkspaceDiff: %v", err)
	}
	stored, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatalf("ComputeWorkspaceDiff: %v", err)
	}
	got, want := diffShape(live), diffShape(stored)
	if len(want) != 4 {
		t.Fatalf("stored diff should see 4 changes, got %v", want)
	}
	if len(got) != len(want) {
		t.Fatalf("live %v, stored %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("live %v, stored %v", got, want)
		}
	}
}

// Nothing changed yet: the card of a turn that has only read files stays empty.
func TestLiveWorkspaceDiffOfAnUntouchedWorkspaceIsEmpty(t *testing.T) {
	cwd := t.TempDir()
	writeWorkspaceFile(t, cwd, "a.txt", "a\n")
	before := TakeWorkspaceSnapshot(cwd)
	live, err := LiveWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if live != nil && len(live.Changes) != 0 {
		t.Fatalf("want no changes, got %v", diffShape(live))
	}
}

// The live view trusts size and modification time, which is what makes it cheap
// enough to run after every tool call. A same-size rewrite that also carries the
// old timestamp therefore goes unseen until the turn ends - and the stored diff,
// the one a rollback replays, compares contents and still catches it.
func TestLiveWorkspaceDiffTrustsStatTheStoredDiffDoesNot(t *testing.T) {
	cwd := t.TempDir()
	p := writeWorkspaceFile(t, cwd, "a.txt", "aaaa\n")
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	before := TakeWorkspaceSnapshot(cwd)

	writeWorkspaceFile(t, cwd, "a.txt", "bbbb\n")
	if err := os.Chtimes(p, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	live, err := LiveWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if live != nil && len(live.Changes) != 0 {
		t.Fatalf("live view read an unchanged-looking file: %v", diffShape(live))
	}
	stored, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if got := diffShape(stored); len(got) != 1 || got[0] != "a.txt:aaaa\n->bbbb\n" {
		t.Fatalf("stored diff must compare contents, got %v", got)
	}

	// A later write moves the timestamp, and the live view sees it.
	later := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	live, err = LiveWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if got := diffShape(live); len(got) != 1 || got[0] != "a.txt:aaaa\n->bbbb\n" {
		t.Fatalf("live view should see a moved timestamp, got %v", got)
	}
}

// While a turn runs, its edits have no stored diff yet; the card folds the
// live one in as the newest turn.
func TestAggregateSessionChangesWithLiveFoldsTheRunningTurn(t *testing.T) {
	dir := t.TempDir()
	storeTurn(t, dir, 1, WorkspaceChange{Path: "a.txt", Before: file("v1\n"), After: file("v2\n")})
	live := &WorkspaceDiff{Changes: []WorkspaceChange{
		{Path: "a.txt", Before: file("v2\n"), After: file("v3\n")},
		{Path: "b.txt", After: file("new\n")},
	}}

	got, err := AggregateSessionChangesWithLive(dir, live)
	if err != nil {
		t.Fatalf("AggregateSessionChangesWithLive: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 changes, got %+v", got)
	}
	if got[0].Path != "a.txt" || got[0].Kind != FileModified || string(got[0].Before) != "v1\n" || string(got[0].After) != "v3\n" {
		t.Fatalf("a.txt should span both turns: %+v", got[0])
	}
	if got[1].Path != "b.txt" || got[1].Kind != FileAdded {
		t.Fatalf("b.txt should be added: %+v", got[1])
	}
}

// A turn's diff is stored while a request may still read its live entry, so
// the same turn can be folded in twice. That must not change the answer.
func TestAggregateSessionChangesWithLiveToleratesTheStoredTurn(t *testing.T) {
	dir := t.TempDir()
	turn := []WorkspaceChange{
		{Path: "a.txt", Before: file("v1\n"), After: file("v2\n")},
		{Path: "b.txt", After: file("new\n")},
		{Path: "c.txt", Before: file("old\n")},
	}
	storeTurn(t, dir, 1, turn...)

	plain, err := AggregateSessionChanges(dir)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := AggregateSessionChangesWithLive(dir, &WorkspaceDiff{Changes: turn})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != len(twice) {
		t.Fatalf("plain %+v, twice %+v", plain, twice)
	}
	for i := range plain {
		a, b := plain[i], twice[i]
		if a.Path != b.Path || a.Kind != b.Kind || string(a.Before) != string(b.Before) || string(a.After) != string(b.After) {
			t.Fatalf("row %d differs: %+v vs %+v", i, a, b)
		}
	}
}

// The last-turn scope of a running turn is the live diff on its own.
func TestAggregateWorkspaceDiffReportsOneTurn(t *testing.T) {
	got := AggregateWorkspaceDiff(&WorkspaceDiff{Changes: []WorkspaceChange{
		{Path: "x.txt", Before: file("a\n"), After: file("b\n")},
		{Path: filepath.Join(".svn", "wc.db"), Before: file("1"), After: file("2")},
	}})
	if len(got) != 1 || got[0].Path != "x.txt" || got[0].Kind != FileModified {
		t.Fatalf("want x.txt modified only, got %+v", got)
	}
	if AggregateWorkspaceDiff(nil) != nil {
		t.Fatal("a nil diff has no changes")
	}
}
