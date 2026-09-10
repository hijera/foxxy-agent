package agent

// Godog harness for features/loop_protection.feature: drives the real Agent.Run
// with a fake LLM provider that simulates each way a turn can run away — a
// degenerating answer stream, a degenerating reasoning channel, and the same
// tool call requested forever. Real models cannot be made to degenerate on
// demand, so simulation is what makes this spec deterministic and LLM-free.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// bddLoopedSentence is the passage the simulated model gets stuck on, taken from
// the report that motivated the guard.
const bddLoopedSentence = "Still writing validation logic... "

// bddLoopedThought is the reasoning-channel equivalent.
const bddLoopedThought = "I should re-check the validation rules once more, then decide. "

// bddLoopProvider simulates a model that degenerates. It streams the looping
// passage until the agent cancels the stream (exactly what a real provider does:
// it stops on ctx and returns what it produced together with context.Canceled),
// then answers normally so recovery after a nudge is observable.
type bddLoopProvider struct {
	// channel selects which streamed channel degenerates.
	channel loopAbortChannel
	// toolCall, when set, is requested on every turn instead of streaming text.
	toolCall *llm.ToolCall
	// rotating, when set, is cycled through one call per turn. swapAt replaces the
	// call at that position with a foreign one, so the cycle is imperfect - the
	// shape a real re-read loop takes, and the one an exact-match test misses.
	rotating []llm.ToolCall
	swapAt   int
	// answerAfter makes a rotating model give up on tools and answer, which is
	// what one does after the guard has taken the loop away.
	answerAfter int
	// toollessCalls counts requests that arrived with no tool definitions at all.
	toollessCalls int
	// recoverAfter is how many degenerate turns precede the real answer.
	recoverAfter int

	calls      int
	seen       [][]llm.Message
	cancelled  int
	maxDeltas  int
	realAnswer string
}

func (p *bddLoopProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Complete must not be used by the loop guard suite")
}

func (p *bddLoopProvider) Stream(ctx context.Context, messages []llm.Message, defs []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	p.seen = append(p.seen, append([]llm.Message(nil), messages...))

	if len(p.rotating) > 0 && len(defs) == 0 {
		// The guard withheld the tools: there is nothing left to do but answer.
		p.toollessCalls++
		onChunk(llm.StreamChunk{TextDelta: p.realAnswer})
		return &llm.Response{Content: p.realAnswer, StopReason: "end_turn"}, nil
	}
	if len(p.rotating) > 0 && p.answerAfter > 0 && p.calls > p.answerAfter {
		onChunk(llm.StreamChunk{TextDelta: p.realAnswer})
		return &llm.Response{Content: p.realAnswer, StopReason: "end_turn"}, nil
	}
	if len(p.rotating) > 0 {
		tc := p.rotating[(p.calls-1)%len(p.rotating)]
		if p.calls-1 == p.swapAt {
			tc = llm.ToolCall{Name: "glob", InputJSON: `{"pattern":"**/*odd.go"}`}
		}
		tc.ID = fmt.Sprintf("call_rot_%d", p.calls)
		onChunk(llm.StreamChunk{ToolCall: &tc})
		return &llm.Response{ToolCalls: []llm.ToolCall{tc}, StopReason: "tool_use"}, nil
	}

	if p.toolCall != nil {
		tc := *p.toolCall
		onChunk(llm.StreamChunk{ToolCall: &tc})
		return &llm.Response{ToolCalls: []llm.ToolCall{tc}, StopReason: "tool_use"}, nil
	}

	if p.calls > p.recoverAfter {
		onChunk(llm.StreamChunk{TextDelta: p.realAnswer})
		return &llm.Response{Content: p.realAnswer, StopReason: "end_turn"}, nil
	}

	unit := bddLoopedSentence
	if p.channel == loopAbortReasoning {
		unit = bddLoopedThought
	}
	var produced strings.Builder
	limit := p.maxDeltas
	if limit <= 0 {
		limit = 200
	}
	for i := 0; i < limit; i++ {
		if ctx.Err() != nil {
			p.cancelled++
			break
		}
		if p.channel == loopAbortReasoning {
			onChunk(llm.StreamChunk{ReasoningDelta: unit})
		} else {
			produced.WriteString(unit)
			onChunk(llm.StreamChunk{TextDelta: unit})
		}
	}
	if ctx.Err() == nil {
		// The guard never fired: report a clean finish so the assertion that the
		// stream was cut fails loudly instead of hanging.
		return &llm.Response{Content: produced.String(), StopReason: "end_turn"}, nil
	}
	return &llm.Response{Content: produced.String(), StopReason: "tool_use"}, context.Canceled
}

