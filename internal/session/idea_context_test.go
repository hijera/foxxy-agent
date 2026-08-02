package session_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestLoadIntelliJProjectContextRecursivelyLoadsUTF8Files(t *testing.T) {
	tmp := t.TempDir()
	ideaDir := filepath.Join(tmp, ".idea")
	modulesDir := filepath.Join(ideaDir, "modules")
	if err := os.MkdirAll(modulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideaDir, "externalDependencies.xml"), []byte(`<plugin id="org.jetbrains.plugins.go" />`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modulesDir, "backend.iml"), []byte(`<module type="GO_MODULE" />`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := session.LoadIntelliJProjectContext(tmp)
	for _, want := range []string{
		"IntelliJ IDEA project context",
		`.idea/externalDependencies.xml`,
		`org.jetbrains.plugins.go`,
		`.idea/modules/backend.iml`,
		`GO_MODULE`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("context is missing %q:\n%s", want, got)
		}
	}
}

func TestLoadIntelliJProjectContextSkipsBinaryFiles(t *testing.T) {
	tmp := t.TempDir()
	ideaDir := filepath.Join(tmp, ".idea")
	if err := os.Mkdir(ideaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideaDir, "misc.xml"), []byte(`<project version="4" />`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideaDir, "binary.dat"), []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatal(err)
	}

	got := session.LoadIntelliJProjectContext(tmp)
	if !strings.Contains(got, `.idea/misc.xml`) {
		t.Fatalf("text metadata is missing:\n%s", got)
	}
	if strings.Contains(got, `binary.dat`) {
		t.Fatalf("binary metadata must not enter the prompt:\n%s", got)
	}
}

func TestLoadIntelliJProjectContextWithoutIdeaDirectoryIsEmpty(t *testing.T) {
	if got := session.LoadIntelliJProjectContext(t.TempDir()); got != "" {
		t.Fatalf("context = %q, want empty", got)
	}
}

func TestLoadIntelliJProjectContextCapsTotalSizeAndReportsOmittedFiles(t *testing.T) {
	tmp := t.TempDir()
	ideaDir := filepath.Join(tmp, ".idea")
	if err := os.Mkdir(ideaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	large := []byte(strings.Repeat("x", 300*1024))
	for _, name := range []string{"a.xml", "b.xml", "c.xml"} {
		if err := os.WriteFile(filepath.Join(ideaDir, name), large, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := session.LoadIntelliJProjectContext(tmp)
	if len(got) > 600*1024 {
		t.Fatalf("context is unexpectedly large: %d bytes", len(got))
	}
	if !strings.Contains(got, `context_truncated omitted_files="1"`) {
		t.Fatalf("context must report files omitted by the total limit; length=%d", len(got))
	}
}
