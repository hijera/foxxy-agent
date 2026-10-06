package svnws_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
)

const fakeRepoRoot = "https://svn.example.test/repo"

// newFake builds the fake client, registers a working copy at wc, and returns
// the handle plus the Options pointing at the fake binary.
func newFake(t *testing.T, wc string) (svntest.Fake, svnws.Options) {
	t.Helper()
	fake, err := svntest.Build(t.TempDir())
	if err != nil {
		t.Fatalf("build fake svn: %v", err)
	}
	fake.Setenv(t.Setenv)
	if err := os.MkdirAll(wc, 0o755); err != nil {
		t.Fatalf("mkdir wc: %v", err)
	}
	if err := fake.WriteState(svntest.NewState(fakeRepoRoot, wc)); err != nil {
		t.Fatalf("write fake state: %v", err)
	}
	return fake, svnws.Options{Binary: fake.Binary, TimeoutSeconds: 30}
}

func TestDescribeWorkingCopy(t *testing.T) {
	wc := filepath.Join(t.TempDir(), "branch-folder")
	_, opts := newFake(t, wc)

	info := svnws.Describe(context.Background(), wc, opts)
	if !info.Available {
		t.Fatalf("svn client should be available: %+v", info)
	}
	if !info.IsSVNRepo {
		t.Fatalf("working copy not detected: %+v", info)
	}
	if info.Branch != "trunk" {
		t.Errorf("branch = %q, want trunk", info.Branch)
	}
	if info.Revision != 12 {
		t.Errorf("revision = %d, want 12", info.Revision)
	}
	if info.URL != fakeRepoRoot+"/trunk" {
		t.Errorf("url = %q", info.URL)
	}
	if info.RepositoryRoot != fakeRepoRoot {
		t.Errorf("repository root = %q", info.RepositoryRoot)
	}
	if info.Nested {
		t.Errorf("working copy root should not be reported as nested")
	}
	if len(info.Branches) != 0 {
		t.Errorf("branch lookup is off; got %v", info.Branches)
	}
}