type loopGuardSender struct {
	resumePermissionSender
	updates []interface{}
}

func (s *loopGuardSender) SendSessionUpdate(_ string, update interface{}) error {
	s.updates = append(s.updates, update)
	return nil
}

type loopGuardFeatureState struct {
	tmpDirs  []string
	st       *session.State
	ag       *Agent
	provider *bddLoopProvider
	sender   *loopGuardSender
	cfg      *config.Config

	stop    string
	runErr  error
	toolRan int
}

func (s *loopGuardFeatureState) reset() error {
	s.close()
	s.sender = &loopGuardSender{}
	s.provider = nil
	s.stop = ""
	s.runErr = nil
	s.toolRan = 0
	return nil
}

func (s *loopGuardFeatureState) close() {
	for _, d := range s.tmpDirs {
		_ = os.RemoveAll(d)
	}
	s.tmpDirs = nil
	s.st = nil
	s.ag = nil
	s.cfg = nil
}

func (s *loopGuardFeatureState) tempDir() (string, error) {
	d, err := os.MkdirTemp("", "foxxycode-bdd-loop-*")
	if err != nil {
		return "", err
	}
	s.tmpDirs = append(s.tmpDirs, d)
	return d, nil
}

func (s *loopGuardFeatureState) agentWithLoopGuard() error {
	cwd, err := s.tempDir()
	if err != nil {
		return err
	}
	sessionDir, err := s.tempDir()
	if err != nil {
		return err
	}
	s.st = &session.State{
		ID:         "sess_bdd_loop_guard",
		CWD:        cwd,
		Mode:       session.ModeAgent,
		SessionDir: sessionDir,
	}
	s.cfg = &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 24},
	}
	return nil
}

func (s *loopGuardFeatureState) agentWithLoopGuardStopping() error {
	if err := s.agentWithLoopGuard(); err != nil {
		return err
	}
	s.cfg.Agent.LoopStuckAction = config.AgentLoopStuckActionStop
	return nil
}

func (s *loopGuardFeatureState) buildAgent() {
	s.ag = NewAgent(s.cfg, s.st, s.sender, nil)
	s.ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return s.provider, nil
	}
}

func (s *loopGuardFeatureState) modelRepeatsSentenceThenAnswers() error {
	s.provider = &bddLoopProvider{
		channel:      loopAbortText,
		recoverAfter: 1,
		realAnswer:   "Validation is in place; here is the summary.",
	}
	s.buildAgent()
	return nil
}

func (s *loopGuardFeatureState) modelRepeatsThoughtThenAnswers() error {
	s.provider = &bddLoopProvider{
		channel:      loopAbortReasoning,
		recoverAfter: 1,
		realAnswer:   "Done deliberating; here is the answer.",
	}
	s.buildAgent()
	return nil
}

func (s *loopGuardFeatureState) modelAlwaysRequestsTheSameToolCall() error {
	s.provider = &bddLoopProvider{
		toolCall: &llm.ToolCall{ID: "call_loop", Name: "glob", InputJSON: `{"pattern":"**/*.go"}`},
	}
	s.buildAgent()
	return nil
}

func (s *loopGuardFeatureState) modelCyclesThroughToolCalls() error {
	s.provider = &bddLoopProvider{
		rotating: []llm.ToolCall{
			{Name: "glob", InputJSON: `{"pattern":"**/*a.go"}`},
			{Name: "glob", InputJSON: `{"pattern":"**/*b.go"}`},
			{Name: "glob", InputJSON: `{"pattern":"**/*c.go"}`},
		},
		// The seventh call breaks the rotation, so no exact repetition of the unit
		// spans the tail the detector examines.
		swapAt: 6,
		// What it says once the guard withholds the tools. This model never gives
		// up on the loop by itself, so this is the only way it ever answers.
		realAnswer: "I could not finish the search; here is what I have.",
	}
	s.buildAgent()
	return nil
}

func (s *loopGuardFeatureState) modelCyclesThenAnswers() error {
	s.provider = &bddLoopProvider{
		rotating: []llm.ToolCall{
			{Name: "glob", InputJSON: `{"pattern":"**/*a.go"}`},
			{Name: "glob", InputJSON: `{"pattern":"**/*b.go"}`},
			{Name: "glob", InputJSON: `{"pattern":"**/*c.go"}`},
		},
		swapAt:      6,
		answerAfter: 11,
		realAnswer:  "Three packages match; here is the summary.",
	}
	s.buildAgent()
	return nil
}

