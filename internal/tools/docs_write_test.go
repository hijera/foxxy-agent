package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tools"
)

func TestDocsWriteAllowsMarkdownInDocsAndRoot(t *testing.T) {
	cwd := t.TempDir()
	env := &tools.Env{CWD: cwd}
	tool := tools.DocsWriteTool()

	for _, rel := range []string{"docs/foo.md", "docs/release..notes.md", "README.md", "AGENTS.md", "DESIGN.md"} {
		t.Run(rel, func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"path": rel, "content": "# Title\n"})
			out, err := tool.Execute(context.Background(), string(args), env)
			if err != nil {
				t.Fatalf("Execute(%q): %v", rel, err)
			}
			if !strings.Contains(out, "wrote") {
				t.Fatalf("unexpected output: %q", out)
			}
			abs := filepath.Join(cwd, rel)
			data, err := os.ReadFile(abs)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "# Title\n" {
				t.Fatalf("content mismatch: %q", data)
			}
		})
	}
}

func TestDocsWriteRejectsNonMarkdownAndProtectedPaths(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "internal", "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := &tools.Env{CWD: cwd}
	tool := tools.DocsWriteTool()

	for _, tc := range []struct {
		path string
	}{
		{path: "main.go"},
		{path: "../outside.md"},
		{path: "internal/prompts/agent.md"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"path": tc.path, "content": "x"})
			_, err := tool.Execute(context.Background(), string(args), env)
			if err == nil {
				t.Fatalf("expected error for path %q", tc.path)
			}
		})
	}
}

func TestDocsWriteRequiresExplicitOverwrite(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "README.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := &tools.Env{CWD: cwd}
	tool := tools.DocsWriteTool()

	args, _ := json.Marshal(map[string]interface{}{
		"path":    "README.md",
		"content": "new",
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err == nil {
		t.Fatal("expected overwrite without explicit opt-in to fail")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("failed overwrite changed file to %q", data)
	}

	args, _ = json.Marshal(map[string]interface{}{
		"path":      "README.md",
		"content":   "new",
		"overwrite": true,
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err != nil {
		t.Fatalf("explicit overwrite: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("explicit overwrite wrote %q", data)
	}
}

func TestDocsWriteRejectsSymlinkEscape(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(cwd, "linked-docs")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	env := &tools.Env{CWD: cwd}
	tool := tools.DocsWriteTool()
	args, _ := json.Marshal(map[string]interface{}{
		"path":    "linked-docs/escape.md",
		"content": "outside",
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err == nil {
		t.Fatal("expected symlink escape to fail")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.md")); !os.IsNotExist(err) {
		t.Fatalf("outside file was created or stat failed unexpectedly: %v", err)
	}
}

func TestDocsWriteRejectsDanglingSymlink(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "not-created.md")
	link := filepath.Join(cwd, "dangling.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	env := &tools.Env{CWD: cwd}
	tool := tools.DocsWriteTool()
	args, _ := json.Marshal(map[string]interface{}{
		"path":      "dangling.md",
		"content":   "outside",
		"overwrite": true,
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err == nil {
		t.Fatal("expected dangling symlink to fail")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("outside target was created or stat failed unexpectedly: %v", err)
	}
}
