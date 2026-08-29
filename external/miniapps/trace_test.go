//go:build miniapps

package miniapps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestNormalizeSessionTracePairsToolCallsByIDAndPreservesMissing(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Collect the report for Paris."},
		{Role: llm.RoleAssistant, Content: "I will inspect the sources.", ToolCalls: []llm.ToolCall{
			{ID: "call-a", Name: "search", InputJSON: `{"query":"Paris"}`},
			{ID: "call-b", Name: "write_file", InputJSON: `{"path":"report.md","content":"draft"}`},
		}},
		{Role: llm.RoleTool, ToolCallID: "call-b", Content: "wrote report.md"},
		{Role: llm.RoleAssistant, Content: "<foxxycode_terminal_context>secret cwd</foxxycode_terminal_context>Done."},
	}

	trace, err := NormalizeSessionTrace("session-1", messages, nil)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	if len(trace.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(trace.Actions))
	}
	if trace.Actions[0].CallID != "call-a" || !trace.Actions[0].MissingResult {
		t.Fatalf("first action = %+v, want missing call-a", trace.Actions[0])
	}
	if trace.Actions[1].CallID != "call-b" || trace.Actions[1].Status != TraceActionSucceeded || trace.Actions[1].Result != "wrote report.md" {
		t.Fatalf("second action = %+v, want successful call-b", trace.Actions[1])
	}
	if strings.Contains(trace.LastAssistantResult, "foxxycode_terminal_context") || trace.LastAssistantResult != "Done." {
		t.Fatalf("injected context was not stripped: %q", trace.LastAssistantResult)
	}
}

func TestNormalizeSessionTraceMergesEvidenceAndRedactsSecrets(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Publish the build."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			ID: "call-1", Name: "publish", InputJSON: `{"api_key":"sk-12345678901234567890","channel":"stable"}`,
		}}},
	}
	evidence := []TraceCallEvidence{{
		ID: "call-1", Status: TraceActionSucceeded, Result: `{"url":"https://example.test/build"}`,
		StartedAt: "2026-08-14T10:00:00Z", FinishedAt: "2026-08-14T10:00:02Z",
		Artifacts: []TraceArtifact{{Path: "build.zip", SHA256: "abc"}},
	}}

	trace, err := NormalizeSessionTrace("session-2", messages, evidence)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	action := trace.Actions[0]
	if !action.ResultFound || action.DurationMS != 2000 || len(action.Artifacts) != 1 {
		t.Fatalf("merged action = %+v", action)
	}
	if strings.Contains(action.Arguments, "sk-12345678901234567890") {
		t.Fatalf("secret leaked in normalized arguments: %q", action.Arguments)
	}
	if !strings.Contains(action.Arguments, "REDACTED") {
		t.Fatalf("redaction marker missing from arguments: %q", action.Arguments)
	}
}

func TestNormalizeSessionTraceRedactsStructuralAuthLiteralEverywhere(t *testing.T) {
	const secret = "short-auth-value"
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Publish using " + secret + "."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			ID: "call-auth", Name: "publish", InputJSON: `{"auth":"` + secret + `","channel":"stable"}`,
		}}},
		{Role: llm.RoleTool, ToolCallID: "call-auth", Content: "published with " + secret},
		{Role: llm.RoleAssistant, Content: "Finished with " + secret + "."},
	}

	trace, err := NormalizeSessionTrace("session-auth", messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("structural auth literal leaked into normalized evidence: %s", raw)
	}
	if len(trace.Actions) != 1 || !strings.Contains(trace.Actions[0].Arguments, "REDACTED") {
		t.Fatalf("auth argument was not structurally redacted: %+v", trace.Actions)
	}
	inputs := ClassifyTraceInputs(trace, TraceConfirmedScenario{ActionIndexes: []int{0}})
	if !containsInputClass(inputs, TraceInputSecret) {
		t.Fatalf("auth argument was not classified as a secret: %+v", inputs)
	}
}

func TestNormalizeSessionTraceMarksDeniedResults(t *testing.T) {
	trace, err := NormalizeSessionTrace("session-denied", []llm.Message{
		{Role: llm.RoleUser, Content: "Delete the old build."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-denied", Name: "delete", InputJSON: `{"path":"build"}`}}},
		{Role: llm.RoleTool, ToolCallID: "call-denied", Content: "permission denied: protected path"},
		{Role: llm.RoleAssistant, Content: "I could not delete it."},
	}, nil)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	if len(trace.Actions) != 1 || trace.Actions[0].Status != TraceActionDenied || !trace.Actions[0].Denied || !trace.Actions[0].Failed {
		t.Fatalf("denied action = %+v", trace.Actions)
	}
}

func TestAssessTraceEligibilityRejectsConversationOnly(t *testing.T) {
	trace, err := NormalizeSessionTrace("session-3", []llm.Message{
		{Role: llm.RoleUser, Content: "Explain this design."},
		{Role: llm.RoleAssistant, Content: "Here is an explanation."},
	}, nil)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	result := AssessTraceEligibility(trace, false)
	if result.Eligible || result.Status != TraceEligibilityNotSuitable {
		t.Fatalf("eligibility = %+v, want not_suitable", result)
	}
}

func TestGenerateAndConfirmScenario(t *testing.T) {
	trace, err := NormalizeSessionTrace("session-4", []llm.Message{
		{Role: llm.RoleUser, Content: "Create a release note."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "write_file", InputJSON: `{"path":"RELEASE.md","content":"done"}`}}},
		{Role: llm.RoleTool, ToolCallID: "call-1", Content: "created RELEASE.md"},
		{Role: llm.RoleAssistant, Content: "Release note created."},
	}, nil)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	candidates := GenerateScenarioCandidates(trace)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want one: %+v", len(candidates), candidates)
	}
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		t.Fatalf("ConfirmScenario() error = %v", err)
	}
	if confirmed.Task != "Create a release note." || len(confirmed.ActionIndexes) != 1 {
		t.Fatalf("confirmed scenario = %+v", confirmed)
	}
}

