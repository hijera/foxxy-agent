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

	for _, rel := range []string{"docs/foo.md", "README.md", "AGENTS.md", "DESIGN.md"} {
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
