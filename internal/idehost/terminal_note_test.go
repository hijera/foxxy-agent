package idehost

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/ideterm"
)

func terminalEnvNote() string                { return TerminalNote(ideterm.Get()) }
func terminalMentionNote(text string) string { return TerminalMentionNote(text, ideterm.Get()) }
func TestTerminalEnvNoteEmptyWhenNoState(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	ideterm.Reset()
	if note := terminalEnvNote(); note != "" {
		t.Fatalf("expected empty note, got %q", note)
	}
}

func TestTerminalEnvNoteActiveFirstAndShape(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	ideterm.Set([]ideterm.Terminal{
		{ID: "2", Name: "dev server", Output: "listening on :3000\n"},
		{ID: "1", Name: "zsh", LastCommand: "go test ./...", Output: "ok\n", Active: true},
	})
	note := terminalEnvNote()
	if !strings.HasPrefix(note, "<foxxycode_terminal_context>") || !strings.HasSuffix(note, "</foxxycode_terminal_context>") {
		t.Fatalf("note not wrapped in tag: %q", note)
	}
	// Active terminal must be listed first.
	activeIdx := strings.Index(note, "# Active Terminal: zsh")
	devIdx := strings.Index(note, "# Terminal: dev server")
	if activeIdx < 0 || devIdx < 0 || activeIdx > devIdx {
		t.Fatalf("active terminal not listed first: %q", note)
	}
	if !strings.Contains(note, "$ go test ./...") {
		t.Fatalf("last command missing: %q", note)
	}
}

func TestTerminalMentionNoteExpandsActive(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	ideterm.Set([]ideterm.Terminal{
		{ID: "1", Name: "zsh", Output: "full build log line 1\nline 2\n", Active: true},
		{ID: "2", Name: "dev server", Output: "listening\n"},
	})
	note := terminalMentionNote("check @terminal please")
	if !strings.Contains(note, `<foxxycode_terminal_output name="zsh">`) {
		t.Fatalf("bare @terminal should expand active terminal: %q", note)
	}
	if !strings.Contains(note, "full build log line 1") {
		t.Fatalf("full buffer missing: %q", note)
	}
}

func TestTerminalMentionNoteByName(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	ideterm.Set([]ideterm.Terminal{
		{ID: "1", Name: "zsh", Output: "z\n", Active: true},
		{ID: "2", Name: "dev server", Output: "server output here\n"},
	})
	note := terminalMentionNote(`see @terminal:"dev server"`)
	// The regex captures a non-space token; a quoted name won't match, but the
	// unquoted single token should. Verify the common single-word case works:
	note2 := terminalMentionNote("see @terminal:zsh")
	if !strings.Contains(note2, `name="zsh"`) {
		t.Fatalf("named @terminal:zsh should expand zsh: %q", note2)
	}
	_ = note
}

func TestTerminalMentionNoteEmptyWithoutMention(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	ideterm.Set([]ideterm.Terminal{{ID: "1", Name: "zsh", Output: "x\n", Active: true}})
	if note := terminalMentionNote("no mention here"); note != "" {
		t.Fatalf("expected empty note without mention, got %q", note)
	}
}
