//go:build http

package httpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The file list behind @-mentions shows the project, not the metadata a version
// control client keeps next to it: .svn holds a pristine copy of every file, so
// listing it would offer each file twice.
func TestWorkspaceFileListLeavesVersionControlMetadataOut(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range []string{
		"main.go",
		filepath.Join("sub", "b.go"),
		filepath.Join(".git", "HEAD"),
		filepath.Join(".svn", "wc.db"),
		filepath.Join(".svn", "pristine", "ab", "abcdef.svn-base"),
	} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	items, err := collectWorkspaceListedItems(root, true)
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, item := range items {
		listed = append(listed, item.PathRel)
		if strings.HasPrefix(item.PathRel, ".svn") || strings.HasPrefix(item.PathRel, ".git") {
			t.Errorf("version control metadata is listed: %s", item.PathRel)
		}
	}
	if got, want := strings.Join(listed, " "), "main.go sub/ sub/b.go"; got != want {
		t.Fatalf("listed %q, want %q", got, want)
	}
}
