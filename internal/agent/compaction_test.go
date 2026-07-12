package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// summarizeProvider is a fake llm.Provider whose Complete returns a canned summary and records
// the messages it was asked to summarize.
type summarizeProvider struct {
	summary string
	seen    []llm.Message
	calls   int
}

func (p *summarizeProvider) Complete(_ context.Context, msgs []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	p.calls++
	p.seen = append([]llm.Message(nil), msgs...)
	return &llm.Response{Content: p.summary, StopReason: "end_turn"}, nil
}

func (p *summarizeProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, _ func(llm.StreamChunk)) (*llm.Response, error) {
	return &llm.Response{StopReason: "end_turn"}, nil
}

// convo builds a simple user→assistant→tool→...→user history for boundary tests.
func userMsg(s string) llm.Message { return llm.Message{Role: llm.RoleUser, Content: s} }
func asstMsg(s string) llm.Message {
	return llm.Message{Role: llm.RoleAssistant, Content: s, ToolCalls: []llm.ToolCall{{ID: "c", Name: "read"}}}
}
func toolMsg(s string) llm.Message {
	return llm.Message{Role: llm.RoleTool, Content: s, ToolCallID: "c"}
}

func TestCompactionBoundaryKeepsLastTurnsAndPairsToolResults(t *testing.T) {
	// Three user turns; each turn: user, assistant(tool_calls), tool result.
	h := []llm.Message{
		userMsg("u1"), asstMsg("a1"), toolMsg("t1"),
		userMsg("u2"), asstMsg("a2"), toolMsg("t2"),
		userMsg("u3"), asstMsg("a3"), toolMsg("t3"),
	}
	// Keep last 2 user turns → boundary at start of turn 2 (index 3).
	got := compactionBoundary(h, 2)
	if got != 3 {
		t.Fatalf("boundary = %d, want 3", got)
	}
	// The message at the boundary must be a RoleUser (never a tool result → no orphaning).
	if h[got].Role != llm.RoleUser {
		t.Fatalf("boundary message role = %s, want user", h[got].Role)
	}
}

func TestCompactionBoundaryTooFewTurns(t *testing.T) {
	h := []llm.Message{userMsg("u1"), asstMsg("a1"), toolMsg("t1"), userMsg("u2")}
	if got := compactionBoundary(h, 2); got != 0 {
		t.Fatalf("boundary = %d, want 0 (only 2 user turns, keep 2)", got)
	}
}

func TestCompactionBoundarySkipsWhenNothingFresh(t *testing.T) {
	// Everything before the boundary is already compacted / a prior summary → no fresh content.
	h := []llm.Message{
		{Role: llm.RoleUser, Content: "old", Compacted: true},
		{Role: llm.RoleUser, Content: "summary", CompactionSummary: true},
		userMsg("u2"),
		userMsg("u3"),
	}
	if got := compactionBoundary(h, 2); got != 0 {
		t.Fatalf("boundary = %d, want 0 (nothing fresh before boundary)", got)
	}
}

func TestBuildCompactedHistory(t *testing.T) {
	h := []llm.Message{
		userMsg("u1"), asstMsg("a1"), toolMsg("t1"),
		userMsg("u2"), asstMsg("a2"), toolMsg("t2"),
	}
	out := buildCompactedHistory(h, 3, "SUMMARY")
	// First 3 marked compacted.
	for i := 0; i < 3; i++ {
		if !out[i].Compacted {
			t.Fatalf("message %d should be compacted", i)
		}
	}
	// One inserted summary message, non-compacted.
	sum := out[3]
	if !sum.CompactionSummary || sum.Compacted {
		t.Fatalf("expected non-compacted summary at index 3, got %+v", sum)
	}
	if !strings.Contains(sum.Content, "SUMMARY") {
		t.Fatalf("summary content = %q", sum.Content)
	}
	// Tail preserved verbatim after the summary.
	if out[4].Content != "u2" || out[5].Content != "a2" || out[6].Content != "t2" {
		t.Fatalf("tail not preserved: %+v", out[4:])
	}
	if len(out) != len(h)+1 {
		t.Fatalf("len = %d, want %d", len(out), len(h)+1)
	}
}

