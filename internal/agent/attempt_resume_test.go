package agent

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// The two openings a restart loop leaves in the transcript: the same answer begun
// twice, diverging only where the connection let it get further the second time.
const (
	resumeAttemptA = "Understood. I will fix the compilation errors. The main problems: " +
		"1. Wrong constant names (PARTICIPANT_CODE_FIELD)"
	resumeAttemptB = "Understood. I will fix the compilation errors. The main problems: " +
		"1. Wrong constant names (ATS_PARTICIPANT_CODE_FIELD, ATS_PERSON_ID_FIELD)"
)

func resumeAssistant(content, callID, path string) llm.Message {
	m := llm.Message{Role: llm.RoleAssistant, Content: content}
	if callID != "" {
		m.ToolCalls = []llm.ToolCall{{ID: callID, Name: "read", InputJSON: `{"path":"` + path + `"}`}}
	}
	return m
}

func resumeToolResult(callID, content string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: callID, Content: content}
}

func TestPriorTurnRestartsSpotsARepeatedOpening(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant(resumeAttemptA, "c1", "PersonService.java"),
		resumeToolResult("c1", "package app;"),
		resumeAssistant(resumeAttemptB, "c2", "PersonService.java"),
		resumeToolResult("c2", "package app;"),
		{Role: llm.RoleUser, Content: "continue"},
	}

	restarts, steps := priorTurnRestarts(history)
	if restarts != 1 {
		t.Fatalf("restarts = %d, want 1", restarts)
	}
	if len(steps) == 0 || !strings.Contains(steps[0], "read(PersonService.java)") {
		t.Fatalf("steps = %v, want the executed read listed", steps)
	}
}

func TestPriorTurnRestartsCountsEveryRepeat(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant(resumeAttemptA, "", ""),
		resumeAssistant(resumeAttemptB, "", ""),
		resumeAssistant(resumeAttemptA, "", ""),
		{Role: llm.RoleUser, Content: "continue"},
	}

	if restarts, _ := priorTurnRestarts(history); restarts != 2 {
		t.Fatalf("restarts = %d, want 2", restarts)
	}
}

func TestPriorTurnRestartsIgnoresAHealthyTurn(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant("Understood. I will start by reading the repository layout to find the service.", "c1", "layout.md"),
		resumeToolResult("c1", "src/"),
		resumeAssistant("The layout is clear. Now I will patch PersonService to return the projection.", "", ""),
		{Role: llm.RoleUser, Content: "thanks, now the tests"},
	}

	if restarts, _ := priorTurnRestarts(history); restarts != 0 {
		t.Fatalf("restarts = %d on a turn that moved forward, want 0", restarts)
	}
}

// A repeat must be consecutive. A turn that comes back to a phrasing several steps
// later is working through a list, not stuck on one answer.
func TestPriorTurnRestartsWantsConsecutiveAttempts(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant(resumeAttemptA, "", ""),
		resumeAssistant("Now the repository return type, which is a different fix entirely here.", "", ""),
		resumeAssistant(resumeAttemptB, "", ""),
		{Role: llm.RoleUser, Content: "continue"},
	}

	if restarts, _ := priorTurnRestarts(history); restarts != 0 {
		t.Fatalf("restarts = %d for non-consecutive attempts, want 0", restarts)
	}
}

// The compaction summary is stored with the user role, so a naive scan for "the
// previous prompt" would stop on it and read the wrong turn.
func TestPriorTurnRestartsSkipsCompactionSummaries(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant(resumeAttemptA, "", ""),
		resumeAssistant(resumeAttemptB, "", ""),
		{Role: llm.RoleUser, Content: "summary of earlier turns", CompactionSummary: true},
		{Role: llm.RoleUser, Content: "continue"},
	}

	if restarts, _ := priorTurnRestarts(history); restarts != 1 {
		t.Fatalf("restarts = %d with a compaction summary in the way, want 1", restarts)
	}
}

func TestPriorTurnRestartsWithoutAPriorTurn(t *testing.T) {
	for name, history := range map[string][]llm.Message{
		"empty":       nil,
		"only prompt": {{Role: llm.RoleUser, Content: "fix the compilation errors"}},
	} {
		t.Run(name, func(t *testing.T) {
			if restarts, steps := priorTurnRestarts(history); restarts != 0 || steps != nil {
				t.Fatalf("restarts = %d steps = %v, want 0 and none", restarts, steps)
			}
		})
	}
}

