package session_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// A follow-up the turn boundary answers with a run of its own enters the
// conversation the way one read between two steps does: the clients watching the
// turn are shown it where it was read, and the transcript records it as a queued
// follow-up rather than as the prompt of a new turn. A client re-attaching to the
// turn tells the two apart by that marker.
func TestTurnBoundaryRecordsAndAnnouncesTheFollowUpItAnswers(t *testing.T) {
	var mgr *session.Manager
	var sid string
	runs := 0
	runner := func(_ context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		var text strings.Builder
		for _, b := range prompt {
			text.WriteString(b.Text)
		}
		// What Agent.Run does with its prompt.
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: text.String()})
		if runs == 0 {
			if _, _, err := mgr.EnqueueTurnMessage(sid, "one more thing"); err != nil {
				t.Errorf("enqueue during the turn: %v", err)
			}
		}
		runs++
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "answer"})
		return string(acp.StopReasonEndTurn), nil
	}
	dir := t.TempDir()
	mgr = session.NewManager(testConfig(), noopSender{}, runner, slog.Default(), dir, nil)
	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}
	sid = res.SessionID

	watch := &captureSender{}
	if _, err := mgr.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
		SessionID: sid,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "start the work"}},
	}, watch, nil); err != nil {
		t.Fatal(err)
	}

	var users []llm.Message
	for _, m := range mgr.SessionByID(sid).GetMessages() {
		if m.Role == llm.RoleUser {
			users = append(users, m)
		}
	}
	if len(users) != 2 {
		t.Fatalf("recorded %d user messages, want the prompt and the follow-up: %+v", len(users), users)
	}
	if users[0].Queued {
		t.Error("the prompt the turn started from is marked as a queued follow-up")
	}
	if !users[1].Queued || users[1].Content != "one more thing" {
		t.Errorf("the follow-up is recorded as %+v, want it marked queued", users[1])
	}

	announced := false
	watch.mu.Lock()
	for _, u := range watch.ups {
		if c, ok := u.(acp.MessageChunkUpdate); ok && c.SessionUpdate == acp.UpdateTypeUserMessageChunk && c.Content.Text == "one more thing" {
			announced = true
		}
	}
	watch.mu.Unlock()
	if !announced {
		t.Error("the clients watching the turn were not shown the follow-up it answered")
	}

	// The marker belongs to that one run: the next turn's prompt is a prompt.
	if _, err := mgr.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
		SessionID: sid,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "a new turn"}},
	}, watch, nil); err != nil {
		t.Fatal(err)
	}
	msgs := mgr.SessionByID(sid).GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleUser {
			if msgs[i].Queued {
				t.Errorf("the next turn's prompt %q is marked queued", msgs[i].Content)
			}
			break
		}
	}
}

// A run that records no message at all - a prompt a hook refused - must not
// leave the marker behind for whatever user message comes next.
func TestTheQueuedMarkerDoesNotOutliveItsRun(t *testing.T) {
	st := &session.State{ID: "sess_marker", Mode: session.ModeAgent}
	st.MarkNextPromptQueued()
	st.ClearNextPromptQueued()
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "a prompt"})
	if st.GetMessages()[0].Queued {
		t.Fatal("a withdrawn marker still landed on the next user message")
	}
	st.MarkNextPromptQueued()
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "not a user message"})
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "the follow-up"})
	if msgs := st.GetMessages(); msgs[1].Queued || !msgs[2].Queued {
		t.Fatalf("the marker must skip other roles and land on the next user message: %+v", msgs)
	}
}
