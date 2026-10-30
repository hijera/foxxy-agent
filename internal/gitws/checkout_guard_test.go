package gitws

import (
	"os"
	"path/filepath"
	"testing"
)

// A branch name reaches `git checkout` from the request that switches a
// workspace. Git reads a name beginning with "-" as an option, and a name that
// is a file in the tree as a path to restore - discarding the uncommitted edits
// in it. Checkout must refuse both and leave the working copy as it was.
func TestCheckoutRefusesOptionLikeBranch(t *testing.T) {
	dir := initRepo(t)
	before := Describe(dir).Branch
	if err := Checkout(dir, "--orphan=escape"); err == nil {
		t.Fatal("an option-like name must be refused")
	}
	if got := Describe(dir).Branch; got != before {
		t.Fatalf("branch moved from %q to %q by a refused name", before, got)
	}
}

func TestCheckoutRefusesAFileNameAndKeepsItsEdits(t *testing.T) {
	dir := initRepo(t)
	file := filepath.Join(dir, "notes.txt")
	mustGit(t, dir, "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "--allow-empty", "-m", "second")
	if err := os.WriteFile(file, []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "notes.txt")
	mustGit(t, dir, "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "-m", "add notes")
	if err := os.WriteFile(file, []byte("unsaved edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Checkout(dir, "notes.txt"); err == nil {
		t.Fatal("a file name is not a branch and must be refused")
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unsaved edit\n" {
		t.Fatalf("the uncommitted edit was overwritten: %q", got)
	}
}

// A branch that only origin has is switched to as before: git creates the
// local branch from the remote-tracking one.
func TestCheckoutSwitchesToARemoteOnlyBranch(t *testing.T) {
	dir := initRepo(t)
	mustGit(t, dir, "push", "origin", "feature/login")
	mustGit(t, dir, "branch", "-D", "feature/login")
	if err := Checkout(dir, "feature/login"); err != nil {
		t.Fatalf("checkout of a branch only origin has: %v", err)
	}
	if got := Describe(dir).Branch; got != "feature/login" {
		t.Fatalf("branch after checkout = %q", got)
	}
}

func TestCheckoutRefusesAnUnknownBranchWithoutCreatingIt(t *testing.T) {
	dir := initRepo(t)
	if err := Checkout(dir, "no-such-branch"); err == nil {
		t.Fatal("expected error for unknown branch")
	}
	if refExists(dir, "refs/heads/no-such-branch") {
		t.Fatal("a refused checkout created the branch")
	}
}