func (s *loopGuardFeatureState) loopingCallsStopBeingExecuted() error {
	for _, m := range s.st.GetMessages() {
		if m.Role == llm.RoleTool && m.Content == toolQuarantinedResult {
			return nil
		}
	}
	return fmt.Errorf("the guard never took the looping calls away")
}

func (s *loopGuardFeatureState) modelAskedForAnAnswerWithoutTools() error {
	if s.provider.toollessCalls == 0 {
		return fmt.Errorf("the tools were never withheld, so the model was never made to answer")
	}
	return nil
}

func (s *loopGuardFeatureState) turnEndsWithoutAnError() error {
	if s.runErr != nil {
		return fmt.Errorf("turn ended with an error instead of an answer: %v", s.runErr)
	}
	if s.stop != string(acp.StopReasonEndTurn) {
		return fmt.Errorf("stop reason = %q, want end_turn", s.stop)
	}
	return nil
}

func (s *loopGuardFeatureState) noticeNamesARepeatingSequence() error {
	if s.runErr == nil {
		return fmt.Errorf("turn ended without a notice; stop reason %q", s.stop)
	}
	if s.runErr.Error() != toolCycleStopNotice {
		return fmt.Errorf("notice = %q, want the sequence notice %q", s.runErr, toolCycleStopNotice)
	}
	return nil
}

func (s *loopGuardFeatureState) userSendsPrompt() error {
	if s.ag == nil {
		return fmt.Errorf("no agent prepared")
	}
	s.stop, s.runErr = s.ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "add the validation"}})
	return nil
}

func (s *loopGuardFeatureState) streamWasCut() error {
	if s.provider.cancelled == 0 {
		return fmt.Errorf("the model streamed to its own end: the loop guard never cancelled it")
	}
	return nil
}

// transcriptText concatenates the field of interest across assistant messages.
func (s *loopGuardFeatureState) transcriptText(reasoning bool) string {
	var b strings.Builder
	for _, m := range s.st.GetMessages() {
		if m.Role != llm.RoleAssistant {
			continue
		}
		if reasoning {
			b.WriteString(m.Reasoning)
		} else {
			b.WriteString(m.Content)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (s *loopGuardFeatureState) transcriptKeepsAnswerWithoutRepeatedTail() error {
	got := s.transcriptText(false)
	if n := strings.Count(got, strings.TrimSpace(bddLoopedSentence)); n != 1 {
		return fmt.Errorf("looped passage kept %d times in the transcript, want exactly 1", n)
	}
	if !strings.Contains(got, loopGuardTruncationMarker) {
		return fmt.Errorf("transcript has no truncation marker: %q", got)
	}
	return nil
}

func (s *loopGuardFeatureState) transcriptKeepsReasoningWithoutRepeatedTail() error {
	got := s.transcriptText(true)
	if n := strings.Count(got, strings.TrimSpace(bddLoopedThought)); n != 1 {
		return fmt.Errorf("looped thought kept %d times in the transcript, want exactly 1", n)
	}
	return nil
}

func (s *loopGuardFeatureState) modelNudgedOnce() error {
	if len(s.provider.seen) < 2 {
		return fmt.Errorf("the model was never re-prompted after the loop was cut")
	}
	last := s.provider.seen[len(s.provider.seen)-1]
	for _, m := range last {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, "repeating the same passage") {
			return nil
		}
	}
	return fmt.Errorf("no loop nudge found in the re-prompt: %#v", last)
}

func (s *loopGuardFeatureState) nudgedRequestCarriesNoRepeatedPassage() error {
	last := s.provider.seen[len(s.provider.seen)-1]
	for _, m := range last {
		if strings.Count(m.Content, strings.TrimSpace(bddLoopedSentence)) > 1 {
			return fmt.Errorf("the repeated passage was replayed to the model: %q", m.Content)
		}
	}
	return nil
}

func (s *loopGuardFeatureState) turnEndsWithRealAnswer() error {
	if s.runErr != nil {
		return fmt.Errorf("turn failed instead of recovering: %v", s.runErr)
	}
	if s.stop != string(acp.StopReasonEndTurn) {
		return fmt.Errorf("stop reason = %q, want end_turn", s.stop)
	}
	msgs := s.st.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, s.provider.realAnswer) {
		return fmt.Errorf("final answer missing: %+v", last)
	}
	return nil
}

func (s *loopGuardFeatureState) toolRanFewerTimesThanTheLimit() error {
	limit := s.cfg.Agent.EffectiveLoopToolRepeatLimit()
	executed := 0
	for _, m := range s.st.GetMessages() {
		if m.Role == llm.RoleTool && m.Content != toolLoopNudge && m.Content != toolLoopSkippedResult {
			executed++
		}
	}
	if executed >= limit {
		return fmt.Errorf("tool executed %d times, want fewer than the repeat limit %d", executed, limit)
	}
	return nil
}

func (s *loopGuardFeatureState) everyToolCallHasAResult() error {
	answered := map[string]bool{}
	requested := 0
	for _, m := range s.st.GetMessages() {
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			answered[m.ToolCallID] = true
		}
	}
	for _, m := range s.st.GetMessages() {
		if m.Role != llm.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			requested++
			if !answered[tc.ID] {
				return fmt.Errorf("tool call %q was announced but never answered", tc.ID)
			}
		}
	}
	if requested == 0 {
		return fmt.Errorf("no tool calls were requested at all")
	}
	return nil
}

