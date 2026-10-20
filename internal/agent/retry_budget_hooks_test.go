package agent

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/hooks"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestStopHookKeepsUsefulStepsAfterNoAnswerRecovery(t *testing.T) {
	s := &hooksFeatureState{}
	if err := s.reset(); err != nil {
		t.Fatal(err)
	}
	defer s.close()
	s.maxTurns = 2
	if err := s.hookBlocksOnce(hooks.EventStop, "check the answer"); err != nil {
		t.Fatal(err)
	}
	if err := s.agentSession(); err != nil {
		t.Fatal(err)
	}
	if err := s.runPrompt("go", answerStep(""), answerStep("first answer"), answerStep("checked answer")); err != nil {
		t.Fatal(err)
	}
	if s.turnErr != nil || s.stopReason != string(acp.StopReasonEndTurn) {
		t.Fatalf("err=%v stop=%q", s.turnErr, s.stopReason)
	}
	if n := s.modelCalls(); n != 3 {
		t.Fatalf("model called %d times, want recovery plus two useful steps", n)
	}
	for _, message := range s.sess.GetMessages() {
		if message.Role == llm.RoleAssistant && strings.Contains(message.Content, "checked answer") {
			return
		}
	}
	t.Fatal("the Stop hook follow-up was dropped after provider recovery")
}
