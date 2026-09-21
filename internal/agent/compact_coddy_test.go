package agent

// The coddy compaction engine's unit tests, as upstream keeps them in its
// react_test.go. The fork's react_test.go is split differently, so they live in
// a file of their own; compact_fold_test.go builds on the helpers defined here.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// compactCannedProvider serves Complete (summarization) with a canned summary
// and fails the test if Stream is called.
type compactCannedProvider struct {
	t        *testing.T
	summary  string
	err      error
	requests [][]llm.Message
}

func (p *compactCannedProvider) Complete(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	p.requests = append(p.requests, append([]llm.Message(nil), messages...))
	if p.err != nil {
		return nil, p.err
	}
	return &llm.Response{Content: p.summary, StopReason: "end_turn"}, nil
}

func (p *compactCannedProvider) Stream(context.Context, []llm.Message, []llm.ToolDefinition, func(llm.StreamChunk)) (*llm.Response, error) {
	p.t.Fatal("Stream must not be called by CompactSession")
	return nil, nil
}

func compactTestAgent(t *testing.T, st *session.State, comp config.CompactionConfig, provider llm.Provider) *Agent {
	t.Helper()
	ag := NewAgent(&config.Config{
		Providers:  []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:     []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:      config.Agent{Model: "fake/model"},
		Compaction: comp,
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}
	return ag
}

func seededCompactState(t *testing.T, exchanges int) *session.State {
	t.Helper()
	st := &session.State{
		ID:   "sess_compact_unit",
		CWD:  t.TempDir(),
		Mode: session.ModeAgent,
	}
	for i := 1; i <= exchanges; i++ {
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("question %d", i)})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("answer %d", i)})
	}
	return st
}

func TestCompactSessionInsertsSummaryAtBoundary(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	provider := &compactCannedProvider{t: t, summary: "dense summary"}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)

	res, err := ag.CompactSession(context.Background(), "focus on file paths", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "dense summary" {
		t.Fatalf("summary = %q", res.Summary)
	}
	if res.CompactedMessages != 4 || res.KeptMessages != 2 {
		t.Fatalf("counts = %d/%d, want 4/2", res.CompactedMessages, res.KeptMessages)
	}

	msgs := st.GetMessages()
	if len(msgs) != 7 {
		t.Fatalf("len = %d, want 7", len(msgs))
	}
	if !msgs[4].CompactionSummary {
		t.Fatalf("summary not inserted before the kept tail: %+v", msgs[4])
	}

	// The summarization request must carry the head transcript and the extra instructions.
	if len(provider.requests) != 1 {
		t.Fatalf("Complete called %d times", len(provider.requests))
	}
	req := transcriptText(provider.requests[0])
	for _, want := range []string{"question 1", "answer 2", "focus on file paths"} {
		if !strings.Contains(req, want) {
			t.Fatalf("summarization request misses %q:\n%s", want, req)
		}
	}
	if strings.Contains(req, "question 3") {
		t.Fatalf("kept tail leaked into the summarization request:\n%s", req)
	}
}

func TestCompactSessionPrunesHeadUsingWritesFromKeptTail(t *testing.T) {
	st := &session.State{
		ID:   "sess_compact_stale_read",
		CWD:  testCWD,
		Mode: session.ModeAgent,
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "inspect the file"})
	st.AddMessage(asstRead("read", "big.go", 1, 500, true))
	st.AddMessage(toolResult("read", bigBody("STALE FILE CONTENT")))
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "now change it"})
	st.AddMessage(asstWrite("write", "write", "big.go"))
	st.AddMessage(toolResult("write", "written"))

	keepTurns := 1
	keepResults := 0
	minBytes := 10
	provider := &compactCannedProvider{t: t, summary: "dense summary"}
	ag := compactTestAgent(t, st, config.CompactionConfig{
		KeepRecentTurns: &keepTurns,
		ResultEviction: config.ResultEviction{
			KeepRecent:     &keepResults,
			MinResultBytes: &minBytes,
		},
	}, provider)

	if _, err := ag.CompactSession(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	request := transcriptText(provider.requests[0])
	if strings.Contains(request, "STALE FILE CONTENT") {
		t.Fatalf("compaction request retained a read made stale by a write in the kept tail:\n%s", request)
	}
	if !strings.Contains(request, "modified after this read") {
		t.Fatalf("compaction request missing stale-read placeholder:\n%s", request)
	}
}

func TestCompactSessionNothingToCompact(t *testing.T) {
	// One user turn: the prompt being answered, which an automatic compaction
	// never folds, whatever keep_recent_turns says.
	st := seededCompactState(t, 1)
	keep := 2
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, &compactCannedProvider{t: t, summary: "s"})

	if _, err := ag.CompactSession(context.Background(), "", false); !errors.Is(err, ErrNothingToCompact) {
		t.Fatalf("err = %v, want ErrNothingToCompact", err)
	}
	if len(st.GetMessages()) != 2 {
		t.Fatal("history must stay untouched")
	}
}

