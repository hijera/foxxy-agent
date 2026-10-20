package session_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// The notice the agent recorded for a turn that stopped short reaches the
// caller on the prompt result, once, and is not written to the UI log: the
// agent has already put it into the answer and the transcript
// (fork(stop-notice-transcript)).
func TestPromptResultCarriesTheStopNotice(t *testing.T) {
	calls := 0
	runner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		calls++
		if calls == 1 {
			st.SetTurnStopNotice("Stopped after 30 steps.")
			return string(acp.StopReasonMaxTurns), nil
		}
		return string(acp.StopReasonEndTurn), nil
	}
	cwd := t.TempDir()
	m := session.NewManager(testConfig(), noopSender{}, runner, slog.Default(), cwd, &session.FileStore{Root: t.TempDir()})
	ctx := context.Background()
	created, err := m.HandleSessionNew(ctx, acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	prompt := acp.SessionPromptParams{SessionID: created.SessionID, Prompt: []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "go"}}}

	res, err := m.HandleSessionPromptWithSender(ctx, prompt, noopSender{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopNotice != "Stopped after 30 steps." {
		t.Fatalf("StopNotice = %q", res.StopNotice)
	}
	st, err := m.EnsureHTTPSession(ctx, created.SessionID, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if log := st.GetUILog(); len(log) != 0 {
		t.Fatalf("the notice was written to the UI log as well: %+v", log)
	}

	res, err = m.HandleSessionPromptWithSender(ctx, prompt, noopSender{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopNotice != "" {
		t.Fatalf("the next turn carried the old notice: %q", res.StopNotice)
	}
}
