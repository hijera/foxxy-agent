//go:build cli

package cli

import (
	"context"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// dropBackend answers the queue calls the drop path makes. The rest of the
// backend interface is never reached from them, so it stays nil.
type dropBackend struct {
	backend
	rows     []session.QueuedMessage
	taken    session.QueuedMessage
	queued   []acp.ImagePartRef
	queuedTo string
}

func (d *dropBackend) QueuedTurnMessages(string) ([]session.QueuedMessage, error) {
	return d.rows, nil
}

func (d *dropBackend) TakeBackQueuedTurnMessage(_, messageID string) (session.QueuedMessage, []session.QueuedMessage, error) {
	if d.taken.ID != messageID {
		return session.QueuedMessage{}, d.rows, session.ErrQueuedMessageNotFound
	}
	return d.taken, nil, nil
}

func (d *dropBackend) EnqueueFollowUpWithMode(_ context.Context, _, text, _ string, _ session.QueueMode, parts []acp.ImagePartRef) (session.QueuedMessage, bool, string, error) {
	d.queuedTo = text
	d.queued = parts
	return session.QueuedMessage{ID: "q_new", Text: text, ImageParts: parts}, true, "", nil
}

// While the first-use question is open, "1" and "2" answer it only on an empty
// input: a digit typed into the next message is part of that message.
func TestQueueChoiceKeysDoNotStealTheDraft(t *testing.T) {
	a := newTestApp(t)
	a.pendingQueueText = "queued follow-up"
	a.editor.SetText("line 1")

	if a.handleGlobalKey([]byte("1")) {
		t.Fatal("the queue-mode question took a digit typed into the draft")
	}
	if got := a.editor.Text(); got != "line 1" {
		t.Fatalf("the draft became %q, want the text typed into it", got)
	}
	if a.pendingQueueText == "" {
		t.Fatal("a digit taken for the draft must not answer the question")
	}

	// With the input empty the same key is the answer.
	a.mgr = &dropBackend{}
	a.editor.SetText("")
	if !a.handleGlobalKey([]byte("1")) {
		t.Fatal("the question went unanswered on an empty input")
	}
	if a.pendingQueueText != "" {
		t.Fatal("the question stayed open after its answer")
	}
	if a.queuePreference != session.QueueModeSteer {
		t.Fatalf("queue preference = %q, want steer from key 1", a.queuePreference)
	}
}

// Escape answers the question with the draft back, but nothing typed since may
// be lost: the restored message goes ahead of it, the way a refused queue
// returns a message.
func TestQueueChoiceEscapeKeepsWhatWasTypedSince(t *testing.T) {
	a := newTestApp(t)
	a.pendingQueueText = "queued follow-up"
	a.editor.SetText("typed since")

	if !a.handleGlobalKey([]byte("\x1b")) {
		t.Fatal("escape did not answer the question")
	}
	if got := a.editor.PendingText(); got != "queued follow-up\ntyped since" {
		t.Fatalf("the input holds %q, want the pending message ahead of what was typed since", got)
	}
	if a.queueChoicePending() {
		t.Fatal("the question stayed open after escape")
	}
}

// /queue drop takes a message back whole. Its images cannot sit in a terminal
// input, so they wait with the draft and ride out on the next message - never
// silently gone with the row.
func TestQueueDropRestoresImagesWithTheText(t *testing.T) {
	img := acp.ImagePartRef{Name: "shot.png", DataURL: "data:image/png;base64,AAAA"}
	mgr := &dropBackend{
		rows:  []session.QueuedMessage{{ID: "q_1", Text: "look"}},
		taken: session.QueuedMessage{ID: "q_1", Text: "look", ImageParts: []acp.ImagePartRef{img}},
	}
	res := runQueueCommand(mgr, "sess", "drop 1")
	if res.err != nil {
		t.Fatal(res.err)
	}
	if res.text != "look" || len(res.images) != 1 || res.images[0].DataURL != img.DataURL {
		t.Fatalf("drop answered %+v, want the text with its image", res)
	}

	a := newTestApp(t)
	a.mgr = mgr
	a.applyQueueResult(res)
	if got := a.editor.PendingText(); got != "look" {
		t.Fatalf("the input holds %q, want the taken-back text", got)
	}
	if len(a.draftImages) != 1 || a.draftImages[0].Name != "shot.png" {
		t.Fatalf("the draft carries %+v, want the taken-back image", a.draftImages)
	}

	a.enqueuePromptWithMode("look again", session.QueueModeSteer, false)
	if mgr.queuedTo != "look again" {
		t.Fatalf("queued %q, want the text of the next message", mgr.queuedTo)
	}
	if len(mgr.queued) != 1 || mgr.queued[0].DataURL != img.DataURL {
		t.Fatalf("the next message went out with %+v, want the restored image", mgr.queued)
	}
	if len(a.draftImages) != 0 {
		t.Fatalf("the draft kept %+v after the images rode out", a.draftImages)
	}
}