func TestCompactSessionAutoKeepsFewerTurnsWhenTheTailCoversEveryTurn(t *testing.T) {
	// keep_recent_turns 2 over a window of exactly 2 user turns: a long session
	// of a few big agent turns. The automatic trigger folds the older turn and
	// keeps the latest one verbatim instead of skipping.
	st := seededCompactState(t, 2)
	keep := 2
	provider := &compactCannedProvider{t: t, summary: "folded first turn"}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)

	res, err := ag.CompactSession(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.CompactedMessages != 2 || res.KeptMessages != 2 {
		t.Fatalf("counts = %d/%d, want 2/2", res.CompactedMessages, res.KeptMessages)
	}
	msgs := st.GetMessages()
	if len(msgs) != 5 || !msgs[2].CompactionSummary {
		t.Fatalf("summary not inserted before the latest turn: %+v", msgs)
	}
	if msgs[3].Role != llm.RoleUser || msgs[3].Content != "question 2" {
		t.Fatalf("latest prompt not kept verbatim after the summary: %+v", msgs[3])
	}
	if req := transcriptText(provider.requests[0]); strings.Contains(req, "question 2") {
		t.Fatalf("the latest prompt leaked into the summarization request:\n%s", req)
	}
}

func TestCompactSessionDisabled(t *testing.T) {
	st := seededCompactState(t, 3)
	off := false
	ag := compactTestAgent(t, st, config.CompactionConfig{Enabled: &off}, &compactCannedProvider{t: t, summary: "s"})

	if _, err := ag.CompactSession(context.Background(), "", false); err == nil {
		t.Fatal("disabled compaction must error")
	}
}

func TestCompactSessionEmptySummaryFails(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, &compactCannedProvider{t: t, summary: "   "})

	if _, err := ag.CompactSession(context.Background(), "", false); err == nil {
		t.Fatal("empty summary must error")
	}
	if len(st.GetMessages()) != 6 {
		t.Fatal("failed compaction must not mutate history")
	}
}

func TestCompactSessionProviderErrorKeepsHistory(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	provider := &compactCannedProvider{t: t, err: errors.New("boom")}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)

	if _, err := ag.CompactSession(context.Background(), "", false); err == nil {
		t.Fatal("provider error must propagate")
	}
	if len(st.GetMessages()) != 6 {
		t.Fatal("failed compaction must not mutate history")
	}
}

// --- compact.go: /compact command interception -------------------------------

func TestParseCompactCommand(t *testing.T) {
	cases := []struct {
		in      string
		wantOK  bool
		wantArg string
	}{
		{in: "/compact", wantOK: true},
		{in: "  /compact  ", wantOK: true},
		{in: "/compact focus on file paths", wantOK: true, wantArg: "focus on file paths"},
		{in: "/compact\nkeep decisions", wantOK: true, wantArg: "keep decisions"},
		{in: "/compacted", wantOK: false},
		{in: "hello /compact", wantOK: false},
		{in: "", wantOK: false},
	}
	for _, tc := range cases {
		arg, ok := parseCompactCommand(tc.in)
		if ok != tc.wantOK || arg != tc.wantArg {
			t.Errorf("parseCompactCommand(%q) = (%q, %v), want (%q, %v)", tc.in, arg, ok, tc.wantArg, tc.wantOK)
		}
	}
}

func TestRunCompactCommandPersistsUserMessage(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	provider := &compactCannedProvider{t: t, summary: "dense summary"}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "/compact focus on tests"}})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q", stop)
	}

	msgs := st.GetMessages()
	foundCmd := false
	for _, m := range msgs {
		if m.Role == llm.RoleUser && strings.TrimSpace(m.Content) == "/compact focus on tests" {
			foundCmd = true
		}
	}
	if !foundCmd {
		t.Fatalf("the /compact command must be persisted as a user message so it shows in the transcript: %+v", msgs)
	}
	if !msgs[4].CompactionSummary {
		t.Fatalf("summary not inserted at boundary: %+v", msgs[4])
	}
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(strings.ToLower(last.Content), "compacted") {
		t.Fatalf("missing confirmation message: %+v", last)
	}
	if len(provider.requests) != 1 || !strings.Contains(transcriptText(provider.requests[0]), "focus on tests") {
		t.Fatalf("instructions not forwarded to the summarizer: %v", provider.requests)
	}
}

