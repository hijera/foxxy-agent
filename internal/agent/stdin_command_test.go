package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// Piped data cannot become an argument of an operator's slash command.
func TestBuiltinCommandsReadTypedTextOnly(t *testing.T) {
	cwd := t.TempDir()
	st := &session.State{ID: "sess_stdin_command", CWD: cwd, Mode: session.ModeAgent, SessionDir: t.TempDir()}
	ag := NewAgent(&config.Config{}, st, resumePermissionSender{}, nil)
	stop, err := ag.Run(context.Background(), []acp.ContentBlock{
		{Type: "text", Text: "/export md chat.md"},
		session.StdinAttachment("piped words that are no path\n"),
	})
	if err != nil || stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("Run: %v, stop %q", err, stop)
	}
	if _, err := os.Stat(filepath.Join(cwd, "chat.md")); err != nil {
		t.Fatalf("export did not use the typed target: %v", err)
	}
}
