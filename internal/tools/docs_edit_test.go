package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tools"
)

func TestDocsEditUpdatesMarkdownFile(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &tools.Env{CWD: cwd}
	tool := tools.DocsEditTool()
	args, _ := json.Marshal(map[string]string{
		"path":      "docs/guide.md",
		"oldString": "world",
		"newString": "docs",
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello docs" {
		t.Fatalf("got %q", data)
	}
}

func TestDocsEditRejectsNonMarkdownPath(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "main.go")
	if err := os.WriteFile(path, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := &tools.Env{CWD: cwd}
	tool := tools.DocsEditTool()
	args, _ := json.Marshal(map[string]string{
		"path":      "main.go",
		"oldString": "package",
		"newString": "mod",
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err == nil {
		t.Fatal("expected error editing non-markdown path")
	}
}

func TestDocsEditRejectsEmptyOldString(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "README.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := &tools.Env{CWD: cwd}
	tool := tools.DocsEditTool()
	args, _ := json.Marshal(map[string]string{
		"path":      "README.md",
		"oldString": "",
		"newString": "replacement",
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err == nil {
		t.Fatal("expected empty oldString to fail")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("failed edit changed file to %q", data)
	}
}

func TestDocsEditRejectsAmbiguousOldString(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "README.md")
	if err := os.WriteFile(path, []byte("same same"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := &tools.Env{CWD: cwd}
	tool := tools.DocsEditTool()
	args, _ := json.Marshal(map[string]string{
		"path":      "README.md",
		"oldString": "same",
		"newString": "changed",
	})
	if _, err := tool.Execute(context.Background(), string(args), env); err == nil {
		t.Fatal("expected ambiguous oldString to fail")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "same same" {
		t.Fatalf("failed edit changed file to %q", data)
	}
}
