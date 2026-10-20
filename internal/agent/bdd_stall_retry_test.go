package agent

// Godog harness for features/llm_stall_retry.feature. It drives the real ReAct
// loop against the fake provider from react_stall_test.go, with the guards tuned
// to milliseconds so the minute-scale schedule is exercised without waiting it
// out: the delays are configuration, so the test needs no clock seam.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type stallFeatureState struct {
	t        *testing.T
	harness  *stallHarness
	provider *stallProvider
	tune     func(*config.Agent)
	stop     string
	runErr   error
}

func (s *stallFeatureState) reset(t *testing.T) {
	s.t = t
	s.harness = nil
	s.provider = nil
	s.tune = nil
	s.stop = ""
	s.runErr = nil
}

func (s *stallFeatureState) build(script []stallBehaviour) error {
	s.provider = &stallProvider{script: script}
	s.harness = newStallHarness(s.t, s.provider, s.tune)
	return nil
}

func (s *stallFeatureState) modelAnswersNothingThenReplies() error {
	return s.build([]stallBehaviour{
		{silent: true},
		{answer: "Here is the answer."},
	})
}

// modelAnswersNothingTwiceThenReplies spends the free immediate re-issue on the
// second silence, so the scenario reaches the waiting ladder behind it.
func (s *stallFeatureState) modelAnswersNothingTwiceThenReplies() error {
	return s.build([]stallBehaviour{
		{silent: true},
		{silent: true},
		{answer: "Here is the answer."},
	})
}

func (s *stallFeatureState) modelDiesWithTransportErrorThenReplies() error {
	return s.build([]stallBehaviour{
		{err: fmt.Errorf("openai stream: Post \"https://api.example/v1/chat/completions\": unexpected EOF")},
		{answer: "Here is the answer."},
	})
}

func (s *stallFeatureState) modelStopsMidAnswerThenFinishes() error {
	return s.build([]stallBehaviour{
		{partial: "The first half of the answer."},
		{answer: " And the rest."},
	})
}

func (s *stallFeatureState) modelWritesOneLongToolCall() error {
	// Twenty silent frames at 10ms each is 200ms of stream against a 60ms guard:
	// only the progress signal keeps this alive.
	return s.build([]stallBehaviour{
		{progressOnly: 20, answer: "Tool call finished."},
	})
}

func (s *stallFeatureState) turnBudgetAllowsOneStep() error {
	// Applied before the agent is built, so re-build with the tighter budget.
	s.tune = func(c *config.Agent) { c.MaxTurns = 1 }
	if s.provider == nil {
		return fmt.Errorf("no model configured yet")
	}
	s.harness = newStallHarness(s.t, s.provider, s.tune)
	return nil
}

func (s *stallFeatureState) operatorSendsPrompt() error {
	if s.harness == nil {
		return fmt.Errorf("no agent built")
	}
	s.stop, s.runErr = s.harness.run(s.t)
	return nil
}

func (s *stallFeatureState) turnCompletesWithoutSystemError() error {
	if s.runErr != nil {
		return fmt.Errorf("the turn failed instead of recovering: %v", s.runErr)
	}
	if s.stop != string(acp.StopReasonEndTurn) {
		return fmt.Errorf("stop reason %q, want end_turn", s.stop)
	}
	return nil
}

func (s *stallFeatureState) modelWasCalled(n int) error {
	if got := s.provider.callCount(); got != n {
		return fmt.Errorf("the model was called %d times, want %d", got, n)
	}
	return nil
}

// waitsAnnounced is how many times the operator was told the turn is waiting on
// the provider, which is what separates the free immediate re-issue from the
// ladder behind it.
func (s *stallFeatureState) waitsAnnounced(n int) error {
	if got := s.harness.sender.waitingPhases(); got != n {
		return fmt.Errorf("the operator was told to wait %d times, want %d", got, n)
	}
	return nil
}

