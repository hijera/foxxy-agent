package autocomplete_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/autocomplete"
)

func TestRelatedFilesRespectExplicitWorkspaceAndInputList(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "main.go")
	neighbor := filepath.Join(root, "helper.go")
	outside := filepath.Join(filepath.Dir(root), "outside.go")
	for _, path := range []string{current, neighbor, outside} {
		if err := os.WriteFile(path, []byte("package example\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := autocomplete.RelatedFiles(root, current, 3, []string{outside, current, neighbor})
	if len(files) != 1 || files[0].Path != "helper.go" || files[0].Content != "package example\n" {
		t.Fatalf("unconfined or missing context: %+v", files)
	}
	if files := autocomplete.RelatedFiles(root, current, 3, nil); len(files) != 0 {
		t.Fatalf("context must come from the supplied host list: %+v", files)
	}
}
