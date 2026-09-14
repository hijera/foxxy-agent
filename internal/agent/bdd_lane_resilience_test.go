package agent

// Godog harness for features/llm_lane_resilience.feature. It drives the real
// ReAct loop against a provider that answers with reasoning and nothing else,
// which is how a sick member of a load-balanced group fails: the tool call is
// flattened into reasoning_content and its name is lost.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type laneFeatureState struct {
	t        *testing.T
	ag       *Agent
	provider laneProvider
	stop     string
	runErr   error
}

// laneProvider is what the two scripted providers in this suite share: the
// scenarios only ever ask how many requests arrived and what they carried.
type laneProvider interface {
	llm.Provider
	callCount() int
	request(n int) []llm.Message
}

func (p *emptyThenAnsweringProvider) callCount() int { return p.calls }

func (p *emptyThenAnsweringProvider) request(n int) []llm.Message {
	if n < 1 || n > len(p.seen) {
		return nil
	}
	return p.seen[n-1]
}

func (p *emptyThenNudgedProvider) callCount() int { return p.calls }

func (p *emptyThenNudgedProvider) request(n int) []llm.Message {
	if n < 1 || n > len(p.seen) {
		return nil
	}
	return p.seen[n-1]
}

func (s *laneFeatureState) reset(t *testing.T) {
	s.t = t
	s.ag = nil
	s.provider = nil
	s.stop = ""
	s.runErr = nil
}

func (s *laneFeatureState) build(p laneProvider, id string) error {
	s.provider = p
	s.ag = newLaneAgent(s.t, id, p)
	return nil
}

func (s *laneFeatureState) modelReasonsThenReplies() error {
	return s.build(&emptyThenAnsweringProvider{}, "sess_lane_recovers")
}

func (s *laneFeatureState) modelReasonsEveryTime() error {
	return s.build(&emptyThenNudgedProvider{}, "sess_lane_never_recovers")
}

func (s *laneFeatureState) operatorSendsPrompt() error {
	s.stop, s.runErr = s.ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "do the thing"}})
	return nil
}

func (s *laneFeatureState) laneReceivedTheRequestTwice() error {
	if got := s.provider.callCount(); got < 2 {
		return fmt.Errorf("the lane received %d requests, want at least 2", got)
	}
	first, second := s.provider.request(1), s.provider.request(2)
	if len(second) != len(first) {
		return fmt.Errorf("the second request carried %d messages, the first %d: it must be the identical request",
			len(second), len(first))
	}
	return nil
}

func (s *laneFeatureState) retryCarriesNoNudgeAndNoEmptyTurn() error {
	for _, m := range s.provider.request(2) {
		if strings.Contains(m.Content, emptyAssistantContinuationNudge) {
			return fmt.Errorf("the replay carried the nudge; words come only after a replay did not help")
		}
		if m.Role == llm.RoleAssistant && strings.TrimSpace(m.Content) == "" {
			return fmt.Errorf("the replay carried the empty assistant turn it is replacing")
		}
	}
	return nil
}

func (s *laneFeatureState) turnEndsWithTheAnswer() error {
	if s.runErr != nil {
		return fmt.Errorf("the turn failed: %w", s.runErr)
	}
	if s.stop != string(acp.StopReasonEndTurn) {
		return fmt.Errorf("stop reason = %q, want end_turn", s.stop)
	}
	for _, m := range s.ag.state.GetMessages() {
		if m.Role == llm.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return nil
		}
	}
	return fmt.Errorf("the transcript holds no answer")
}

func (s *laneFeatureState) transcriptHoldsEmptyTurnAndAnswer() error {
	var empty, answered int
	for _, m := range s.ag.state.GetMessages() {
		if m.Role != llm.RoleAssistant {
			continue
		}
		if strings.TrimSpace(m.Content) == "" {
			empty++
		} else {
			answered++
		}
	}
	if empty != 1 || answered != 1 {
		return fmt.Errorf("the transcript holds %d empty and %d answered assistant turns, want 1 and 1", empty, answered)
	}
	return nil
}

func (s *laneFeatureState) thirdRequestNamesTheEmptyTurn() error {
	third := s.provider.request(3)
	if len(third) == 0 {
		return fmt.Errorf("there was no third request")
	}
	for _, m := range third {
		if strings.Contains(m.Content, emptyAssistantContinuationNudge) {
			return nil
		}
	}
	return fmt.Errorf("the third request does not tell the model its previous message had no answer")
}

func initializeLaneResilienceScenario(t *testing.T, sc *godog.ScenarioContext) {
	s := &laneFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		s.reset(t)
		return ctx, nil
	})

	sc.Step(`^a model that answers with reasoning only on its first call and then replies$`, s.modelReasonsThenReplies)
	sc.Step(`^a model that answers with reasoning only on every call$`, s.modelReasonsEveryTime)
	sc.Step(`^the operator sends a prompt on the lane$`, s.operatorSendsPrompt)
	sc.Step(`^the lane receives the same request a second time$`, s.laneReceivedTheRequestTwice)
	sc.Step(`^the retried request carries no nudge and no empty assistant turn$`, s.retryCarriesNoNudgeAndNoEmptyTurn)
	sc.Step(`^the turn ends with the model's answer$`, s.turnEndsWithTheAnswer)
	sc.Step(`^the transcript holds the empty turn as well as the answer$`, s.transcriptHoldsEmptyTurnAndAnswer)
	sc.Step(`^the third request tells the model its previous message had no answer$`, s.thirdRequestNamesTheEmptyTurn)
}

func TestLaneResilienceFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "llm-lane-resilience",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			initializeLaneResilienceScenario(t, sc)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/llm_lane_resilience.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("llm_lane_resilience.feature failed")
	}
}
