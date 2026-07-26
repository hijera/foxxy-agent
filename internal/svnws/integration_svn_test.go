package svnws_test

// Integration coverage against a real Subversion client. Everything here is
// skipped when svn/svnadmin are not installed; with a client present it proves
// interop for the whole branch-folder workflow, including a working copy that
// also holds a git repository.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/gitws"
	"github.com/hijera/foxxycode-agent/internal/svnws"
)

// realSVN skips the test unless both svn and svnadmin are on PATH.
func realSVN(t *testing.T) svnws.Options {
	t.Helper()
	for _, bin := range []string{"svn", "svnadmin"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed; skipping real Subversion integration test", bin)
		}
	}
	return svnws.Options{TimeoutSeconds: 120, BranchLookup: true}
}

func mustRun(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// fileURL renders a filesystem path as a file:// URL svn accepts on every platform.
func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed // Windows drive letters
	}
	return "file://" + slashed
}

// newRepo creates a repository with the standard trunk/branches/tags layout and
// returns its URL.
func newRepo(t *testing.T, root string) string {
	t.Helper()
	repo := filepath.Join(root, "repo")
	mustRun(t, root, "svnadmin", "create", repo)
	url := fileURL(repo)
	mustRun(t, root, "svn", "--non-interactive", "mkdir", "--parents", "-m", "layout",
		url+"/trunk", url+"/branches", url+"/tags")
	return url
}

