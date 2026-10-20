package agent

// The message queue meets the fork's provider recovery (agent.llm_stall_retry):
// an iteration that re-issues a silent request or carries on a cut answer is not
// a step of the model's, so a follow-up the operator queued during it is read at
// the next real step instead of being folded into the replay.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// queueDuringFirstCall writes a follow-up into the session's queue while the
// first model call of the turn is in flight, the way an operator typing during a
// stalled request would.
type queueDuringFirstCall struct {
	*stallProvider
	enqueue func()
	once    sync.Once
}

func (p *queueDuringFirstCall) Stream(ctx context.Context, messages []llm.Message, defs []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.once.Do(p.enqueue)
	return p.stallProvider.Stream(ctx, messages, defs, onChunk)
}

func newQueueRecoveryHarness(t *testing.T, provider *stallProvider, followUp string, tune func(*config.Agent)) *stallHarness {
	t.Helper()
	h := newStallHarness(t, provider, tune)
	h.st.OpenMessageQueue()
	t.Cleanup(func() { h.st.CloseMessageQueue() })
	wrapped := &queueDuringFirstCall{stallProvider: provider}
	wrapped.enqueue = func() {
		if _, err := h.st.EnqueueMessage(followUp); err != nil {
			t.Errorf("enqueue: %v", err)
		}
	}
	h.ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return wrapped, nil }
	return h
}

func carries(req []llm.Message, text string) bool {
	for _, m := range req {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, text) {
			return true
		}
	}
	return false
}

// A silent first call is re-issued as the identical request. A follow-up queued
// while it hung must not ride along in the replay; it is read once the model has
// answered and gets an answer of its own.
func TestQueuedFollowUpWaitsOutTheSilentReissue(t *testing.T) {
	const followUp = "use the staging database instead"
	p := &stallProvider{script: []stallBehaviour{
		{silent: true},
		{answer: "Here is the first answer."},
		{answer: "Switched to staging."},
	}}
	h := newQueueRecoveryHarness(t, p, followUp, nil)

	if _, err := h.run(t); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := p.callCount(); got != 3 {
		t.Fatalf("provider called %d times, want 3 (silent, its re-issue, the follow-up)", got)
	}
	if carries(p.request(2), followUp) || len(p.request(2)) != len(p.request(1)) {
		t.Errorf("the re-issue carried %d messages (the original %d) and the follow-up=%v: it must be the identical request",
			len(p.request(2)), len(p.request(1)), carries(p.request(2), followUp))
	}
	if !carries(p.request(3), followUp) {
		t.Errorf("the request after the answer does not carry the follow-up")
	}
	msgs := h.st.GetMessages()
	if n := len(msgs); n < 2 || msgs[n-2].Content != followUp || msgs[n-1].Content != "Switched to staging." {
		t.Errorf("transcript does not end with the follow-up and its answer: %+v", msgs)
	} else if !msgs[n-2].Queued || msgs[0].Queued {
		t.Errorf("only the follow-up may be marked queued: prompt=%v follow-up=%v", msgs[0].Queued, msgs[n-2].Queued)
	}
}

// A stream cut mid-answer is continued from what the operator already watched
// arrive. The follow-up is not slipped between the half-written answer and its
// continuation: it comes after the answer is whole.
func TestQueuedFollowUpDoesNotSplitAStalledAnswer(t *testing.T) {
	const followUp = "and mention the timeout"
	p := &stallProvider{script: []stallBehaviour{
		{partial: "The first half"},
		{answer: " and the second half."},
		{answer: "The timeout is 30 seconds."},
	}}
	h := newQueueRecoveryHarness(t, p, followUp, nil)

	if _, err := h.run(t); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if carries(p.request(2), followUp) {
		t.Errorf("the continuation of the cut answer carried the follow-up")
	}
	if !carries(p.request(3), followUp) {
		t.Errorf("the request after the completed answer does not carry the follow-up")
	}
	msgs := h.st.GetMessages()
	firstUser := -1
	for i, m := range msgs {
		if m.Role == llm.RoleUser && m.Content == followUp {
			firstUser = i
		}
	}
	if firstUser < 0 {
		t.Fatalf("the follow-up is not in the transcript: %+v", msgs)
	}
	for _, m := range msgs[firstUser+1:] {
		if strings.Contains(m.Content, "second half") {
			t.Errorf("the continuation of the cut answer landed after the follow-up: %+v", msgs)
		}
	}
}

// The end-of-turn read counts the model's own steps. A re-issue does not shrink
// max_turns, so with two steps allowed and one silent call behind it the answer
// is still on the first step, and a follow-up queued meanwhile is answered.
func TestEndOfTurnQueueReadCountsOnlyTheModelsSteps(t *testing.T) {
	const followUp = "one more thing"
	p := &stallProvider{script: []stallBehaviour{
		{silent: true},
		{answer: "Done."},
		{answer: "And the one more thing."},
	}}
	h := newQueueRecoveryHarness(t, p, followUp, func(c *config.Agent) { c.MaxTurns = 2 })

	if _, err := h.run(t); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := p.callCount(); got != 3 {
		t.Fatalf("provider called %d times, want 3: the follow-up must be read on the second step", got)
	}
	if q := h.st.QueuedMessages(); len(q) != 0 {
		t.Errorf("the follow-up is still waiting: %+v", q)
	}
}
