package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// A Subversion working copy keeps its metadata, pristine copies of every file
// included, in .svn at the root. Like .git, it belongs to the client: an svn
// command the agent ran during the turn rewrites it, and a rollback of the
// turn must not try to undo that.
func TestWorkspaceSnapshotLeavesSubversionMetadataOut(t *testing.T) {
	cwd := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(cwd, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n")
	write(filepath.Join(".svn", "wc.db"), "revision 12\n")

	before := session.TakeWorkspaceSnapshot(cwd)
	write(filepath.Join(".svn", "wc.db"), "revision 13\n")
	write(filepath.Join(".svn", "pristine", "ab", "abcdef.svn-base"), "package main\n")

	diff, err := session.ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && len(diff.Changes) > 0 {
		t.Fatalf("changes inside .svn were recorded: %+v", diff.Changes)
	}
}
