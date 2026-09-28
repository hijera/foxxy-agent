package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// fork(continue-path) guard: with agent.llm_continue off the turn ends at the
// cut, as upstream 1.1.47 always does, but the half-written answer the user
// watched arrive is still kept and the notice names the setting.
func TestContinueOffEndsTheTurnAtTheCut(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{partial: "The first half of the answer."},
		{answer: " never asked for"},
	}}
	off := false
	h := newStallHarness(t, p, func(a *config.Agent) { a.LLMContinue = &off })

	stop, err := h.run(t)
	if err == nil || !strings.Contains(err.Error(), "agent.llm_continue") {
		t.Fatalf("err = %v, want the notice to name agent.llm_continue", err)
	}
	if stop != string(acp.StopReasonRefused) {
		t.Errorf("stop reason = %q, want agent_refused", stop)
	}
	if got := p.callCount(); got != 1 {
		t.Errorf("provider called %d times, want 1 (no continuation)", got)
	}
	msgs := h.assistantMessages()
	if len(msgs) != 1 || msgs[0].Content != "The first half of the answer." {
		t.Fatalf("assistant messages = %+v, want the partial answer kept", msgs)
	}
}

// fork(continue-budget) guard: the budget is agent.llm_continue_max, and 0
// behaves like llm_continue: false.
func TestContinueMaxIsConfigurable(t *testing.T) {
	for _, c := range []struct {
		max       int
		wantCalls int
		wantMsg   string
	}{
		{max: 1, wantCalls: 2, wantMsg: "did not finish after 1 continuation"},
		{max: 0, wantCalls: 1, wantMsg: "agent.llm_continue"},
	} {
		p := &stallProvider{script: []stallBehaviour{{partial: "Half an answer."}}}
		max := c.max
		h := newStallHarness(t, p, func(a *config.Agent) { a.LLMContinueMax = &max })
		_, err := h.run(t)
		if err == nil || !strings.Contains(err.Error(), c.wantMsg) {
			t.Errorf("llm_continue_max %d: err = %v, want %q", c.max, err, c.wantMsg)
		}
		if got := p.callCount(); got != c.wantCalls {
			t.Errorf("llm_continue_max %d: provider called %d times, want %d", c.max, got, c.wantCalls)
		}
	}
}

// timedStall wraps a stallProvider and records when each call went out.
type timedStall struct {
	*stallProvider
	mu     sync.Mutex
	starts []time.Time
	ends   []time.Time
}

func (p *timedStall) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.mu.Lock()
	p.starts = append(p.starts, time.Now())
	p.mu.Unlock()
	resp, err := p.stallProvider.Stream(ctx, messages, tools, onChunk)
	p.mu.Lock()
	p.ends = append(p.ends, time.Now())
	p.mu.Unlock()
	return resp, err
}

// The pause before carrying on a stalled answer is agent.llm_continue_stall_delays_ms,
// and the client is told how long it is.
func TestStallContinuationWaitsTheConfiguredPause(t *testing.T) {
	const pause = 150 * time.Millisecond
	p := &timedStall{stallProvider: &stallProvider{script: []stallBehaviour{
		{partial: "Half an answer."},
		{answer: " The rest."},
	}}}
	h := newStallHarness(t, p.stallProvider, func(a *config.Agent) {
		a.LLMContinueStallDelaysMS = []int{int(pause / time.Millisecond)}
	})
	h.ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return p, nil }

	if _, err := h.run(t); err != nil {
		t.Fatalf("Run: %v", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.starts) != 2 {
		t.Fatalf("calls = %d, want 2", len(p.starts))
	}
	if gap := p.starts[1].Sub(p.ends[0]); gap < pause {
		t.Errorf("the continuation went out %v after the cut, before the %v pause", gap, pause)
	}
	var announced bool
	h.sender.mu.Lock()
	for _, u := range h.sender.retries {
		if u.Phase == acp.LLMRetryPhaseContinuing && u.DelayMS == int64(pause/time.Millisecond) {
			announced = true
		}
	}
	h.sender.mu.Unlock()
	if !announced {
		t.Error("the continuing update did not carry the pause")
	}
}
