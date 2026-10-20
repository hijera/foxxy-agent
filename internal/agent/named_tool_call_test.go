package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// namedThenAnswerProvider names a call while its arguments would still be
// streaming and then finishes without it, the way a provider-level replay does:
// the announcement of the dead attempt is never followed by the call.
type namedThenAnswerProvider struct{}

func (namedThenAnswerProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (namedThenAnswerProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	onChunk(llm.StreamChunk{ToolCallNamed: &llm.ToolCall{ID: "call_dead", Name: "read"}})
	onChunk(llm.StreamChunk{TextDelta: "done"})
	return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
}

type toolRowSender struct {
	resumePermissionSender
	mu      sync.Mutex
	pending []string
	closed  map[string]string
}

func (s *toolRowSender) SendSessionUpdate(_ string, update interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch u := update.(type) {
	case acp.ToolCallUpdate:
		if u.Status == "pending" {
			s.pending = append(s.pending, u.ToolCallID)
		}
	case acp.ToolCallStatusUpdate:
		if s.closed == nil {
			s.closed = map[string]string{}
		}
		s.closed[u.ToolCallID] = u.Status
	}
	return nil
}

// A named-only announcement draws a pending row. When the call never arrives
// the row has nothing behind it and must not stay pending forever.
func TestNamedOnlyToolCallRowIsRetractedWhenTheCallNeverArrives(t *testing.T) {
	st := &session.State{
		ID:         "sess_named",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	sender := &toolRowSender{}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model"}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 4},
	}, st, sender, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return namedThenAnswerProvider{}, nil }

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hi"}}); err != nil {
		t.Fatalf("run: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.pending) != 1 || sender.pending[0] != "call_dead" {
		t.Fatalf("pending rows = %v, want the named call announced once", sender.pending)
	}
	if got := sender.closed["call_dead"]; got != "cancelled" {
		t.Fatalf("named-only row status = %q, want cancelled", got)
	}
	for _, m := range st.GetMessages() {
		if len(m.ToolCalls) > 0 {
			t.Fatalf("a named-only announcement must never be persisted as a call: %+v", m)
		}
	}
}