func TestIsLLMHistoryMessageExcludesCompacted(t *testing.T) {
	if isLLMHistoryMessage(llm.Message{Role: llm.RoleUser, Content: "x", Compacted: true}) {
		t.Error("compacted message must be excluded from LLM payload")
	}
	if !isLLMHistoryMessage(llm.Message{Role: llm.RoleUser, Content: "s", CompactionSummary: true}) {
		t.Error("compaction summary must be kept in LLM payload")
	}
}

// smallWindowConfig returns a config whose only model has a tiny context window so compaction
// thresholds are easy to cross in tests.
func smallWindowConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "p", Type: "openai", APIKey: "k"}},
		Models:    []config.ModelEntry{{Model: "p/m", MaxTokens: 100, MaxContextTokens: 1000, Temperature: 0.1}},
	}
	cfg.Agent.Model = "p/m"
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Compaction.ApplyDefaults() // Enabled=true, threshold 85, keep 2
	return cfg
}

func makeHistory() []llm.Message {
	return []llm.Message{
		userMsg("u1"), asstMsg("a1"), toolMsg("t1"),
		userMsg("u2"), asstMsg("a2"), toolMsg("t2"),
		userMsg("u3"), asstMsg("a3"), toolMsg("t3"),
	}
}

func TestMaybeCompactTriggersAboveThreshold(t *testing.T) {
	cfg := smallWindowConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(makeHistory())
	prov := &summarizeProvider{summary: "the summary"}
	a := NewAgent(cfg, st, resumePermissionSender{}, nil)

	// usable = 1000-100 = 900; threshold 85% = 765. Pass 800 → should trigger.
	did, err := a.maybeCompact(context.Background(), prov, 800)
	if err != nil {
		t.Fatalf("maybeCompact: %v", err)
	}
	if !did {
		t.Fatal("expected compaction to trigger")
	}
	if prov.calls != 1 {
		t.Fatalf("summarizer called %d times, want 1", prov.calls)
	}
	msgs := st.GetMessages()
	var summaries, compacted int
	for _, m := range msgs {
		if m.CompactionSummary {
			summaries++
		}
		if m.Compacted {
			compacted++
		}
	}
	if summaries != 1 {
		t.Fatalf("summary messages = %d, want 1", summaries)
	}
	if compacted == 0 {
		t.Fatal("expected some messages marked compacted")
	}
	// The last user turn (u3/a3/t3) must be preserved verbatim and not compacted.
	last3 := msgs[len(msgs)-3:]
	for _, m := range last3 {
		if m.Compacted {
			t.Fatalf("recent turn must not be compacted: %+v", m)
		}
	}
}

func TestMaybeCompactSkipsBelowThreshold(t *testing.T) {
	cfg := smallWindowConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(makeHistory())
	prov := &summarizeProvider{summary: "x"}
	a := NewAgent(cfg, st, resumePermissionSender{}, nil)

	// 100 < threshold 765 → no compaction.
	did, err := a.maybeCompact(context.Background(), prov, 100)
	if err != nil || did {
		t.Fatalf("did=%v err=%v, want no compaction", did, err)
	}
	if prov.calls != 0 {
		t.Fatal("summarizer must not be called below threshold")
	}
}

func TestMaybeCompactDisabled(t *testing.T) {
	cfg := smallWindowConfig(t)
	off := false
	cfg.Compaction.Enabled = &off
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(makeHistory())
	prov := &summarizeProvider{summary: "x"}
	a := NewAgent(cfg, st, resumePermissionSender{}, nil)

	did, err := a.maybeCompact(context.Background(), prov, 999999)
	if err != nil || did {
		t.Fatalf("did=%v err=%v, want no compaction when disabled", did, err)
	}
}

func TestMaybeCompactAbortsOnEmptySummary(t *testing.T) {
	cfg := smallWindowConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(makeHistory())
	prov := &summarizeProvider{summary: "   "} // whitespace only
	a := NewAgent(cfg, st, resumePermissionSender{}, nil)

	did, err := a.maybeCompact(context.Background(), prov, 800)
	if err != nil {
		t.Fatalf("maybeCompact: %v", err)
	}
	if did {
		t.Fatal("must not compact when summary is empty")
	}
	// History must be untouched.
	for _, m := range st.GetMessages() {
		if m.Compacted || m.CompactionSummary {
			t.Fatal("history must be unchanged when summary is empty")
		}
	}
}
