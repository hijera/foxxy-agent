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

func (s *stallFeatureState) secondRequestAskedToContinue() error {
	for _, m := range s.provider.lastMessages() {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, "cut off part-way through") {
			return nil
		}
	}
	return fmt.Errorf("the continuation request carried no nudge")
}

func initializeStallRetryScenario(t *testing.T, sc *godog.ScenarioContext) {
	s := &stallFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		s.reset(t)
		return ctx, nil
	})

	sc.Step(`^a model that answers nothing on its first call and then replies$`, s.modelAnswersNothingThenReplies)
	sc.Step(`^a model whose first call dies with a transport error and then replies$`, s.modelDiesWithTransportErrorThenReplies)
	sc.Step(`^a model that stops mid-answer on its first call and then finishes$`, s.modelStopsMidAnswerThenFinishes)
	sc.Step(`^a model that spends longer than the stall guard writing one tool call$`, s.modelWritesOneLongToolCall)
	sc.Step(`^the turn budget allows only one reasoning step$`, s.turnBudgetAllowsOneStep)
	sc.Step(`^the operator sends a prompt$`, s.operatorSendsPrompt)
	sc.Step(`^the turn completes without a system error$`, s.turnCompletesWithoutSystemError)
	sc.Step(`^the model was called once$`, func() error { return s.modelWasCalled(1) })
	sc.Step(`^the model was called twice$`, func() error { return s.modelWasCalled(2) })
	sc.Step(`^the transcript holds exactly one answer$`, s.transcriptHoldsOneAnswer)
	sc.Step(`^the partial answer survives in the transcript$`, s.partialAnswerSurvives)
	sc.Step(`^the second request asked the model to continue$`, s.secondRequestAskedToContinue)
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
