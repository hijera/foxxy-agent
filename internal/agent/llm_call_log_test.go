package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// causeProvider answers like stallProvider and records the cause the agent
// cancelled each call with.
type causeProvider struct {
	*stallProvider
	mu     sync.Mutex
	causes []error
}

func (p *causeProvider) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	resp, err := p.stallProvider.Stream(ctx, messages, tools, onChunk)
	p.mu.Lock()
	p.causes = append(p.causes, context.Cause(ctx))
	p.mu.Unlock()
	return resp, err
}

type syncLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *syncLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(b)
}

func (l *syncLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// Each guard cancels the model call with a cause of its own, and the debug log
// says per call which guard cut it: in a log of a turn that hung, a request cut by
// a timer must not read like one the proxy dropped.
func TestGuardCancelsCarryTheirCause(t *testing.T) {
	p := &causeProvider{stallProvider: &stallProvider{script: []stallBehaviour{
		{silent: true},
		{partial: "Half an answer.\n"},
		{answer: "The rest."},
	}}}
	h := newStallHarness(t, p.stallProvider, nil)
	h.ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return p, nil }
	var logs syncLog
	h.ag.log = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if _, err := h.run(t); err != nil {
		t.Fatalf("Run: %v", err)
	}

	p.mu.Lock()
	causes := append([]error(nil), p.causes...)
	p.mu.Unlock()
	if len(causes) != 3 {
		t.Fatalf("recorded %d calls, want 3", len(causes))
	}
	if !errors.Is(causes[0], errFirstTokenCut) {
		t.Errorf("silent call cancelled with %v, want the first-token cause", causes[0])
	}
	if !errors.Is(causes[1], errStallCut) {
		t.Errorf("stalled call cancelled with %v, want the stall cause", causes[1])
	}

	out := logs.String()
	for _, want := range []string{
		`msg="llm call started" session=sess_stall turn=`,
		"call=1",
		"cut_by=first_token_timeout",
		"cut_by=stall_timeout",
		"first_progress_after=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
	// The call that answered was cut by nobody.
	var last string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, `msg="llm call finished"`) {
			last = line
		}
	}
	if !strings.Contains(last, "call=3") || strings.Contains(last, "cut_by=") {
		t.Errorf("last finished call = %q, want call 3 with no cut_by", last)
	}
}

func TestLLMCallCutBy(t *testing.T) {
	cases := []struct {
		cause error
		user  bool
		want  string
	}{
		{nil, false, ""},
		{context.Canceled, false, ""},
		{errFirstTokenCut, false, "first_token_timeout"},
		{errStallCut, true, "stall_timeout"},
		{errLoopGuardCut, false, "loop_guard"},
		{context.Canceled, true, "user"},
		{context.DeadlineExceeded, false, "context deadline exceeded"},
	}
	for _, c := range cases {
		if got := llmCallCutBy(c.cause, c.user); got != c.want {
			t.Errorf("llmCallCutBy(%v, %v) = %q, want %q", c.cause, c.user, got, c.want)
		}
	}
}