// An attempt that produced nothing is the provider's outage, not the model
// repeating itself, and two of them must not read as a loop.
func TestPriorTurnRestartsIgnoresEmptyAttempts(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		{Role: llm.RoleAssistant, Content: ""},
		{Role: llm.RoleAssistant, Content: ""},
		{Role: llm.RoleUser, Content: "continue"},
	}

	if restarts, _ := priorTurnRestarts(history); restarts != 0 {
		t.Fatalf("restarts = %d for empty attempts, want 0", restarts)
	}
}

func TestResumedRestartNudgeNamesWhatAlreadyRan(t *testing.T) {
	nudge := resumedRestartNudge(2, []string{"read(PersonService.java)"})
	for _, want := range []string{"previous turn", "read(PersonService.java)", "do not start it over"} {
		if !strings.Contains(nudge, want) {
			t.Errorf("nudge missing %q:\n%s", want, nudge)
		}
	}
}

// End to end: a session whose last turn died in a restart loop, reopened after a
// restart of the app. Nothing about that loop survives in memory - the nudges were
// never persisted, the guard's counters are gone - so the first request of the new
// turn is where the model has to be told, or it picks the loop straight back up.
func TestResumedTurnTellsTheModelItWasRepeating(t *testing.T) {
	h := newStallHarness(t, &stallProvider{script: []stallBehaviour{{answer: "Fixed the constant names."}}}, nil)
	for _, m := range []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant(resumeAttemptA, "c1", "PersonService.java"),
		resumeToolResult("c1", "package app;"),
		resumeAssistant(resumeAttemptB, "c2", "PersonService.java"),
		resumeToolResult("c2", "package app;"),
	} {
		h.st.AddMessage(m)
	}

	if _, err := h.run(t); err != nil {
		t.Fatalf("run: %v", err)
	}

	first := h.provider.request(1)
	if len(first) == 0 {
		t.Fatal("no request reached the provider")
	}
	last := first[len(first)-1].Content
	if !strings.Contains(last, "previous turn") || !strings.Contains(last, "read(PersonService.java)") {
		t.Errorf("the first request did not carry the resume notice, last message was:\n%s", last)
	}

	// LLM-facing only, the same contract as every other nudge: the transcript must
	// not gain a message the user never wrote.
	for _, m := range h.st.GetMessages() {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, "previous turn") {
			t.Errorf("the resume notice was persisted to the transcript: %q", m.Content)
		}
	}
}

// A healthy previous turn must leave the new prompt untouched.
func TestResumedTurnStaysQuietAfterAHealthyTurn(t *testing.T) {
	h := newStallHarness(t, &stallProvider{script: []stallBehaviour{{answer: "Sure."}}}, nil)
	for _, m := range []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant("Understood. I will start by reading the repository layout to find the service.", "", ""),
	} {
		h.st.AddMessage(m)
	}

	if _, err := h.run(t); err != nil {
		t.Fatalf("run: %v", err)
	}

	first := h.provider.request(1)
	if len(first) == 0 {
		t.Fatal("no request reached the provider")
	}
	if last := first[len(first)-1].Content; strings.Contains(last, "previous turn") {
		t.Errorf("a healthy turn was told it had been repeating:\n%s", last)
	}
}

// A turn that stumbled and then went on to answer is not the failure this warns
// about: only a turn that ended repeating counts.
func TestPriorTurnRestartsIgnoresARepeatItRecoveredFrom(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "fix the compilation errors"},
		resumeAssistant(resumeAttemptA, "", ""),
		resumeAssistant(resumeAttemptB, "", ""),
		resumeAssistant("All four fixes are in: constants renamed, repository projection changed, and the build is clean.", "", ""),
		{Role: llm.RoleUser, Content: "thanks, now the tests"},
	}

	if restarts, _ := priorTurnRestarts(history); restarts != 0 {
		t.Fatalf("restarts = %d for a turn that recovered and answered, want 0", restarts)
	}
}
