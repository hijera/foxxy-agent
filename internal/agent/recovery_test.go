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

// TestPartialFailurePersistsAnswerAndRecoveryState drives the failure through a
// real HTTP stream that dies mid-answer (the shape the fork's typed stream
// errors describe): the partial answer the user watched stays in the
// transcript, and the execution checkpoint records the interruption.
func TestPartialFailurePersistsAnswerAndRecoveryState(t *testing.T) {
	srv, _ := stallingHub(t)
	off := false
	ag, st := stallingHubAgent(t, srv, func(a *config.Agent) { a.LLMContinue = &off })
	_, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hello"}})
	if err == nil || !strings.Contains(err.Error(), "200ms") {
		t.Fatalf("the turn must end with the stall naming the idle time, got err=%v", err)
	}
	msgs := st.GetMessages()
	if len(msgs) != 2 || msgs[1].Content != "Hello fr" {
		t.Fatalf("lost partial answer: %+v", msgs)
	}
	cp, err := session.ReadExecutionCheckpoint(st.SessionDir)
	if err != nil || cp.Status != "interrupted" {
		t.Fatalf("checkpoint=%+v err=%v", cp, err)
	}
}

func TestRecoveryRepairsMissingToolResultsWithoutChangingHistory(t *testing.T) {
	msgs := []llm.Message{{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "a", Name: "write"}, {ID: "b", Name: "read"}}}, {Role: llm.RoleTool, ToolCallID: "b", Content: "known"}, {Role: llm.RoleUser, Content: "continue"}}
	got := closeInterruptedToolCalls(msgs)
	if len(got) != 4 || got[1].Content != "known" || got[2].ToolCallID != "a" || !strings.Contains(got[2].Content, "Outcome unknown") || got[3].Role != llm.RoleUser {
		t.Fatalf("invalid repair: %+v", got)
	}
	if len(msgs) != 3 || msgs[1].Content != "known" {
		t.Fatal("mutated history")
	}
	if len(closeInterruptedToolCalls(got)) != 4 {
		t.Fatal("repair is not idempotent")
	}
}

func TestContinuationRequestIncludesNaturalLoopCorrections(t *testing.T) {
	for _, text := range []string{"тебя зациклило, продолжай дальше", "После рестарта продолжи с последнего шага", "You are stuck; continue from the last result."} {
		if !continuationRequest(text) {
			t.Errorf("correction would reset the loop guard: %q", text)
		}
	}
	if continuationRequest("Implement authentication") || continuationRequest("Explain continuations in Go") {
		t.Fatal("unrelated request treated as continuation")
	}
}

func TestLoopCorrectionAllowsNewInformation(t *testing.T) {
	s := &recoveryFeature{t: t}
	if err := s.setup(false); err != nil {
		t.Fatal(err)
	}
	cp := &session.ExecutionCheckpoint{Status: "no_progress", Repeats: 2, Seen: []string{"previous-observation"}}
	if err := cp.Save(s.st.SessionDir); err != nil {
		t.Fatal(err)
	}
	s.p.repeat = false
	_ = s.run()
	if s.err != nil || len(s.p.prompts) != 1 || !strings.Contains(s.p.prompts[0], loopCorrection) {
		t.Fatalf("correction did not recover: %v", s.err)
	}
	cp, err := session.ReadExecutionCheckpoint(s.st.SessionDir)
	if err != nil || cp.Repeats != 0 || cp.Status != "completed" {
		t.Fatalf("checkpoint=%+v error=%v", cp, err)
	}
}
