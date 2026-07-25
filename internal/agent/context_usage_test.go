package agent

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// coddyCompactedHistory is a transcript folded by the coddy engine: the older exchange keeps NO
// flag (the transcript stays intact for the UI) and the summary row marks where the replayed
// window starts. Anything before that row must not count toward the context estimate.
func coddyCompactedHistory() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleUser, Content: "FOLDED-QUESTION"},
		{Role: llm.RoleAssistant, Content: "FOLDED-ANSWER"},
		{Role: llm.RoleUser, Content: summaryPrefix + "THE-SUMMARY", CompactionSummary: true},
		{Role: llm.RoleUser, Content: "live question"},
		{Role: llm.RoleAssistant, Content: "live answer"},
	}
}

// openCodeCompactedHistory is the same conversation folded by the opencode engine, which flags the
// folded messages instead of relying on the summary position.
func openCodeCompactedHistory() []llm.Message {
	h := coddyCompactedHistory()
	h[0].Compacted = true
	h[1].Compacted = true
	return h
}

func agentWithEngine(t *testing.T, engine string, history []llm.Message) (*Agent, *session.State) {
	t.Helper()
	cfg := &config.Config{
		Models: []config.ModelEntry{{Model: "fake/model", MaxContextTokens: 128000}},
		Agent:  config.Agent{Model: "fake/model"},
		Compaction: config.CompactionConfig{
			Engine: engine,
		},
	}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Compaction.ApplyDefaults()
	st := &session.State{ID: "usage-test", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(history)
	return NewAgent(cfg, st, nil, nil), st
}

func TestLLMVisibleMessagesMatchesEngineWindow(t *testing.T) {
	// The coddy engine replays from the summary row onward; the opencode engine keeps every
	// message that is not flagged Compacted. Same conversation, different bookkeeping.
	cases := []struct {
		engine  string
		history []llm.Message
		want    int
	}{
		{config.CompactionEngineCoddy, coddyCompactedHistory(), 3},
		{config.CompactionEngineOpenCode, openCodeCompactedHistory(), 3},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			a, _ := agentWithEngine(t, tc.engine, tc.history)
			window := a.llmVisibleMessages()
			if len(window) != tc.want {
				t.Fatalf("%s window size = %d, want %d", tc.engine, len(window), tc.want)
			}
			for _, m := range window {
				if m.Compacted || m.Content == "FOLDED-QUESTION" || m.Content == "FOLDED-ANSWER" {
					t.Fatalf("folded message stayed in the %s window: %q", tc.engine, m.Content)
				}
			}
		})
	}
}

// The context HUD numerator must describe the payload, not the archive. Before this the fork fed
// the unfiltered transcript into computeContextBreakdown, so on the default coddy engine — whose
// folded messages carry no flag — the ring never dropped after compaction.
func TestBuildSystemPromptExcludesFoldedTurnsFromConversation(t *testing.T) {
	a, st := agentWithEngine(t, config.CompactionEngineCoddy, coddyCompactedHistory())
	_ = a.buildSystemPrompt("agent", nil, nil, "", nil)

	b := st.GetLastContextBreakdown()
	if b == nil {
		t.Fatal("expected a context breakdown")
	}
	wantConversation := session.EstimateTokens(conversationText(a.llmVisibleMessages()))
	if b.Conversation != wantConversation {
		t.Fatalf("conversation tokens = %d, want %d", b.Conversation, wantConversation)
	}
	full := session.EstimateTokens(conversationText(st.GetMessages()))
	if b.Conversation >= full {
		t.Fatalf("conversation tokens %d still count the folded turns (full transcript = %d)", b.Conversation, full)
	}
	if b.Summary <= 0 {
		t.Fatalf("compaction summary not accounted separately: %+v", b)
	}
}

func TestRefreshConversationContextUsageMovesTextIntoSummary(t *testing.T) {
	a, st := agentWithEngine(t, config.CompactionEngineCoddy, coddyCompactedHistory())
	st.SetLastContextBreakdown(&session.ContextBreakdown{
		SystemPrompt: 100,
		Conversation: 10000,
		Summary:      0,
	})

	a.refreshConversationContextUsage(false)

	b := st.GetLastContextBreakdown()
	if b == nil {
		t.Fatal("expected a context breakdown")
	}
	if b.SystemPrompt != 100 {
		t.Fatalf("static category was clobbered: %+v", b)
	}
	if b.Conversation >= 10000 {
		t.Fatalf("conversation estimate not refreshed: %+v", b)
	}
	if b.Summary <= 0 {
		t.Fatalf("summary estimate not refreshed: %+v", b)
	}
	if b.EstimatedTotal != b.SystemPrompt+b.Conversation+b.Summary {
		t.Fatalf("total not re-summed: %+v", b)
	}
}
