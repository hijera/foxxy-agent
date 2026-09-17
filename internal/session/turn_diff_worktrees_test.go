package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// A turn in the main checkout snapshots the files it may roll back. The git
// worktrees FoxxyCode keeps under .foxxycode/worktrees are whole checkouts of other
// branches: an edit there is not this turn's to record, while the rest of
// .foxxycode (rules, MCP and hook files the agent may edit) still is.
func TestWorkspaceSnapshotLeavesTheWorktreesFolderOut(t *testing.T) {
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
	write(filepath.Join(".foxxycode", "worktrees", "feature-login", "main.go"), "package main\n")
	write(filepath.Join(".foxxycode", "rules", "style.md"), "be brief\n")

	before := session.TakeWorkspaceSnapshot(cwd)
	write(filepath.Join(".foxxycode", "worktrees", "feature-login", "main.go"), "package main // edited in the worktree\n")
	write(filepath.Join(".foxxycode", "worktrees", "feature-login", "new.go"), "package main\n")

	diff, err := session.ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && len(diff.Changes) > 0 {
		t.Fatalf("edits inside .foxxycode/worktrees were recorded: %+v", diff.Changes)
	}

	write(filepath.Join(".foxxycode", "rules", "style.md"), "be briefer\n")
	diff, err = session.ComputeWorkspaceDiff(cwd, before)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil || len(diff.Changes) != 1 || diff.Changes[0].Path != filepath.Join(".foxxycode", "rules", "style.md") {
		t.Fatalf("an edit to a rule file was not recorded: %+v", diff)
	}
}