func TestClassifyTraceInputs(t *testing.T) {
	trace, err := NormalizeSessionTrace("session-5", []llm.Message{
		{Role: llm.RoleUser, Content: "Publish cats from /workspace."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "publish", InputJSON: `{"query":"cats","cwd":"/workspace","api_token":"token-value"}`}}},
		{Role: llm.RoleTool, ToolCallID: "call-1", Content: "published"},
		{Role: llm.RoleAssistant, Content: "Published cats."},
	}, nil)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	candidates := GenerateScenarioCandidates(trace)
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		t.Fatalf("ConfirmScenario() error = %v", err)
	}
	inputs := ClassifyTraceInputs(trace, confirmed)
	classes := map[string]TraceInputClass{}
	for _, input := range inputs {
		classes[input.ID] = input.Class
	}
	if !containsInputClass(inputs, TraceInputOperator) || !containsInputClass(inputs, TraceInputEnvironment) || !containsInputClass(inputs, TraceInputSecret) {
		t.Fatalf("classified inputs = %+v", inputs)
	}
	if len(classes) < 3 {
		t.Fatalf("classified inputs = %+v, want at least three", inputs)
	}
}

func TestClassifyTraceInputsKeepsDistinctValuesAndStructuralPaths(t *testing.T) {
	trace := NormalizedTrace{
		Messages: []TraceMessage{{Role: string(llm.RoleUser), Content: "Copy a.txt to b.txt, then c.txt to d.txt."}},
		Actions: []TraceAction{
			{Index: 0, Name: "copy", Status: TraceActionSucceeded, Arguments: `{"source":{"path":"a.txt"},"destination":{"path":"b.txt"}}`},
			{Index: 1, Name: "copy", Status: TraceActionSucceeded, Arguments: `{"source":{"path":"c.txt"},"destination":{"path":"d.txt"}}`},
		},
	}
	inputs := ClassifyTraceInputs(trace, TraceConfirmedScenario{ActionIndexes: []int{0, 1}})
	paths := make(map[string]bool)
	for _, input := range inputs {
		if input.Class != TraceInputOperator {
			continue
		}
		paths[input.ObservedValue.(string)] = true
		if len(input.Occurrences) != 1 {
			t.Fatalf("distinct path/value was collapsed: %+v", input)
		}
	}
	for _, want := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		if !paths[want] {
			t.Fatalf("operator input %q missing from %+v", want, inputs)
		}
	}
}

func TestClassifyTraceInputsDoesNotInferAmbiguousPriorResult(t *testing.T) {
	trace := NormalizedTrace{
		Messages: []TraceMessage{{Role: string(llm.RoleUser), Content: "Write the selected city."}},
		Actions: []TraceAction{
			{Index: 0, Name: "first", Status: TraceActionSucceeded, Result: "Paris", ResultFound: true},
			{Index: 1, Name: "second", Status: TraceActionSucceeded, Result: "Paris", ResultFound: true},
			{Index: 2, Name: "write", Status: TraceActionSucceeded, Arguments: `{"content":"Paris"}`},
		},
	}
	inputs := ClassifyTraceInputs(trace, TraceConfirmedScenario{ActionIndexes: []int{0, 1, 2}})
	for _, input := range inputs {
		if input.ObservedValue == "Paris" {
			if input.Class == TraceInputPriorStep || input.PriorActionIndex != nil {
				t.Fatalf("ambiguous prior result was inferred: %+v", input)
			}
			return
		}
	}
	t.Fatalf("later argument was not classified: %+v", inputs)
}

func TestNormalizeSessionTracePreservesToolArgumentWhitespace(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Write the greeting."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-write", Name: "write", InputJSON: `{"path":"greeting.txt","content":"hello\n"}`}}},
		{Role: llm.RoleTool, ToolCallID: "call-write", Content: "wrote file"},
		{Role: llm.RoleAssistant, Content: "Done."},
	}
	trace, err := NormalizeSessionTrace("session-whitespace", messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Actions) != 1 || !strings.Contains(trace.Actions[0].Arguments, `"content":"hello\n"`) {
		t.Fatalf("normalized arguments changed content whitespace: %q", trace.Actions[0].Arguments)
	}
}

func containsInputClass(inputs []TraceInputSpec, want TraceInputClass) bool {
	for _, input := range inputs {
		if input.Class == want {
			return true
		}
	}
	return false
}
