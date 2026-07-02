package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxy-agent/internal/session"
)

func TestLoadInstructionsSingleFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("# Hello\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := session.LoadInstructions(tmp, []string{"AGENTS.md"})
	if got != "# Hello\nworld" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestLoadInstructionsMultipleFiles(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.md"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "b.md"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := session.LoadInstructions(tmp, []string{"a.md", "b.md"})
	if got != "first\n\nsecond" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestLoadInstructionsMissingFilesSkipped(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "real.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := session.LoadInstructions(tmp, []string{"missing.md", "real.md", "also-missing.md"})
	if got != "content" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestLoadInstructionsEmptyFilesSkipped(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "empty.md"), []byte("   \n  "), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "content.md"), []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := session.LoadInstructions(tmp, []string{"empty.md", "content.md"})
	if got != "real" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestLoadInstructionsNoneExist(t *testing.T) {
	tmp := t.TempDir()
	got := session.LoadInstructions(tmp, []string{"AGENTS.md"})
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestLoadInstructionsNoFiles(t *testing.T) {
	got := session.LoadInstructions("/some/dir", []string{})
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestLoadInstructionsBlankNameSkipped(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "real.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := session.LoadInstructions(tmp, []string{"", "  ", "real.md"})
	if got != "hello" {
		t.Fatalf("unexpected content: %q", got)
	}
}
