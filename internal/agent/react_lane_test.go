package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// emptyThenNudgedProvider answers with reasoning and nothing else, and records
// what it was sent so a test can tell the plain replay from the nudge that
// follows it.
type emptyThenNudgedProvider struct {
	calls int
	seen  [][]llm.Message
}

func (p *emptyThenNudgedProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *emptyThenNudgedProvider) Stream(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	p.seen = append(p.seen, append([]llm.Message(nil), messages...))
	onChunk(llm.StreamChunk{ReasoningDelta: `We should read the file.{"path":"main.go"}`})
	return &llm.Response{Content: "", StopReason: "end_turn"}, nil
}

// newLaneAgent builds an agent whose only moving part is the provider: no title
// pass to spend an extra scripted answer, and no stall ladder, so a test that
// counts calls counts only the recoveries it is about.
func newLaneAgent(t *testing.T, id string, provider llm.Provider) *Agent {
	t.Helper()
	st := &session.State{
		ID:         id,
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 20, LLMStallRetry: new(bool)},
		Title:     config.TitleConfig{Enabled: new(bool)},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	return ag
}

// TestEmptyTurnNudgesOnlyAfterTheReplayDidNotHelp fixes the order of the two
// recoveries: the identical request goes out first (a sick deployment answers
// differently on the next draw), and only then is the model argued with. It also
// pins the total, so neither recovery is skipped or doubled.
func TestEmptyTurnNudgesOnlyAfterTheReplayDidNotHelp(t *testing.T) {
	provider := &emptyThenNudgedProvider{}
	ag := newLaneAgent(t, "sess_empty_then_nudge", provider)

	// The fork ends such a turn with a notice rather than an error, so what is
	// pinned here is the recovery budget, not the wording of the give-up.
	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "do the thing"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if want := 1 + maxEmptyAssistantReissues + maxEmptyAssistantContinuations; provider.calls != want {
		t.Fatalf("provider called %d times, want %d (first attempt, replays, nudges)", provider.calls, want)
	}
	carriesNudge := func(msgs []llm.Message) bool {
		for _, m := range msgs {
			if strings.Contains(m.Content, emptyAssistantContinuationNudge) {
				return true
			}
		}
		return false
	}
	if carriesNudge(provider.seen[1]) {
		t.Fatal("the first recovery was a nudge; the plain replay must come first")
	}
	if len(provider.seen[1]) != len(provider.seen[0]) {
		t.Fatalf("the replay is not the original request: %d messages vs %d",
			len(provider.seen[1]), len(provider.seen[0]))
	}
	if !carriesNudge(provider.seen[2]) {
		t.Fatal("the second recovery is not the nudge")
	}
}

// emptyThenAnsweringProvider is the lane this recovery exists for: the first
// member loses the tool call into the reasoning channel, the next one answers.
type emptyThenAnsweringProvider struct {
	calls int
	seen  [][]llm.Message
}

func (p *emptyThenAnsweringProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *emptyThenAnsweringProvider) Stream(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	p.seen = append(p.seen, append([]llm.Message(nil), messages...))
	if p.calls == 1 {
		onChunk(llm.StreamChunk{ReasoningDelta: "Thinking about it."})
		return &llm.Response{Content: "", StopReason: "end_turn"}, nil
	}
	onChunk(llm.StreamChunk{TextDelta: "Here is the answer."})
	return &llm.Response{Content: "Here is the answer.", StopReason: "end_turn"}, nil
}

// TestEmptyTurnReplayRecoversWithoutNudgingTheModel is the payoff: one replay,
// no words spent on the model, and the turn ends with the answer. The empty turn
// stays in the transcript because the user watched its reasoning stream in, but
// the request that went out again must not carry it.
func TestEmptyTurnReplayRecoversWithoutNudgingTheModel(t *testing.T) {
	provider := &emptyThenAnsweringProvider{}
	ag := newLaneAgent(t, "sess_empty_then_answer", provider)

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "do the thing"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if provider.calls != 2 {
		t.Fatalf("provider called %d times, want 2 (the empty turn and its replay)", provider.calls)
	}
	for _, m := range provider.seen[1] {
		if strings.Contains(m.Content, emptyAssistantContinuationNudge) {
			t.Fatal("the replay carried the nudge; words come only after a replay did not help")
		}
	}
	if len(provider.seen[1]) != len(provider.seen[0]) {
		t.Fatalf("the replay is not the identical request: %d messages vs %d",
			len(provider.seen[1]), len(provider.seen[0]))
	}
	// The transcript keeps the empty turn: the user watched its reasoning arrive.
	var assistants int
	for _, m := range ag.state.GetMessages() {
		if m.Role == llm.RoleAssistant {
			assistants++
		}
	}
	if assistants != 2 {
		t.Errorf("transcript holds %d assistant messages, want 2 (the empty turn and the answer)", assistants)
	}
}
