package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
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

func TestLoadInstructionsDoesNotRepeatAFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("root agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "extra.md"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	inPrompt := filepath.Join(tmp, "AGENTS.md")
	tests := []struct {
		name          string
		files         []string
		alreadyLoaded []string
		want          string
	}{
		{name: "a file already in the prompt is skipped, the rest load", files: []string{"AGENTS.md", "extra.md"}, alreadyLoaded: []string{inPrompt}, want: "extra"},
		{name: "another spelling of a file already in the prompt", files: []string{"./AGENTS.md"}, alreadyLoaded: []string{inPrompt}, want: ""},
		{name: "a file named twice in the list loads once", files: []string{"extra.md", "./extra.md"}, want: "extra"},
		{name: "an already-loaded path that does not exist skips nothing", files: []string{"extra.md"}, alreadyLoaded: []string{filepath.Join(tmp, "gone.md")}, want: "extra"},
		{name: "nothing already loaded", files: []string{"AGENTS.md"}, want: "root agents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := session.LoadInstructions(tmp, tt.files, tt.alreadyLoaded...); got != tt.want {
				t.Fatalf("LoadInstructions(%q, already loaded %q) = %q, want %q", tt.files, tt.alreadyLoaded, got, tt.want)
			}
		})
	}
}

// The skip compares files on disk, not names: this repository's own CLAUDE.md
// is a symlink to AGENTS.md.
func TestLoadInstructionsSkipsLoadedFileUnderAnotherName(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("root agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	inPrompt := filepath.Join(tmp, "AGENTS.md")

	t.Run("hard link", func(t *testing.T) {
		if err := os.Link(inPrompt, filepath.Join(tmp, "AGENTS.hardlink.md")); err != nil {
			t.Skipf("cannot create a hard link here: %v", err)
		}
		if got := session.LoadInstructions(tmp, []string{"AGENTS.hardlink.md"}, inPrompt); got != "" {
			t.Fatalf("a hard link to a file already in the prompt loaded again: %q", got)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink("AGENTS.md", filepath.Join(tmp, "CLAUDE.md")); err != nil {
			t.Skipf("cannot create a symlink here: %v", err)
		}
		if got := session.LoadInstructions(tmp, []string{"CLAUDE.md"}, inPrompt); got != "" {
			t.Fatalf("a symlink to a file already in the prompt loaded again: %q", got)
		}
	})
	t.Run("differently cased name", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(tmp, "agents.md")); err != nil {
			t.Skip("case-sensitive volume: agents.md is a different file here")
		}
		if got := session.LoadInstructions(tmp, []string{"agents.md"}, inPrompt); got != "" {
			t.Fatalf("a differently cased name of a file already in the prompt loaded again: %q", got)
		}
	})
}