// TestRunCompactCommandForcesShortChat covers ask: manual /compact must fold even
// a very short conversation (below the keep-recent boundary), not refuse it.
func TestRunCompactCommandForcesShortChat(t *testing.T) {
	st := seededCompactState(t, 1) // one exchange (1 user turn) — below keep=2
	keep := 2
	provider := &compactCannedProvider{t: t, summary: "short summary"}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "/compact"}})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q", stop)
	}
	msgs := st.GetMessages()
	sawSummary := false
	for _, m := range msgs {
		if m.CompactionSummary {
			sawSummary = true
		}
	}
	if !sawSummary {
		t.Fatalf("forced /compact must summarize even a short chat: %+v", msgs)
	}
}

func TestRunCompactCommandNothingToCompact(t *testing.T) {
	st := seededCompactState(t, 0) // empty history: even forced compaction has nothing to fold
	keep := 2
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, &compactCannedProvider{t: t, summary: "s"})

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "/compact"}})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q", stop)
	}
	msgs := st.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, "Nothing to compact") {
		t.Fatalf("expected friendly notice, got %+v", last)
	}
	for _, m := range msgs {
		if m.CompactionSummary {
			t.Fatal("no summary row expected")
		}
	}
}

func TestRunCompactCommandDisabled(t *testing.T) {
	st := seededCompactState(t, 3)
	off := false
	ag := compactTestAgent(t, st, config.CompactionConfig{Enabled: &off}, &compactCannedProvider{t: t, summary: "s"})

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "/compact"}})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q", stop)
	}
	msgs := st.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(strings.ToLower(last.Content), "disabled") {
		t.Fatalf("expected disabled notice, got %+v", last)
	}
}

// --- compact.go: auto-compaction at the context threshold --------------------

func TestMaybeAutoCompactThresholdBoundary(t *testing.T) {
	cases := []struct {
		name        string
		est         int
		maxContext  int
		enabled     bool
		wantCompact bool
	}{
		{name: "exactly at threshold", est: 80, maxContext: 100, enabled: true, wantCompact: true},
		{name: "below threshold", est: 79, maxContext: 100, enabled: true, wantCompact: false},
		{name: "above threshold", est: 95, maxContext: 100, enabled: true, wantCompact: true},
		// A model without max_context_tokens measures against the default
		// window, the one GET /v1/models hands the web UI ring (#245).
		{name: "no max_context_tokens at the default window threshold", est: 102400, maxContext: 0, enabled: true, wantCompact: true},
		{name: "no max_context_tokens below the default window threshold", est: 102399, maxContext: 0, enabled: true, wantCompact: false},
		{name: "disabled", est: 1000, maxContext: 100, enabled: false, wantCompact: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := seededCompactState(t, 3)
			keep := 1
			comp := config.CompactionConfig{KeepRecentTurns: &keep}
			if !tc.enabled {
				off := false
				comp.Enabled = &off
			}
			provider := &compactCannedProvider{t: t, summary: "auto summary"}
			ag := compactTestAgent(t, st, comp, provider)
			ag.cfg.Models[0].MaxContextTokens = tc.maxContext
			st.SetLastContextBreakdown(&session.ContextBreakdown{EstimatedTotal: tc.est})

			got := ag.maybeAutoCompact(context.Background())
			if got != tc.wantCompact {
				t.Fatalf("maybeAutoCompact = %v, want %v", got, tc.wantCompact)
			}
			hasSummary := false
			for _, m := range st.GetMessages() {
				if m.CompactionSummary {
					hasSummary = true
				}
			}
			if hasSummary != tc.wantCompact {
				t.Fatalf("summary row present = %v, want %v", hasSummary, tc.wantCompact)
			}
		})
	}
}

func TestMaybeAutoCompactFailOpen(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	provider := &compactCannedProvider{t: t, err: errors.New("summarizer down")}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)
	ag.cfg.Models[0].MaxContextTokens = 100
	st.SetLastContextBreakdown(&session.ContextBreakdown{EstimatedTotal: 90})

	if ag.maybeAutoCompact(context.Background()) {
		t.Fatal("failed compaction must report false")
	}
	if len(st.GetMessages()) != 6 {
		t.Fatal("history must stay untouched on failure")
	}
}

// windowedState is a session whose manager resolved a context window from the
// provider's model listing (session.State.ContextWindow).
type windowedState struct {
	*session.State
	window int
}

func (w windowedState) ContextWindow(*config.Config) (int, string) {
	return w.window, session.ContextWindowFromProvider
}

func TestMaybeAutoCompactMeasuresAgainstTheSessionWindow(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	provider := &compactCannedProvider{t: t, summary: "auto summary"}
	// max_context_tokens stays unset: the window comes from the session, which
	// read it from the provider's listing.
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)
	ag.state = windowedState{State: st, window: 1000}
	st.SetLastContextBreakdown(&session.ContextBreakdown{EstimatedTotal: 800})

	if !ag.maybeAutoCompact(context.Background()) {
		t.Fatal("80% of the session's 1000-token window must compact")
	}
	if len(provider.requests) != 1 {
		t.Fatalf("summarizer called %d times, want 1", len(provider.requests))
	}
}

