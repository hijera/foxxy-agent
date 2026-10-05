package svnws_test

// Working-copy change coverage for the review window's git-equivalent scopes.
// Skipped when no Subversion client is installed, like the rest of the
// integration tests in this package.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/svnws"
)

// newWorkingCopy creates a repository holding the given files and checks it out.
func newWorkingCopy(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	wc := filepath.Join(root, "wc")
	mustRun(t, root, "svnadmin", "create", repo)
	mustRun(t, root, "svn", "checkout", fileURL(repo), wc)

	for name, body := range files {
		writeWCFile(t, wc, name, body)
		mustRun(t, wc, "svn", "add", "--parents", filepath.FromSlash(name))
	}
	if len(files) > 0 {
		mustRun(t, wc, "svn", "commit", "-m", "init")
	}
	return wc
}

func writeWCFile(t *testing.T, wc, name, body string) {
	t.Helper()
	p := filepath.Join(wc, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func changesByPath(changes []svnws.WorkChange) map[string]svnws.WorkChange {
	out := make(map[string]svnws.WorkChange, len(changes))
	for _, c := range changes {
		out[c.Path] = c
	}
	return out
}

func TestWorkingCopyChangesReportsEveryKind(t *testing.T) {
	opts := realSVN(t)
	wc := newWorkingCopy(t, map[string]string{
		"keep.txt":     "same\n",
		"edit.txt":     "old\n",
		"remove.txt":   "bye\n",
		"dir/deep.txt": "deep\n",
	})
	writeWCFile(t, wc, "edit.txt", "new\n")
	writeWCFile(t, wc, "fresh.txt", "added\n")
	mustRun(t, wc, "svn", "add", "fresh.txt")
	mustRun(t, wc, "svn", "delete", "remove.txt")

	changes, skipped, err := svnws.WorkingCopyChanges(context.Background(), wc, opts, false)
	if err != nil {
		t.Fatalf("WorkingCopyChanges: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("nothing is unversioned here, got skipped=%d", skipped)
	}
	got := changesByPath(changes)
	if len(got) != 3 {
		t.Fatalf("want 3 changes, got %d: %+v", len(got), changes)
	}
	if c := got["edit.txt"]; c.Status != "modified" ||
		string(c.Before) != "old\n" || string(c.After) != "new\n" {
		t.Fatalf("edit.txt wrong: %+v", c)
	}
	if c := got["fresh.txt"]; c.Status != "added" || c.Before != nil ||
		string(c.After) != "added\n" {
		t.Fatalf("fresh.txt wrong: %+v", c)
	}
	if c := got["remove.txt"]; c.Status != "deleted" || c.After != nil ||
		string(c.Before) != "bye\n" {
		t.Fatalf("remove.txt wrong: %+v", c)
	}
	if _, ok := got["keep.txt"]; ok {
		t.Fatal("an unchanged file must not be reported")
	}
}

// The tracked-only scope counts unversioned files without reading them, exactly
// as the git side does.
func TestWorkingCopyChangesCountsUnversionedWithoutListing(t *testing.T) {
	opts := realSVN(t)
	wc := newWorkingCopy(t, map[string]string{"a.txt": "a\n"})
	writeWCFile(t, wc, "junk1.txt", "x\n")
	writeWCFile(t, wc, "junk2.txt", "y\n")

	changes, skipped, err := svnws.WorkingCopyChanges(context.Background(), wc, opts, false)
	if err != nil {
		t.Fatalf("WorkingCopyChanges: %v", err)
	}
	if skipped != 2 {
		t.Fatalf("want 2 unversioned counted, got %d", skipped)
	}
	if len(changes) != 0 {
		t.Fatalf("unversioned files must not be listed: %+v", changes)
	}
}

func TestWorkingCopyChangesIncludesUnversioned(t *testing.T) {
	opts := realSVN(t)
	wc := newWorkingCopy(t, map[string]string{"a.txt": "a\n"})
	writeWCFile(t, wc, "fresh.txt", "brand new\n")
	writeWCFile(t, wc, "deep/nested.txt", "also new\n")

	changes, skipped, err := svnws.WorkingCopyChanges(context.Background(), wc, opts, true)
	if err != nil {
		t.Fatalf("WorkingCopyChanges: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("nothing should be skipped, got %d", skipped)
	}
	got := changesByPath(changes)
	if c := got["fresh.txt"]; c.Status != "added" || string(c.After) != "brand new\n" {
		t.Fatalf("fresh.txt wrong: %+v", c)
	}
	if c := got[filepath.FromSlash("deep/nested.txt")]; string(c.After) != "also new\n" {
		t.Fatalf("nested unversioned file missing or wrong: %+v", c)
	}
}

// svn stores pristine text with the repository's line endings, so the two sides
// must be levelled the same way the git side levels them.
func TestWorkingCopyChangesLevelsLineEndings(t *testing.T) {
	opts := realSVN(t)
	wc := newWorkingCopy(t, map[string]string{"crlf.txt": "alpha\nbeta\n"})
	writeWCFile(t, wc, "crlf.txt", "alpha\r\nBETA\r\n")

	changes, _, err := svnws.WorkingCopyChanges(context.Background(), wc, opts, false)
	if err != nil {
		t.Fatalf("WorkingCopyChanges: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %+v", changes)
	}
	before, after := string(changes[0].Before), string(changes[0].After)
	if strings.Contains(before, "\r") || strings.Contains(after, "\r") {
		t.Fatalf("sides must be levelled: before=%q after=%q", before, after)
	}
	if before != "alpha\nbeta\n" || after != "alpha\nBETA\n" {
		t.Fatalf("wrong sides: before=%q after=%q", before, after)
	}
}

func TestWorkingCopyChangeForReadsOneFile(t *testing.T) {
	opts := realSVN(t)
	wc := newWorkingCopy(t, map[string]string{"a.txt": "a1\n", "b.txt": "b1\n"})
	writeWCFile(t, wc, "a.txt", "a2\n")
	writeWCFile(t, wc, "b.txt", "b2\n")
	writeWCFile(t, wc, "loose.txt", "loose\n")

	change, err := svnws.WorkingCopyChangeFor(context.Background(), wc, opts, "b.txt", false)
	if err != nil {
		t.Fatalf("WorkingCopyChangeFor: %v", err)
	}
	if change == nil || change.Status != "modified" ||
		string(change.Before) != "b1\n" || string(change.After) != "b2\n" {
		t.Fatalf("b.txt wrong: %+v", change)
	}

	// Unversioned only resolves in the scope that asked for it.
	if c, err := svnws.WorkingCopyChangeFor(context.Background(), wc, opts, "loose.txt", false); err != nil || c != nil {
		t.Fatalf("loose.txt must not resolve without unversioned: %+v %v", c, err)
	}
	c, err := svnws.WorkingCopyChangeFor(context.Background(), wc, opts, "loose.txt", true)
	if err != nil {
		t.Fatalf("WorkingCopyChangeFor: %v", err)
	}
	if c == nil || string(c.After) != "loose\n" {
		t.Fatalf("loose.txt wrong: %+v", c)
	}
}

// The path arrives from a request, so anything svn does not report as changed
// must stay unreadable through this route.
func TestWorkingCopyChangeForRefusesUnlistedPaths(t *testing.T) {
	opts := realSVN(t)
	wc := newWorkingCopy(t, map[string]string{"a.txt": "a1\n", "clean.txt": "c\n"})
	writeWCFile(t, wc, "a.txt", "a2\n")

	for _, p := range []string{"clean.txt", "../escape.txt", "missing.txt", ""} {
		change, err := svnws.WorkingCopyChangeFor(context.Background(), wc, opts, p, true)
		if err != nil {
			t.Fatalf("path %q: %v", p, err)
		}
		if change != nil {
			t.Fatalf("path %q must not resolve: %+v", p, change)
		}
	}
}

// A folder that is not a working copy is an empty result, not an error, so the
// viewer can offer the scope everywhere.
func TestWorkingCopyChangesOutsideAWorkingCopy(t *testing.T) {
	opts := realSVN(t)
	changes, skipped, err := svnws.WorkingCopyChanges(context.Background(), t.TempDir(), opts, true)
	if err != nil {
		t.Fatalf("want no error outside a working copy, got %v", err)
	}
	if len(changes) != 0 || skipped != 0 {
		t.Fatalf("want an empty result, got %+v / %d", changes, skipped)
	}
}
