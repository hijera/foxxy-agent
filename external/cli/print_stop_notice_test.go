//go:build cli

package cli

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// fork(stop-notice-transcript) guard: the agent streams the notice that says
// why a turn stopped short into the answer, so -p prints it once, on stdout
// with the answer. Upstream 1.2.9 also prints StopNotice to stderr; here that
// would say it twice.
func TestPrintPromptDoesNotRepeatStopNotice(t *testing.T) {
	const notice = "Stopped after 30 steps, the step limit set by agent.max_turns."
	runner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, sender acp.UpdateSender) (string, error) {
		_ = sender.SendSessionUpdate(st.GetID(), acp.MessageChunkUpdate{
			SessionUpdate: acp.UpdateTypeAgentMessageChunk,
			Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: "Partial work.\n\n" + notice},
		})
		st.SetTurnStopNotice(notice)
		return string(acp.StopReasonMaxTurns), nil
	}
	cwd := t.TempDir()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "p", Type: "openai", APIKey: "k"}},
		Models:    []config.ModelEntry{{Model: "p/m"}},
		Agent:     config.Agent{Model: "p/m"},
	}
	cfg.Paths.CWD = cwd
	mgr := session.NewManager(cfg, nil, runner, slog.Default(), cwd, &session.FileStore{Root: t.TempDir()})

	var out, errOut bytes.Buffer
	if err := PrintPrompt(context.Background(), mgr, PrintOptions{Prompt: "go", Out: &out, ErrOut: &errOut, Config: cfg}); err != nil {
		t.Fatalf("PrintPrompt: %v", err)
	}
	if got := strings.Count(out.String()+errOut.String(), notice); got != 1 {
		t.Fatalf("the notice was printed %d times (stdout %q, stderr %q), want once", got, out.String(), errOut.String())
	}
}
