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

func TestLoadVSCodeProjectContextLoadsWorkspaceFiles(t *testing.T) {
	tmp := t.TempDir()
	vscodeDir := filepath.Join(tmp, ".vscode")
	if err := os.Mkdir(vscodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "settings.json"), []byte(`{"go.lintTool": "golangci-lint"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "tasks.json"), []byte(`{"tasks": [{"label": "make test"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := session.LoadVSCodeProjectContext(tmp)
	for _, want := range []string{
		"VS Code project context",
		"<vscode_project_context>",
		`.vscode/settings.json`,
		`golangci-lint`,
		`.vscode/tasks.json`,
		`make test`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("context is missing %q:\n%s", want, got)
		}
	}
}

func TestLoadVSCodeProjectContextSkipsBinaryFiles(t *testing.T) {
	tmp := t.TempDir()
	vscodeDir := filepath.Join(tmp, ".vscode")
	if err := os.Mkdir(vscodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "extensions.json"), []byte(`{"recommendations": ["golang.go"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "cache.pch"), []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatal(err)
	}

	got := session.LoadVSCodeProjectContext(tmp)
	if !strings.Contains(got, `.vscode/extensions.json`) {
		t.Fatalf("text metadata is missing:\n%s", got)
	}
	if strings.Contains(got, `cache.pch`) {
		t.Fatalf("binary metadata must not enter the prompt:\n%s", got)
	}
}

func TestLoadVSCodeProjectContextWithoutDirectoryIsEmpty(t *testing.T) {
	if got := session.LoadVSCodeProjectContext(t.TempDir()); got != "" {
		t.Fatalf("context = %q, want empty", got)
	}
}

func TestLoadVSCodeProjectContextCapsTotalSizeAndReportsOmittedFiles(t *testing.T) {
	tmp := t.TempDir()
	vscodeDir := filepath.Join(tmp, ".vscode")
	if err := os.Mkdir(vscodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	large := []byte(strings.Repeat("x", 300*1024))
	for _, name := range []string{"a.log", "b.log", "c.log"} {
		if err := os.WriteFile(filepath.Join(vscodeDir, name), large, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := session.LoadVSCodeProjectContext(tmp)
	if len(got) > 600*1024 {
		t.Fatalf("context is unexpectedly large: %d bytes", len(got))
	}
	if !strings.Contains(got, `context_truncated omitted_files="1"`) {
		t.Fatalf("context must report files omitted by the total limit; length=%d", len(got))
	}
}

// A bulky extension cache sorts before settings.json alphabetically; the known
// configuration files must still reach the prompt.
func TestLoadVSCodeProjectContextReadsKnownFilesFirst(t *testing.T) {
	tmp := t.TempDir()
	vscodeDir := filepath.Join(tmp, ".vscode")
	if err := os.Mkdir(vscodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	large := []byte(strings.Repeat("x", 300*1024))
	for _, name := range []string{"aaa-cache.log", "bbb-cache.log"} {
		if err := os.WriteFile(filepath.Join(vscodeDir, name), large, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "settings.json"), []byte(`{"editor.tabSize": 2}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := session.LoadVSCodeProjectContext(tmp)
	if !strings.Contains(got, `editor.tabSize`) {
		t.Fatalf("settings.json must outrank bulky neighbours:\n%s", got[:min(len(got), 400)])
	}
}

// .vscode/settings.json idiomatically carries comments and trailing commas. The
// files are prompt text, never parsed, so such a file must survive verbatim.
func TestLoadVSCodeProjectContextKeepsJSONWithCommentsVerbatim(t *testing.T) {
	tmp := t.TempDir()
	vscodeDir := filepath.Join(tmp, ".vscode")
	if err := os.Mkdir(vscodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonc := "{\n  // the formatter of record\n  \"editor.defaultFormatter\": \"golang.go\",\n}\n"
	if err := os.WriteFile(filepath.Join(vscodeDir, "settings.json"), []byte(jsonc), 0o644); err != nil {
		t.Fatal(err)
	}

	got := session.LoadVSCodeProjectContext(tmp)
	for _, want := range []string{"// the formatter of record", `"golang.go",`} {
		if !strings.Contains(got, want) {
			t.Fatalf("context is missing %q:\n%s", want, got)
		}
	}
}