func TestRealSVNBranchFolderWorkflow(t *testing.T) {
	opts := realSVN(t)
	ctx := context.Background()
	root := t.TempDir()
	url := newRepo(t, root)

	wc := filepath.Join(root, "trunk-folder")
	if _, err := svnws.Checkout(ctx, opts, url+"/trunk", wc, ""); err != nil {
		t.Fatalf("checkout trunk: %v", err)
	}

	info := svnws.Describe(ctx, wc, opts)
	if !info.IsSVNRepo || info.Branch != "trunk" {
		t.Fatalf("describe trunk checkout: %+v", info)
	}
	if info.RepositoryRoot == "" || info.WCRoot == "" {
		t.Errorf("describe missed repository/wc roots: %+v", info)
	}

	// Add and commit a file through the package API.
	if err := os.WriteFile(filepath.Join(wc, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svnws.Add(ctx, wc, opts, []string{"main.go"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	out, err := svnws.Commit(ctx, wc, opts, "add main", []string{"main.go"})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !strings.Contains(out, "Committed revision") {
		t.Errorf("commit output = %q", out)
	}

	// Branch with svn copy, then list it through Describe.
	mustRun(t, wc, "svn", "--non-interactive", "copy", "-m", "branch",
		url+"/trunk", url+"/branches/feature-x")
	info = svnws.Describe(ctx, wc, opts)
	found := false
	for _, b := range info.Branches {
		if b == "branches/feature-x" {
			found = true
		}
	}
	if !found {
		t.Errorf("branch lookup did not list the new branch: %v", info.Branches)
	}

	// Switch the working copy in place.
	branchURL, err := svnws.BranchURL(info, "branches/feature-x")
	if err != nil {
		t.Fatalf("branch url: %v", err)
	}
	if _, err := svnws.Switch(ctx, wc, opts, branchURL); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if got := svnws.Describe(ctx, wc, opts); got.Branch != "branches/feature-x" {
		t.Fatalf("after switch: %+v", got)
	}

	// Commit on the branch, switch back, and merge it into trunk.
	if err := os.WriteFile(filepath.Join(wc, "main.go"), []byte("package main // branch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svnws.Commit(ctx, wc, opts, "branch change", []string{"main.go"}); err != nil {
		t.Fatalf("commit on branch: %v", err)
	}
	if _, err := svnws.Switch(ctx, wc, opts, url+"/trunk"); err != nil {
		t.Fatalf("switch back: %v", err)
	}
	mergeOut, err := svnws.Merge(ctx, wc, opts, branchURL, "")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(mergeOut, "Merging") && !strings.Contains(mergeOut, "merge") {
		t.Errorf("merge output = %q", mergeOut)
	}
	status, err := svnws.Status(ctx, wc, opts, nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(status, "main.go") {
		t.Errorf("merged change missing from status: %q", status)
	}

	// A second branch folder side by side - the workflow the SVN chip drives.
	second := filepath.Join(root, "feature-folder")
	if _, err := svnws.Checkout(ctx, opts, branchURL, second, ""); err != nil {
		t.Fatalf("checkout branch folder: %v", err)
	}
	if got := svnws.Describe(ctx, second, opts); got.Branch != "branches/feature-x" {
		t.Fatalf("branch folder: %+v", got)
	}
}

// A git repository inside an svn branch folder must not disturb either side.
func TestRealSVNMixedWithGitRepository(t *testing.T) {
	opts := realSVN(t)
	if !gitws.GitAvailable() {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	root := t.TempDir()
	url := newRepo(t, root)

	wc := filepath.Join(root, "branch-folder")
	if _, err := svnws.Checkout(ctx, opts, url+"/trunk", wc, ""); err != nil {
		t.Fatalf("checkout: %v", err)
	}

	// git init directly in the working copy: both VCS describe the same folder.
	mustRun(t, wc, "git", "init", "-b", "main")
	mustRun(t, wc, "git", "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "--allow-empty", "-m", "init")

	gitInfo := gitws.Describe(wc)
	if !gitInfo.IsGitRepo || gitInfo.Branch != "main" {
		t.Fatalf("git describe: %+v", gitInfo)
	}
	svnInfo := svnws.Describe(ctx, wc, opts)
	if !svnInfo.IsSVNRepo || svnInfo.Branch != "trunk" {
		t.Fatalf("svn describe: %+v", svnInfo)
	}
	if svnInfo.Nested {
		t.Errorf("the working copy root itself must not be reported as nested")
	}

	// Committing explicit paths never sweeps the unversioned .git directory in.
	if err := os.WriteFile(filepath.Join(wc, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svnws.Add(ctx, wc, opts, []string{"main.go"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	out, err := svnws.Commit(ctx, wc, opts, "add main", []string{"main.go"})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if strings.Contains(out, ".git") {
		t.Errorf("commit touched the git repository: %q", out)
	}
	if _, err := os.Stat(filepath.Join(wc, ".git")); err != nil {
		t.Errorf("git repository disappeared: %v", err)
	}
	// git is still on its own branch and sees the svn metadata as untracked only.
	if got := gitws.Describe(wc); got.Branch != "main" {
		t.Errorf("git branch changed to %q", got.Branch)
	}
}

// A git clone in an unversioned subfolder of a working copy still resolves the
// enclosing svn branch, flagged as nested.
func TestRealSVNNestedUnversionedGitClone(t *testing.T) {
	opts := realSVN(t)
	if !gitws.GitAvailable() {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	root := t.TempDir()
	url := newRepo(t, root)

	wc := filepath.Join(root, "branch-folder")
	if _, err := svnws.Checkout(ctx, opts, url+"/trunk", wc, ""); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	inner := filepath.Join(wc, "vendor-clone")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, inner, "git", "init", "-b", "main")
	mustRun(t, inner, "git", "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "--allow-empty", "-m", "init")

	info := svnws.Describe(ctx, inner, opts)
	if !info.IsSVNRepo {
		t.Fatalf("enclosing working copy not found from %s: %+v", inner, info)
	}
	if !info.Nested {
		t.Errorf("expected Nested for an unversioned subdirectory: %+v", info)
	}
	if info.Branch != "trunk" {
		t.Errorf("branch = %q, want trunk", info.Branch)
	}
	if gitInfo := gitws.Describe(inner); !gitInfo.IsGitRepo || gitInfo.Branch != "main" {
		t.Errorf("git describe inside the subfolder: %+v", gitInfo)
	}
}