func (s *loopGuardFeatureState) turnStopsWithLoopNoticeBeforeMaxTurns() error {
	if s.runErr == nil {
		return fmt.Errorf("turn ended without a notice; stop reason %q", s.stop)
	}
	if !strings.Contains(s.runErr.Error(), "same") {
		return fmt.Errorf("error does not explain the loop: %v", s.runErr)
	}
	if s.stop != string(acp.StopReasonRefused) {
		return fmt.Errorf("stop reason = %q, want agent_refused", s.stop)
	}
	if s.provider.calls >= s.cfg.Agent.MaxTurns {
		return fmt.Errorf("the loop ran to max_turns (%d calls); the guard should stop it earlier", s.provider.calls)
	}
	return nil
}

func initializeLoopGuardScenario(sc *godog.ScenarioContext) {
	s := &loopGuardFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a foxxycode agent with the loop guard enabled$`, s.agentWithLoopGuard)
	sc.Step(`^a model that repeats the same sentence forever, then answers normally$`, s.modelRepeatsSentenceThenAnswers)
	sc.Step(`^a model that repeats the same thought forever with no answer text, then answers normally$`, s.modelRepeatsThoughtThenAnswers)
	sc.Step(`^a model that always requests the same tool call with the same arguments$`, s.modelAlwaysRequestsTheSameToolCall)
	sc.Step(`^the user sends a prompt$`, s.userSendsPrompt)
	sc.Step(`^the streamed response is cut before the model stops on its own$`, s.streamWasCut)
	sc.Step(`^the stored transcript keeps the answer without the repeated tail$`, s.transcriptKeepsAnswerWithoutRepeatedTail)
	sc.Step(`^the stored transcript keeps the reasoning without the repeated tail$`, s.transcriptKeepsReasoningWithoutRepeatedTail)
	sc.Step(`^the model is nudged once to stop repeating itself$`, s.modelNudgedOnce)
	sc.Step(`^the nudged request carries no repeated passage$`, s.nudgedRequestCarriesNoRepeatedPassage)
	sc.Step(`^the turn ends with the model's real answer$`, s.turnEndsWithRealAnswer)
	sc.Step(`^the tool is executed fewer times than the repeat limit allows$`, s.toolRanFewerTimesThanTheLimit)
	sc.Step(`^every requested tool call has a result recorded$`, s.everyToolCallHasAResult)
	sc.Step(`^the turn stops with a loop notice before max turns is reached$`, s.turnStopsWithLoopNoticeBeforeMaxTurns)
	sc.Step(`^a model that cycles through the same three tool calls, varying one of them$`, s.modelCyclesThroughToolCalls)
	sc.Step(`^the notice names a repeating sequence rather than one repeated call$`, s.noticeNamesARepeatingSequence)
	sc.Step(`^a foxxycode agent whose loop guard is set to stop the turn$`, s.agentWithLoopGuardStopping)
	sc.Step(`^a model that cycles through the same three tool calls, then answers once they stop running$`, s.modelCyclesThenAnswers)
	sc.Step(`^the looping calls stop being executed$`, s.loopingCallsStopBeingExecuted)
	sc.Step(`^the model is asked for an answer with the tools withheld$`, s.modelAskedForAnAnswerWithoutTools)
	sc.Step(`^the turn ends without an error$`, s.turnEndsWithoutAnError)
}

func TestLoopProtectionFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "loop-protection",
		ScenarioInitializer: initializeLoopGuardScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/loop_protection.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("loop protection feature suite failed")
	}
}