func (s *stallFeatureState) transcriptHoldsOneAnswer() error {
	msgs := s.harness.assistantMessages()
	if len(msgs) != 1 {
		return fmt.Errorf("the transcript holds %d answers, want 1", len(msgs))
	}
	return nil
}

func (s *stallFeatureState) partialAnswerSurvives() error {
	for _, m := range s.harness.assistantMessages() {
		if strings.Contains(m.Content, "The first half of the answer.") {
			if len(m.ToolCalls) != 0 {
				return fmt.Errorf("the kept partial message carries tool calls cut mid-write: %+v", m.ToolCalls)
			}
			return nil
		}
	}
	return fmt.Errorf("the text the user already watched arrive was lost")
}

func (s *stallFeatureState) modelStallsOnTheSameOpeningEveryTime() error {
	// One behaviour, repeated for every call: the hub drops the answer at the same
	// place each time, and the model starts it over each time.
	return s.build([]stallBehaviour{{partial: "I will fix the compile errors. First the constants."}})
}

func (s *stallFeatureState) modelStallsAfterThinkingThenAnswers() error {
	return s.build([]stallBehaviour{
		{partialReason: "Let me work out which constants are wrong."},
		{answer: "The constants are fixed."},
	})
}

func (s *stallFeatureState) firstContinuationAsksToCarryOn() error {
	if !hasNudge(s.provider.request(2), "cut off part-way through") {
		return fmt.Errorf("the first continuation did not ask the model to carry on")
	}
	return nil
}

func (s *stallFeatureState) secondContinuationNamesTheRepeat() error {
	if !hasNudge(s.provider.request(3), "begins the same way") {
		return fmt.Errorf("the second continuation did not tell the model it was repeating itself")
	}
	return nil
}

func (s *stallFeatureState) nudgesStayOutOfTranscript() error {
	for _, m := range s.harness.st.GetMessages() {
		for _, leak := range []string{"cut off part-way through", "begins the same way", "only your internal reasoning"} {
			if strings.Contains(m.Content, leak) {
				return fmt.Errorf("a nudge leaked into the persisted transcript: %q", leak)
			}
		}
	}
	return nil
}

func (s *stallFeatureState) turnStopsNamingTheRestarts() error {
	if s.runErr == nil {
		return fmt.Errorf("the turn did not stop when the model kept restarting")
	}
	if !strings.Contains(s.runErr.Error(), "restarted the same answer") {
		return fmt.Errorf("the notice %q does not name the restarts", s.runErr)
	}
	return nil
}

func (s *stallFeatureState) continuationDoesNotPointAtEmptyMessage() error {
	if hasNudge(s.provider.request(2), "Continue from exactly where it stops") {
		return fmt.Errorf("the model was asked to continue from a message with no text in it")
	}
	if !hasNudge(s.provider.request(2), "only your internal reasoning") {
		return fmt.Errorf("the continuation did not say what was actually lost")
	}
	return nil
}

func (s *stallFeatureState) secondRequestAskedToContinue() error {
	for _, m := range s.provider.lastMessages() {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, "cut off part-way through") {
			return nil
		}
	}
	return fmt.Errorf("the continuation request carried no nudge")
}

// A restarted app finds this on disk: a turn that produced one answer twice and
// never finished. Nothing else about that loop survived the process.
func (s *stallFeatureState) sessionWithARepeatingPreviousTurn() error {
	if err := s.build([]stallBehaviour{{answer: "The constants are fixed."}}); err != nil {
		return err
	}
	for _, m := range []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compile errors"},
		{Role: llm.RoleAssistant, Content: "I will fix the compile errors. The problems are the wrong constant names (CODE_FIELD).",
			ToolCalls: []llm.ToolCall{{ID: "c1", Name: "read", InputJSON: `{"path":"Mapper.java"}`}}},
		{Role: llm.RoleTool, ToolCallID: "c1", Content: "package app;"},
		{Role: llm.RoleAssistant, Content: "I will fix the compile errors. The problems are the wrong constant names (ATS_CODE_FIELD, ATS_ID_FIELD)."},
	} {
		s.harness.st.AddMessage(m)
	}
	return nil
}

