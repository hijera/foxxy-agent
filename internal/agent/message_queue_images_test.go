package agent

// The mid-turn read of the queue meets the multimodal gate: a follow-up queued
// while the model took images is folded into the conversation without them -
// and names what it lost - once the session's model is one that takes none.
// The enqueue gate cannot cover this: the model can be switched between the
// moment a message is written and the moment the turn reads it.

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// readQueuedHarness builds an agent whose only life is readQueuedMessages, on a
// session whose model takes images or not as the test says.
func readQueuedHarness(t *testing.T, multimodal bool) (*Agent, *session.State) {
	t.Helper()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100, Multimodal: multimodal}},
		Agent:     config.Agent{Model: "fake/model"},
	}
	st := &session.State{ID: "sess_readq", Mode: session.ModeAgent}
	st.OpenMessageQueue()
	t.Cleanup(func() { st.CloseMessageQueue() })
	return NewAgent(cfg, st, &stallSender{}, nil), st
}

const queuedImageURL = "data:image/png;base64,YQ=="

func readOneQueuedMessage(t *testing.T, a *Agent, st *session.State) llm.Message {
	t.Helper()
	if _, err := st.EnqueueMessageWithMode("inspect this", session.QueueModeSteer,
		[]acp.ImagePartRef{{Name: "shot.png", DataURL: queuedImageURL}}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	var msgs []llm.Message
	if !a.readQueuedMessages(&msgs) {
		t.Fatal("the read found nothing queued")
	}
	if len(msgs) != 1 {
		t.Fatalf("read %d messages, want 1", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "inspect this") {
		t.Fatalf("the message lost its text: %q", msgs[0].Content)
	}
	if !msgs[0].Queued {
		t.Fatalf("the message is not marked queued: %+v", msgs[0])
	}
	return msgs[0]
}

// A model that takes no images gets the follow-up's text with the attachment
// named as lost - the read-side twin of the enqueue gate - and the turn goes on.
func TestReadQueuedMessagesDropsImagesForATextOnlyModel(t *testing.T) {
	a, st := readQueuedHarness(t, false)
	msg := readOneQueuedMessage(t, a, st)
	if len(msg.ImageParts) != 0 {
		t.Fatalf("a text-only model was handed %d image parts: %+v", len(msg.ImageParts), msg.ImageParts)
	}
	if !strings.Contains(msg.Content, "[image not shown] shot.png") {
		t.Fatalf("the message does not name the image it lost: %q", msg.Content)
	}
}

// A model that takes images is handed them exactly as they were queued, with no
// note about any drop.
func TestReadQueuedMessagesKeepsImagesForAMultimodalModel(t *testing.T) {
	a, st := readQueuedHarness(t, true)
	msg := readOneQueuedMessage(t, a, st)
	if len(msg.ImageParts) != 1 || msg.ImageParts[0].Name != "shot.png" || msg.ImageParts[0].DataURL != queuedImageURL {
		t.Fatalf("the multimodal model was handed %+v, want the one queued image", msg.ImageParts)
	}
	if strings.Contains(msg.Content, "[image not shown]") {
		t.Fatalf("the message names a drop that did not happen: %q", msg.Content)
	}
}