func TestMaybeAutoCompactLogsOnceWhenNothingCanBeFolded(t *testing.T) {
	st := &session.State{ID: "sess_compact_one_turn", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "the only prompt, still being answered"})
	var logs bytes.Buffer
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100, MaxContextTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return &compactCannedProvider{t: t, summary: "must not be asked"}, nil
	}
	st.SetLastContextBreakdown(&session.ContextBreakdown{EstimatedTotal: 95})

	for i := 0; i < 3; i++ {
		if ag.maybeAutoCompact(context.Background()) {
			t.Fatal("the prompt being answered must never be folded")
		}
	}
	if got := strings.Count(logs.String(), "auto-compaction skipped"); got != 1 {
		t.Fatalf("skip logged %d times in one turn, want once:\n%s", got, logs.String())
	}
	for _, m := range st.GetMessages() {
		if m.CompactionSummary {
			t.Fatal("no summary row expected")
		}
	}
}

func TestResumeAfterPermissionAutoCompactsBeforeFirstLLMCall(t *testing.T) {
	st := seededCompactState(t, 3)
	st.SessionDir = t.TempDir()
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "run the blocked command"})
	st.AddMessage(llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{
			ID:        "call_blocked_compact",
			Name:      "run_command",
			InputJSON: `{"command":"printf SHOULD_NOT_RUN"}`,
		}},
	})
	provider := &autoCompactRunProvider{}
	keep := 1
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		// Tiny window: the system prompt alone exceeds 80% of 50 tokens.
		Models:     []config.ModelEntry{{Model: "fake/model", MaxTokens: 100, MaxContextTokens: 50}},
		Agent:      config.Agent{Model: "fake/model"},
		Compaction: config.CompactionConfig{KeepRecentTurns: &keep},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}

	if _, err := ag.ResumeAfterPermission(context.Background(), "call_blocked_compact", &acp.PermissionResult{
		Outcome:  "cancelled",
		OptionID: "reject",
	}); err != nil {
		t.Fatal(err)
	}
	if len(provider.streamSeen) == 0 {
		t.Fatal("no stream request recorded")
	}
	sawSummary := false
	for _, m := range provider.streamSeen[0] {
		if m.Role == llm.RoleSystem {
			continue
		}
		if !m.CompactionSummary {
			t.Fatalf("first LLM request after the resume does not start from the summary: %+v", m)
		}
		sawSummary = true
		break
	}
	if !sawSummary {
		t.Fatal("summary missing from the first LLM request after the resume")
	}
}

// autoCompactRunProvider serves Complete (summary) and Stream (answer),
// recording stream requests so the test can assert the post-compaction window.
type autoCompactRunProvider struct {
	streamSeen [][]llm.Message
}

func (p *autoCompactRunProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "auto summary", StopReason: "end_turn"}, nil
}

func (p *autoCompactRunProvider) Stream(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.streamSeen = append(p.streamSeen, append([]llm.Message(nil), messages...))
	onChunk(llm.StreamChunk{TextDelta: "post-auto answer"})
	return &llm.Response{Content: "post-auto answer", StopReason: "end_turn"}, nil
}

func TestRunAutoCompactsBeforeFirstLLMCall(t *testing.T) {
	st := seededCompactState(t, 3)
	provider := &autoCompactRunProvider{}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		// Tiny window: the system prompt alone exceeds 80% of 50 tokens.
		Models: []config.ModelEntry{{Model: "fake/model", MaxTokens: 100, MaxContextTokens: 50}},
		Agent:  config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "continue please"}})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q", stop)
	}

	hasSummary := false
	for _, m := range st.GetMessages() {
		if m.CompactionSummary {
			hasSummary = true
		}
	}
	if !hasSummary {
		t.Fatal("auto-compaction did not insert a summary row")
	}
	if len(provider.streamSeen) == 0 {
		t.Fatal("no stream request recorded")
	}
	first := provider.streamSeen[0]
	sawSummary := false
	for _, m := range first {
		if m.Role == llm.RoleSystem {
			continue
		}
		if m.CompactionSummary {
			sawSummary = true
			break
		}
		// Any non-summary history message before the summary means the window
		// was not rebuilt after compaction.
		t.Fatalf("first LLM request does not start from the summary: %+v", m)
	}
	if !sawSummary {
		t.Fatal("summary missing from the first LLM request")
	}
}

// --- loop guard escalation and false-positive safety -----------------------
