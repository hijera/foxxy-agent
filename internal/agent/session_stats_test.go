package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// statsProvider reports fixed per-call token counters. With toolCallsPerTurn > 0 it spends
// several model calls on one turn, which is what separates per-call counters from the
// turn-cumulative ones. beforeRun lets a test simulate another writer touching stats.json
// while the turn is in flight.
type statsProvider struct {
	in, out          int
	toolCallsPerTurn int
	beforeRun        func()

	calls          int
	toolCallsSoFar int
}

func (p *statsProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Complete must not be used by the stats suite")
}

func (p *statsProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	if p.beforeRun != nil {
		p.beforeRun()
	}
	if p.toolCallsSoFar < p.toolCallsPerTurn {
		p.toolCallsSoFar++
		tc := llm.ToolCall{
			ID:        fmt.Sprintf("call_%d", p.calls),
			Name:      "glob",
			InputJSON: `{"pattern":"**/*.go"}`,
		}
		onChunk(llm.StreamChunk{ToolCall: &tc})
		return &llm.Response{
			ToolCalls:    []llm.ToolCall{tc},
			StopReason:   "tool_use",
			InputTokens:  p.in,
			OutputTokens: p.out,
		}, nil
	}
	p.toolCallsSoFar = 0
	onChunk(llm.StreamChunk{TextDelta: "ok"})
	return &llm.Response{
		Content:      "ok",
		StopReason:   "end_turn",
		InputTokens:  p.in,
		OutputTokens: p.out,
	}, nil
}

type statsSender struct {
	resumePermissionSender
	tokenUsage []acp.TokenUsageUpdate
}

func (s *statsSender) SendSessionUpdate(_ string, update interface{}) error {
	if u, ok := update.(acp.TokenUsageUpdate); ok {
		s.tokenUsage = append(s.tokenUsage, u)
	}
	return nil
}

func statsAgent(t *testing.T, provider llm.Provider, sender acp.UpdateSender) (*Agent, *session.State) {
	t.Helper()
	st := &session.State{
		ID:         "sess_stats",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxContextTokens: 128000}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 4},
	}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Compaction.ApplyDefaults()
	ag := NewAgent(cfg, st, sender, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	return ag, st
}

// Before this, every turn rewrote stats.json from scratch with counters that reset per
// turn, so the stored "total" was the last turn's usage and the session history was gone.
func TestSessionStatsAccumulateAcrossTurns(t *testing.T) {
	provider := &statsProvider{in: 100, out: 20}
	sender := &statsSender{}
	ag, st := statsAgent(t, provider, sender)

	for i := 0; i < 2; i++ {
		if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hello"}}); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}

	stats, err := session.ReadSessionStats(st.SessionDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := stats.TokenUsageTotal; got.InputTokens != 200 || got.OutputTokens != 40 || got.TotalTokens != 240 {
		t.Fatalf("session totals = %+v, want both turns counted", got)
	}
	if len(stats.TokenUsageByTurn) != 2 {
		t.Fatalf("per-turn history = %+v, want one row per turn", stats.TokenUsageByTurn)
	}
}

// Input + Output must equal Total: the SPA renders the three side by side, and the fork
// used to send the last model call's input/output next to a turn-cumulative total. A turn
// with a tool call spends two model calls, which is where the two disagree.
func TestTokenUsageUpdateIsCumulativeForTheTurn(t *testing.T) {
	provider := &statsProvider{in: 70, out: 30, toolCallsPerTurn: 1}
	sender := &statsSender{}
	ag, _ := statsAgent(t, provider, sender)

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hello"}}); err != nil {
		t.Fatal(err)
	}
	if len(sender.tokenUsage) < 2 {
		t.Fatalf("token_usage updates = %d, want one per model call", len(sender.tokenUsage))
	}
	for i, u := range sender.tokenUsage {
		if u.InputTokens+u.OutputTokens != u.TotalTokens {
			t.Fatalf("update %d: %d + %d != %d", i, u.InputTokens, u.OutputTokens, u.TotalTokens)
		}
	}
	last := sender.tokenUsage[len(sender.tokenUsage)-1]
	if last.InputTokens != 140 || last.OutputTokens != 60 {
		t.Fatalf("last update = %+v, want both model calls counted", last)
	}
}

// stats.json is shared: compaction writes the context breakdown into it while the turn
// runs (refreshConversationContextUsage -> WriteSessionContextBreakdown). A token write
// built on a document read before that would silently revert the post-compaction estimate,
// and session load would then restore the stale value.
func TestTokenStatsWriteDoesNotClobberACompactionBreakdown(t *testing.T) {
	compacted := &session.ContextBreakdown{SystemPrompt: 100, Conversation: 300, EstimatedTotal: 400}

	provider := &statsProvider{in: 10, out: 5}
	sender := &statsSender{}
	ag, st := statsAgent(t, provider, sender)
	provider.beforeRun = func() {
		// Stand in for compaction: another writer updates only the breakdown.
		if err := session.WriteSessionContextBreakdown(st.SessionDir, compacted); err != nil {
			t.Errorf("simulated compaction write: %v", err)
		}
		st.SetLastContextBreakdown(compacted)
	}

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hello"}}); err != nil {
		t.Fatal(err)
	}

	stats, err := session.ReadSessionStats(st.SessionDir)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ContextBreakdown == nil {
		t.Fatal("context breakdown was dropped by the token write")
	}
	if stats.ContextBreakdown.EstimatedTotal > compacted.EstimatedTotal {
		t.Fatalf("post-compaction estimate reverted: got %d, want no more than %d",
			stats.ContextBreakdown.EstimatedTotal, compacted.EstimatedTotal)
	}
	if stats.TokenUsageTotal.TotalTokens != 15 {
		t.Fatalf("token totals = %+v, want the turn counted", stats.TokenUsageTotal)
	}
}
