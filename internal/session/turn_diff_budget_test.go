package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withSnapshotLimits shrinks the workspace snapshot limits for one test, so a
// budget can be exceeded with a handful of bytes instead of hundreds of MB.
func withSnapshotLimits(t *testing.T, contentBudget, fileContentCap int64, maxFiles int) {
	t.Helper()
	oldBudget, oldCap, oldFiles := snapshotContentBudget, snapshotFileContentCap, snapshotMaxFiles
	snapshotContentBudget, snapshotFileContentCap, snapshotMaxFiles = contentBudget, fileContentCap, maxFiles
	t.Cleanup(func() {
		snapshotContentBudget, snapshotFileContentCap, snapshotMaxFiles = oldBudget, oldCap, oldFiles
	})
}

func writeWorkspaceFile(t *testing.T, cwd, rel, body string) {
	t.Helper()
	p := filepath.Join(cwd, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readWorkspaceText(t *testing.T, cwd, rel string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cwd, rel))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func changeFor(diff *WorkspaceDiff, rel string) *WorkspaceChange {
	if diff == nil {
		return nil
	}
	for i := range diff.Changes {
		if diff.Changes[i].Path == rel {
			return &diff.Changes[i]
		}
	}
	return nil
}

// The pre-turn snapshot used to hold the content of every file up to 100 MB,
// and the post-turn one another 100 MB: on a large checkout that was what ran
// a backend out of memory at the start of a turn (VirtualAlloc errno 1455 on
// Windows). Content is now read smallest file first until a budget is spent,
// while every file is still known by its size, time and mode.
func TestWorkspaceSnapshotHoldsContentWithinBudget(t *testing.T) {
	withSnapshotLimits(t, 600, 1000, 100)
	cwd := t.TempDir()
	writeWorkspaceFile(t, cwd, "c.txt", strings.Repeat("c", 300))
	writeWorkspaceFile(t, cwd, "a.txt", strings.Repeat("a", 100))
	writeWorkspaceFile(t, cwd, "b.txt", strings.Repeat("b", 200))
	writeWorkspaceFile(t, cwd, "d.txt", strings.Repeat("d", 400))
	writeWorkspaceFile(t, cwd, "big.bin", strings.Repeat("x", 5000))

	snap := TakeWorkspaceSnapshot(cwd)

	var held int64
	for _, e := range snap.files {
		held += int64(len(e.content))
	}
	if held > snapshotContentBudget {
		t.Fatalf("snapshot holds %d bytes of content, budget is %d", held, snapshotContentBudget)
	}
	for _, rel := range []string{"a.txt", "b.txt", "c.txt"} {
		if e := snap.files[rel]; e == nil || !e.hasContent {
			t.Errorf("%s: the smallest files must be read first and fit the budget", rel)
		}
	}
	for _, rel := range []string{"d.txt", "big.bin"} {
		e := snap.files[rel]
		if e == nil {
			t.Errorf("%s: a file the budget cannot hold must still be known to exist", rel)
			continue
		}
		if e.hasContent {
			t.Errorf("%s: content was read past the budget or the per-file cap", rel)
		}
	}
}

// A file whose content the snapshot could not hold used to be missing from it
// altogether, so a turn that changed it recorded it as created, and rolling
// the branch back deleted it. Such a change is now marked as unrestorable and
// a rollback leaves the file alone, while everything it could hold is still
// restored, recreated or removed as before.
func TestRollbackLeavesAFileTheSnapshotCouldNotHold(t *testing.T) {
	withSnapshotLimits(t, 100, 100, 100)
	cwd := t.TempDir()
	sessionDir := t.TempDir()
	writeWorkspaceFile(t, cwd, "small.txt", "original small")
	writeWorkspaceFile(t, cwd, "gone.txt", "will be deleted")
	writeWorkspaceFile(t, cwd, "huge.txt", strings.Repeat("h", 500))

	before := TakeWorkspaceSnapshot(cwd)

	writeWorkspaceFile(t, cwd, "small.txt", "edited small")
	writeWorkspaceFile(t, cwd, "huge.txt", strings.Repeat("H", 700))
	writeWorkspaceFile(t, cwd, "new.txt", "created by the turn")
	if err := os.Remove(filepath.Join(cwd, "gone.txt")); err != nil {
		t.Fatal(err)
	}

	diff, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	huge := changeFor(diff, "huge.txt")
	if huge == nil || !huge.BeforeUnavailable || huge.Before != nil {
		t.Fatalf("huge.txt change = %+v, want BeforeUnavailable with no Before", huge)
	}
	if ch := changeFor(diff, "new.txt"); ch == nil || ch.Before != nil || ch.BeforeUnavailable {
		t.Fatalf("new.txt change = %+v, want a creation", ch)
	}

	if err := StoreWorkspaceDiff(sessionDir, 1, diff); err != nil {
		t.Fatal(err)
	}
	note, err := RestoreWorkspaceFiles(cwd, sessionDir, 0)
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := readWorkspaceText(t, cwd, "huge.txt"); !ok || got != strings.Repeat("H", 700) {
		t.Errorf("huge.txt after rollback: exists=%v, content changed=%v; it must be left as the turn left it", ok, got != strings.Repeat("H", 700))
	}
	if !strings.Contains(note, "huge.txt") {
		t.Errorf("rollback note %q must name the file it could not restore", note)
	}
	if got, _ := readWorkspaceText(t, cwd, "small.txt"); got != "original small" {
		t.Errorf("small.txt = %q, want it restored", got)
	}
	if got, ok := readWorkspaceText(t, cwd, "gone.txt"); !ok || got != "will be deleted" {
		t.Errorf("gone.txt: exists=%v content=%q, want it recreated", ok, got)
	}
	if _, ok := readWorkspaceText(t, cwd, "new.txt"); ok {
		t.Error("new.txt was created by the turn and must be removed by the rollback")
	}
}

// Comparing after the turn must not read the whole tree back into memory: a
// file the snapshot holds no content for is compared by size, time and mode
// and is only read when those moved.
func TestWorkspaceDiffDoesNotReadUnchangedFilesPastTheBudget(t *testing.T) {
	withSnapshotLimits(t, 1000, 200, 100)
	cwd := t.TempDir()
	writeWorkspaceFile(t, cwd, "big.bin", strings.Repeat("x", 5000))
	writeWorkspaceFile(t, cwd, "edited.txt", "before")
	writeWorkspaceFile(t, cwd, "same.txt", "unchanged")

	before := TakeWorkspaceSnapshot(cwd)
	writeWorkspaceFile(t, cwd, "edited.txt", "after the turn")

	var read []string
	orig := readWorkspaceFile
	readWorkspaceFile = func(name string) ([]byte, error) {
		read = append(read, filepath.Base(name))
		return orig(name)
	}
	t.Cleanup(func() { readWorkspaceFile = orig })

	diff, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range read {
		if name == "big.bin" {
			t.Fatalf("an unchanged file past the per-file cap was read after the turn: reads=%v", read)
		}
	}
	if diff == nil || len(diff.Changes) != 1 || diff.Changes[0].Path != "edited.txt" {
		t.Fatalf("diff = %+v, want only edited.txt", diff)
	}
}

// Metadata alone would miss an edit that keeps the size and lands within the
// file system's time resolution. Where the snapshot holds the content, the
// bytes decide.
func TestWorkspaceDiffCatchesASameSizeEditWithTheSameTime(t *testing.T) {
	withSnapshotLimits(t, 1000, 1000, 100)
	cwd := t.TempDir()
	writeWorkspaceFile(t, cwd, "same.txt", "aaaa")
	p := filepath.Join(cwd, "same.txt")
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	mtime := st.ModTime()

	before := TakeWorkspaceSnapshot(cwd)
	writeWorkspaceFile(t, cwd, "same.txt", "bbbb")
	if err := os.Chtimes(p, time.Now(), mtime); err != nil {
		t.Fatal(err)
	}

	diff, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	ch := changeFor(diff, "same.txt")
	if ch == nil || ch.Before == nil || string(ch.Before.Content) != "aaaa" {
		t.Fatalf("same-size edit with an unchanged time was not recorded: %+v", ch)
	}
}

// A workspace with more files than the snapshot tracks cannot say whether a
// file outside what it saw was created by the turn. Rolling back must then
// neither delete such a file nor touch a tracked file the walk after the turn
// did not reach.
func TestRollbackWithTruncatedSnapshotDeletesNothingItCannotAccountFor(t *testing.T) {
	withSnapshotLimits(t, 1000, 1000, 3)
	cwd := t.TempDir()
	sessionDir := t.TempDir()
	for _, name := range []string{"f1.txt", "f2.txt", "f3.txt", "f4.txt", "f5.txt"} {
		writeWorkspaceFile(t, cwd, name, "orig "+name)
	}

	before := TakeWorkspaceSnapshot(cwd)
	writeWorkspaceFile(t, cwd, "f0.txt", "new, sorts first")
	writeWorkspaceFile(t, cwd, "f5.txt", "edited past the tracked files")

	diff, err := ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if err := StoreWorkspaceDiff(sessionDir, 1, diff); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreWorkspaceFiles(cwd, sessionDir, 0); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"f0.txt", "f1.txt", "f2.txt", "f3.txt", "f4.txt", "f5.txt"} {
		if _, ok := readWorkspaceText(t, cwd, name); !ok {
			t.Errorf("%s was deleted by a rollback over a truncated snapshot", name)
		}
	}
	if got, _ := readWorkspaceText(t, cwd, "f5.txt"); got != "edited past the tracked files" {
		t.Errorf("f5.txt = %q: a file the snapshot never tracked was rewritten", got)
	}
}