func (s *stallFeatureState) firstRequestNamesThePreviousRepeat() error {
	if !hasNudge(s.provider.request(1), "previous turn was cut off") {
		return fmt.Errorf("the first request did not tell the model its previous turn had been repeating")
	}
	return nil
}

func (s *stallFeatureState) firstRequestNamesTheStepsAlreadyRun() error {
	if !hasNudge(s.provider.request(1), "read(Mapper.java)") {
		return fmt.Errorf("the first request did not name the step that already ran")
	}
	return nil
}

func initializeStallRetryScenario(t *testing.T, sc *godog.ScenarioContext) {
	s := &stallFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		s.reset(t)
		return ctx, nil
	})

	sc.Step(`^a model that answers nothing on its first call and then replies$`, s.modelAnswersNothingThenReplies)
	sc.Step(`^a model that answers nothing on its first two calls and then replies$`, s.modelAnswersNothingTwiceThenReplies)
	sc.Step(`^a model whose first call dies with a transport error and then replies$`, s.modelDiesWithTransportErrorThenReplies)
	sc.Step(`^a model that stops mid-answer on its first call and then finishes$`, s.modelStopsMidAnswerThenFinishes)
	sc.Step(`^a model that spends longer than the stall guard writing one tool call$`, s.modelWritesOneLongToolCall)
	sc.Step(`^the turn budget allows only one reasoning step$`, s.turnBudgetAllowsOneStep)
	sc.Step(`^the operator sends a prompt$`, s.operatorSendsPrompt)
	sc.Step(`^the turn completes without a system error$`, s.turnCompletesWithoutSystemError)
	sc.Step(`^the model was called once$`, func() error { return s.modelWasCalled(1) })
	sc.Step(`^the model was called twice$`, func() error { return s.modelWasCalled(2) })
	sc.Step(`^the model was called three times$`, func() error { return s.modelWasCalled(3) })
	sc.Step(`^the operator was never told to wait$`, func() error { return s.waitsAnnounced(0) })
	sc.Step(`^the operator was told to wait once$`, func() error { return s.waitsAnnounced(1) })
	sc.Step(`^the transcript holds exactly one answer$`, s.transcriptHoldsOneAnswer)
	sc.Step(`^the partial answer survives in the transcript$`, s.partialAnswerSurvives)
	sc.Step(`^the second request asked the model to continue$`, s.secondRequestAskedToContinue)
	sc.Step(`^a model that stalls on the same opening every time$`, s.modelStallsOnTheSameOpeningEveryTime)
	sc.Step(`^a model that stalls after thinking but before writing, then answers$`, s.modelStallsAfterThinkingThenAnswers)
	sc.Step(`^the first continuation asks the model to carry on$`, s.firstContinuationAsksToCarryOn)
	sc.Step(`^the second continuation tells the model it is repeating itself$`, s.secondContinuationNamesTheRepeat)
	sc.Step(`^the continuation nudges stay out of the transcript$`, s.nudgesStayOutOfTranscript)
	sc.Step(`^the turn stops with a notice naming the restarts$`, s.turnStopsNamingTheRestarts)
	sc.Step(`^the continuation does not point at an empty message$`, s.continuationDoesNotPointAtEmptyMessage)
	sc.Step(`^a session whose previous turn ended writing the same answer twice$`, s.sessionWithARepeatingPreviousTurn)
	sc.Step(`^the first request tells the model its previous turn was repeating$`, s.firstRequestNamesThePreviousRepeat)
	sc.Step(`^the first request names the steps that already ran$`, s.firstRequestNamesTheStepsAlreadyRun)
}

func TestStallRetryFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "llm-stall-retry",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			initializeStallRetryScenario(t, sc)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/llm_stall_retry.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("llm stall retry feature suite failed")
	}
}