func TestDescribePlainFolderIsNotSVN(t *testing.T) {
	root := t.TempDir()
	wc := filepath.Join(root, "wc")
	_, opts := newFake(t, wc)

	plain := filepath.Join(root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	info := svnws.Describe(context.Background(), plain, opts)
	if info.IsSVNRepo {
		t.Fatalf("plain folder reported as a working copy: %+v", info)
	}
	if !info.Available {
		t.Errorf("client should still be reported available")
	}
}

// A git clone dropped inside an SVN branch folder leaves that subdirectory
// unversioned: svn info fails there, so Describe walks up to the working copy
// root and reports the enclosing branch as nested.
func TestDescribeNestedUnversionedSubdir(t *testing.T) {
	wc := filepath.Join(t.TempDir(), "branch-folder")
	fake, opts := newFake(t, wc)

	inner := filepath.Join(wc, "vendor-clone")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	state := svntest.NewState(fakeRepoRoot, wc)
	state.Unversioned = []string{inner}
	if err := fake.WriteState(state); err != nil {
		t.Fatal(err)
	}

	info := svnws.Describe(context.Background(), inner, opts)
	if !info.IsSVNRepo {
		t.Fatalf("enclosing working copy not found from %s: %+v", inner, info)
	}
	if !info.Nested {
		t.Errorf("expected Nested for an unversioned subdirectory")
	}
	if !samePath(info.WCRoot, wc) {
		t.Errorf("wc root = %q, want %q", info.WCRoot, wc)
	}
	if info.Branch != "trunk" {
		t.Errorf("branch = %q, want trunk", info.Branch)
	}
}

func TestDescribeBranchLookup(t *testing.T) {
	wc := filepath.Join(t.TempDir(), "wc")
	_, opts := newFake(t, wc)
	opts.BranchLookup = true

	info := svnws.Describe(context.Background(), wc, opts)
	want := []string{"trunk", "branches/feature-x", "branches/release-1"}
	if strings.Join(info.Branches, ",") != strings.Join(want, ",") {
		t.Fatalf("branches = %v, want %v", info.Branches, want)
	}
}

func TestDescribeWithoutClient(t *testing.T) {
	info := svnws.Describe(context.Background(), t.TempDir(),
		svnws.Options{Binary: filepath.Join(t.TempDir(), "definitely-missing-svn")})
	if info.Available || info.IsSVNRepo {
		t.Fatalf("missing client should yield an empty info: %+v", info)
	}
}

func TestBranchFromRelativeURL(t *testing.T) {
	cases := map[string]string{
		"^/trunk":                    "trunk",
		"^/trunk/cmd/foxxycode":      "trunk",
		"^/branches/feature-x":       "branches/feature-x",
		"^/branches/feature-x/inner": "branches/feature-x",
		"^/tags/v1.2":                "tags/v1.2",
		"^/":                         "",
		"^/sandbox/experiment":       "",
		"":                           "",
	}
	for rel, want := range cases {
		if got := svnws.BranchFromRelativeURL(rel); got != want {
			t.Errorf("BranchFromRelativeURL(%q) = %q, want %q", rel, got, want)
		}
	}
}

func TestBranchURL(t *testing.T) {
	info := svnws.Info{RepositoryRoot: fakeRepoRoot, Path: "/tmp/wc"}
	got, err := svnws.BranchURL(info, "branches/feature-x")
	if err != nil {
		t.Fatalf("BranchURL: %v", err)
	}
	if got != fakeRepoRoot+"/branches/feature-x" {
		t.Errorf("url = %q", got)
	}
	if _, err := svnws.BranchURL(info, "--upgrade"); err == nil {
		t.Errorf("option-like branch should be rejected")
	}
	if _, err := svnws.BranchURL(svnws.Info{}, "trunk"); err == nil {
		t.Errorf("missing repository root should fail")
	}
}

func TestBranchDirName(t *testing.T) {
	cases := map[string]string{
		"trunk":                "trunk",
		"branches/feature-x":   "branches-feature-x",
		" branches/a:b ":       "branches-a-b",
		"branches/release 1.0": "branches-release-1.0",
	}
	for in, want := range cases {
		if got := svnws.BranchDirName(in); got != want {
			t.Errorf("BranchDirName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOperationsRejectOptionLikeArguments(t *testing.T) {
	wc := filepath.Join(t.TempDir(), "wc")
	_, opts := newFake(t, wc)
	ctx := context.Background()

	cases := map[string]func() error{
		"status path": func() error {
			_, err := svnws.Status(ctx, wc, opts, []string{"--version"})
			return err
		},
		"commit path": func() error {
			_, err := svnws.Commit(ctx, wc, opts, "msg", []string{"-rHEAD"})
			return err
		},
		"switch url": func() error {
			_, err := svnws.Switch(ctx, wc, opts, "--config-dir=/tmp")
			return err
		},
		"merge source": func() error {
			_, err := svnws.Merge(ctx, wc, opts, "-x", "")
			return err
		},
		"checkout destination": func() error {
			_, err := svnws.Checkout(ctx, opts, fakeRepoRoot+"/trunk", "-out", "")
			return err
		},
		"diff revision": func() error {
			_, err := svnws.Diff(ctx, wc, opts, nil, "--username=root")
			return err
		},
		"resolve accept": func() error {
			_, err := svnws.Resolve(ctx, wc, opts, []string{"a.txt"}, "whatever")
			return err
		},
	}
	for name, run := range cases {
		if err := run(); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestCommitRequiresMessageAndPaths(t *testing.T) {
	wc := filepath.Join(t.TempDir(), "wc")
	_, opts := newFake(t, wc)
	ctx := context.Background()

	if _, err := svnws.Commit(ctx, wc, opts, "", []string{"a.txt"}); err == nil {
		t.Errorf("empty message should be rejected")
	}
	if _, err := svnws.Commit(ctx, wc, opts, "msg", nil); err == nil {
		t.Errorf("empty path list should be rejected")
	}
}

func TestOperationsBuildExpectedCommandLines(t *testing.T) {
	wc := filepath.Join(t.TempDir(), "wc")
	fake, opts := newFake(t, wc)
	ctx := context.Background()

	if _, err := svnws.Commit(ctx, wc, opts, "fix: thing", []string{"src/main.go"}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	call, ok := fake.FindCall("commit")
	if !ok {
		t.Fatalf("no commit recorded: %+v", mustCalls(t, fake))
	}
	joined := strings.Join(call.Args, " ")
	for _, want := range []string{"--non-interactive", "commit", "--message", "fix: thing", "--", "src/main.go"} {
		if !strings.Contains(joined, want) {
			t.Errorf("commit args %v missing %q", call.Args, want)
		}
	}

	if _, err := svnws.Update(ctx, wc, opts, nil, "HEAD"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if call, ok := fake.FindCall("update"); !ok {
		t.Errorf("no update recorded")
	} else if !strings.Contains(strings.Join(call.Args, " "), "--revision HEAD") {
		t.Errorf("update args = %v", call.Args)
	}
}

func TestStatusAndDiffRelayClientOutput(t *testing.T) {
	wc := filepath.Join(t.TempDir(), "wc")
	_, opts := newFake(t, wc)
	ctx := context.Background()

	status, err := svnws.Status(ctx, wc, opts, nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(status, "M       src/main.go") {
		t.Errorf("status output = %q", status)
	}
	diff, err := svnws.Diff(ctx, wc, opts, []string{"src/main.go"}, "")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(diff, "+// changed") {
		t.Errorf("diff output = %q", diff)
	}
}

func TestOperationsOutsideWorkingCopyFail(t *testing.T) {
	root := t.TempDir()
	wc := filepath.Join(root, "wc")
	_, opts := newFake(t, wc)

	plain := filepath.Join(root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svnws.Status(context.Background(), plain, opts, nil); err == nil {
		t.Fatalf("status outside a working copy should fail")
	}
}

// samePath compares two paths after resolving symlinks and Windows 8.3 short
// names, which temp directories on this platform expose.
func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(ra) == filepath.Clean(rb)
}

func mustCalls(t *testing.T, fake svntest.Fake) []svntest.Call {
	t.Helper()
	calls, err := fake.Calls()
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	return calls
}
