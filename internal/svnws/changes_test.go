package svnws_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/workcopy"
)

func TestRealSVNReviewUsesBaseAndKeepsDiscardScoped(t *testing.T) {
	opts := realSVN(t)
	ctx := context.Background()
	root := t.TempDir()
	wc := filepath.Join(root, "wc")
	mustRun(t, root, "svn", "checkout", newRepo(t, root)+"/trunk", wc)
	svn := func(args ...string) {
		t.Helper()
		mustRun(t, wc, "svn", args...)
	}
	write := func(name string, content []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(wc, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(name string, content []byte) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(wc, name))
		if err != nil || !bytes.Equal(got, content) {
			t.Fatalf("%s = %q, %v; want %q", name, got, err, content)
		}
	}

	original := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2, '\r', '\n', '\r', '\n'}
	write("edited @ draft.txt", original)
	write("missing.txt", []byte("restore me\n"))
	write("deleted.txt", []byte("restore me too\n"))
	write("replaced.txt", []byte("old\n"))
	svn("add", ".", "--force", "--no-auto-props")
	svn("propset", "svn:ignore", "ignored.txt", ".")
	svn("commit", "-m", "base")

	write("edited @ draft.txt", []byte("edited\n"))
	svn("changelist", "review", "--", "edited @ draft.txt@")
	if err := os.Remove(filepath.Join(wc, "missing.txt")); err != nil {
		t.Fatal(err)
	}
	svn("delete", "deleted.txt", "replaced.txt")
	write("replaced.txt", []byte("replacement\n"))
	svn("add", "replaced.txt")
	write("new.txt", []byte("new\n"))
	write("scheduled.txt", []byte("new and scheduled\n"))
	svn("add", "scheduled.txt")
	write("ignored.txt", []byte("keep ignored\n"))
	if err := os.Mkdir(filepath.Join(wc, "untracked-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join("untracked-dir", "child.txt"), []byte("keep unclassified\n"))
	write("big.txt", make([]byte, workcopy.MaxNewBytes+1))

	changes, skipped, err := svnws.WorktreeChanges(ctx, opts, wc)
	if err != nil || skipped != 1 || len(changes) != 5 {
		t.Fatalf("changes = %v, skipped = %d, err = %v", changes, skipped, err)
	}
	edited, err := svnws.WorktreeChangeFor(ctx, opts, wc, "edited @ draft.txt")
	if err != nil || edited == nil || edited.Status != "modified" ||
		!bytes.Equal(edited.Before, original) || string(edited.After) != "edited\n" {
		t.Fatalf("edited = %+v, err = %v", edited, err)
	}
	for _, name := range []string{"missing.txt", "deleted.txt"} {
		change, err := svnws.WorktreeChangeFor(ctx, opts, wc, name)
		if err != nil || change == nil || change.Status != "deleted" || change.After != nil {
			t.Fatalf("%s = %+v, err = %v", name, change, err)
		}
	}
	for _, name := range []string{"ignored.txt", "../repo/format", "replaced.txt", "big.txt"} {
		if got, err := svnws.WorktreeChangeFor(ctx, opts, wc, name); err != nil || got != nil {
			t.Fatalf("%s escaped review filtering: %+v, err = %v", name, got, err)
		}
	}
	if err := svnws.Discard(ctx, opts, wc, []string{"new.txt", "ignored.txt"}); !errors.Is(err, workcopy.ErrNotChanged) {
		t.Fatalf("discard of ignored file = %v", err)
	}
	check("new.txt", []byte("new\n"))
	if err := svnws.Discard(ctx, opts, wc, []string{"edited @ draft.txt"}); err != nil {
		t.Fatal(err)
	}
	check("edited @ draft.txt", original)
	check("new.txt", []byte("new\n"))
	if err := svnws.Discard(ctx, opts, wc, nil); err != nil {
		t.Fatal(err)
	}
	check("deleted.txt", []byte("restore me too\n"))
	check("missing.txt", []byte("restore me\n"))
	check("ignored.txt", []byte("keep ignored\n"))
	check("replaced.txt", []byte("replacement\n"))
	check(filepath.Join("untracked-dir", "child.txt"), []byte("keep unclassified\n"))
	for _, name := range []string{"new.txt", "scheduled.txt", "big.txt"} {
		if _, err := os.Stat(filepath.Join(wc, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was not removed: %v", name, err)
		}
	}
}
