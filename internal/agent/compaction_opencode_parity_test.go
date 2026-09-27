package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// The opencode engine gets everything upstream gave the coddy engine in #262.
// These tests hold that parity: the same door (CompactSession), the engine's own
// way of writing the summary back (Compacted flags, not a summary row window).

func opencodeCompaction(keep int) config.CompactionConfig {
	return config.CompactionConfig{Engine: config.CompactionEngineOpenCode, KeepRecentTurns: &keep}
}

func countFlags(msgs []llm.Message) (compacted, summaries int) {
	for _, m := range msgs {
		if m.Compacted {
			compacted++
		}
		if m.CompactionSummary {
			summaries++
		}
	}
	return compacted, summaries
}

// The manual command and the REST endpoint both call CompactSession. On the
// opencode engine that used to insert a coddy summary row the engine's own
// replay filter never looked at, so the window did not shrink at all.
func TestCompactSessionOnTheOpenCodeEngineFlagsTheFoldedMessages(t *testing.T) {
	st := seededCompactState(t, 3)
	provider := &compactCannedProvider{t: t, summary: "dense summary"}
	ag := compactTestAgent(t, st, opencodeCompaction(1), provider)

	res, err := ag.CompactSession(context.Background(), "keep the file names", true)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}
	msgs := st.GetMessages()
	compacted, summaries := countFlags(msgs)
	if compacted != 4 || summaries != 1 {
		t.Fatalf("flags = %d compacted / %d summaries, want 4 / 1: %+v", compacted, summaries, msgs)
	}
	if res.CompactedMessages != 4 || res.KeptMessages != 2 || res.Steps != 1 {
		t.Fatalf("result = %+v", res)
	}
	// What the model is sent next is the summary plus the kept turn.
	window := ag.llmVisibleMessages()
	if len(window) != 3 || !window[0].CompactionSummary || window[1].Content != "question 3" {
		t.Fatalf("replay window = %+v", window)
	}
	// The engine summarizes under its own system prompt, with the instructions.
	req := provider.requests[0]
	if !strings.Contains(req[0].Content, strings.TrimSpace(compactionSystemPrompt)) {
		t.Fatalf("summarizer system prompt is not the opencode engine's:\n%s", req[0].Content)
	}
	if !strings.Contains(req[1].Content, "keep the file names") {
		t.Fatalf("instructions did not reach the summarizer:\n%s", req[1].Content)
	}
}

// A second compaction used to mark the first summary Compacted and summarize
// only the raw messages after it, so everything the first one held was lost.
func TestSecondOpenCodeCompactionCarriesTheEarlierSummary(t *testing.T) {
	st := seededCompactState(t, 3)
	provider := &compactCannedProvider{t: t, summary: "FIRST SUMMARY about the parser"}
	ag := compactTestAgent(t, st, opencodeCompaction(1), provider)
	if _, err := ag.CompactSession(context.Background(), "", true); err != nil {
		t.Fatalf("first compaction: %v", err)
	}

	for i := 4; i <= 5; i++ {
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("question %d", i)})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("answer %d", i)})
	}
	provider.summary = "SECOND SUMMARY"
	if _, err := ag.CompactSession(context.Background(), "", true); err != nil {
		t.Fatalf("second compaction: %v", err)
	}

	second := transcriptText(provider.requests[len(provider.requests)-1])
	if !strings.Contains(second, "FIRST SUMMARY about the parser") {
		t.Fatalf("the second compaction did not read the first summary:\n%s", second)
	}
	if strings.Contains(second, "question 1") {
		t.Fatalf("messages an earlier compaction folded were summarized again:\n%s", second)
	}
	window := ag.llmVisibleMessages()
	if n := len(window); n != 3 || !window[0].CompactionSummary || !strings.Contains(window[0].Content, "SECOND SUMMARY") {
		t.Fatalf("replay window after the second compaction = %+v", window)
	}
}

// A head that does not fit the summarizer's window is folded in passes, each
// carrying the summary so far - on this engine too.
func TestOpenCodeCompactionFoldsInPassesWhenTheHeadOutgrowsTheSummarizer(t *testing.T) {
	st := &session.State{ID: "sess_opencode_fold", CWD: t.TempDir(), Mode: session.ModeAgent}
	filler := "\n" + strings.Repeat("a line of the log this turn pasted into the session. ", 120)
	for i := 1; i <= 4; i++ {
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("question %d%s", i, filler)})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("answer %d", i)})
	}
	provider := &compactCannedProvider{t: t, summary: "running summary"}
	ag := compactTestAgent(t, st, opencodeCompaction(1), provider)
	// A window this small resolves the pass budget down to the floor.
	ag.cfg.Models[0].MaxContextTokens = 1

	res, err := ag.CompactSession(context.Background(), "", true)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}
	if res.Steps < 2 || len(provider.requests) != res.Steps {
		t.Fatalf("steps = %d over %d requests, want a fold in several passes", res.Steps, len(provider.requests))
	}
	for i, req := range provider.requests[1:] {
		if !strings.Contains(req[1].Content, "<summary-so-far>") {
			t.Fatalf("pass %d did not carry the summary so far:\n%.300s", i+2, req[1].Content)
		}
	}
	if _, summaries := countFlags(st.GetMessages()); summaries != 1 {
		t.Fatalf("a fold in passes must still leave one summary message, got %d", summaries)
	}
}

// compaction.fallback_models serves this engine as well.
func TestOpenCodeCompactionFallsBackToTheNextSummarizer(t *testing.T) {
	st := seededCompactState(t, 3)
	broken := &compactCannedProvider{t: t, err: fmt.Errorf("503 model overloaded")}
	working := &compactCannedProvider{t: t, summary: "SUMMARY FROM THE DEPUTY"}
	comp := opencodeCompaction(1)
	comp.Model = "fake/broken"
	comp.FallbackModels = []string{"fake/model"}
	ag := compactTestAgent(t, st, comp, nil)
	ag.cfg.Models = append(ag.cfg.Models, config.ModelEntry{Model: "fake/broken", MaxTokens: 100})
	ag.providerFactory = func(in llm.ProviderInput) (llm.Provider, error) {
		if strings.Contains(in.Model, "broken") {
			return broken, nil
		}
		return working, nil
	}

	res, err := ag.CompactSession(context.Background(), "", true)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}
	if len(broken.requests) == 0 || res.Model != "fake/model" {
		t.Fatalf("broken tried %d time(s), summary model %q", len(broken.requests), res.Model)
	}
}

type compactionFrameSender struct {
	resumePermissionSender
	mu     sync.Mutex
	phases []string
}

func (s *compactionFrameSender) SendSessionUpdate(_ string, update interface{}) error {
	if u, ok := update.(acp.CompactionUpdate); ok {
		s.mu.Lock()
		s.phases = append(s.phases, string(u.Phase))
		s.mu.Unlock()
	}
	return nil
}

// The start frame raises the client's "compacting" state; a summarizer that
// fails used to leave it raised, because no done frame followed.
func TestOpenCodeCompactionAlwaysTellsTheClientItEnded(t *testing.T) {
	st := seededCompactState(t, 3)
	sender := &compactionFrameSender{}
	keep := 1
	ag := NewAgent(&config.Config{
		Providers:  []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:     []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:      config.Agent{Model: "fake/model"},
		Compaction: config.CompactionConfig{Engine: config.CompactionEngineOpenCode, KeepRecentTurns: &keep},
	}, st, sender, nil)
	broken := &compactCannedProvider{t: t, err: fmt.Errorf("401 bad key")}
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return broken, nil }

	if _, err := ag.CompactSession(context.Background(), "", true); err == nil {
		t.Fatal("a failing summarizer must fail the compaction")
	}
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.phases) != 2 || sender.phases[0] != string(acp.CompactionPhaseStart) || sender.phases[1] != string(acp.CompactionPhaseDone) {
		t.Fatalf("compaction frames = %v, want start then done", sender.phases)
	}
	if compacted, summaries := countFlags(st.GetMessages()); compacted != 0 || summaries != 0 {
		t.Fatal("history must stay untouched when the summarizer fails")
	}
}

// The model's own compact_context tool folds with the engine the session runs on.
func TestCompactContextToolUsesTheOpenCodeEngine(t *testing.T) {
	st := seededCompactState(t, 3)
	ag := compactTestAgent(t, st, opencodeCompaction(1), &compactCannedProvider{t: t, summary: "dense summary"})

	text, err := ag.compactFromTool(context.Background(), "")
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if !strings.Contains(text, "Context compacted") {
		t.Fatalf("tool result = %q", text)
	}
	if compacted, summaries := countFlags(st.GetMessages()); compacted == 0 || summaries != 1 {
		t.Fatalf("the tool did not fold with the opencode engine: %d compacted / %d summaries", compacted, summaries)
	}
}

// The opencode trigger lends the turn's provider to the session model and keeps
// treating an unusable summary as "skip", not as a failed turn.
func TestMaybeCompactUsesTheTurnProviderAndSkipsAnEmptySummary(t *testing.T) {
	cfg := smallWindowConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(makeHistory())
	a := NewAgent(cfg, st, resumePermissionSender{}, nil)
	a.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		t.Fatal("the turn's provider must summarize for the session model; no second provider is built")
		return nil, nil
	}

	blank := &summarizeProvider{summary: "  "}
	if did, err := a.maybeCompact(context.Background(), blank, 800); did || err != nil {
		t.Fatalf("empty summary: did=%v err=%v, want a skipped compaction", did, err)
	}
	good := &summarizeProvider{summary: "the summary"}
	if did, err := a.maybeCompact(context.Background(), good, 800); !did || err != nil {
		t.Fatalf("did=%v err=%v, want a compaction", did, err)
	}
	if good.calls != 1 {
		t.Fatalf("turn provider called %d times, want 1", good.calls)
	}
}
