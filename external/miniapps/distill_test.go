//go:build miniapps

package miniapps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestDistillRequiresScenarioConfirmation(t *testing.T) {
	input := DistillInput{
		SessionID: "session-distill-1",
		Title:     "Publish build",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Publish the build."},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "publish", InputJSON: `{"channel":"stable"}`}}},
			{Role: llm.RoleTool, ToolCallID: "call-1", Content: "published"},
			{Role: llm.RoleAssistant, Content: "Build published."},
		},
	}
	_, evidence, err := Distill(input)
	if err == nil {
		t.Fatal("Distill() error = nil, want scenario confirmation error")
	}
	if len(evidence.ScenarioCandidates) != 1 {
		t.Fatalf("evidence candidates = %+v", evidence.ScenarioCandidates)
	}
	if !strings.Contains(err.Error(), "scenario") {
		t.Fatalf("Distill() error = %v, want scenario detail", err)
	}
}

func TestDistillSynthesizesDeterministicToolWorkflow(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Publish cats from /workspace."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "call-1", Name: "search", InputJSON: `{"query":"cats","cwd":"/workspace"}`},
			{ID: "call-2", Name: "publish", InputJSON: `{"channel":"stable","token":"secret-value"}`},
		}},
		{Role: llm.RoleTool, ToolCallID: "call-1", Content: "found cats"},
		{Role: llm.RoleTool, ToolCallID: "call-2", Content: "published cats"},
		{Role: llm.RoleAssistant, Content: "Published cats."},
	}
	trace, err := NormalizeSessionTrace("session-distill-2", messages, nil)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	candidates := GenerateScenarioCandidates(trace)
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		t.Fatalf("ConfirmScenario() error = %v", err)
	}
	app, evidence, err := Distill(DistillInput{
		SessionID: "session-distill-2", Title: "Publish build", Author: "tester",
		Messages: messages, Scenario: &confirmed,
	})
	if err != nil {
		t.Fatalf("Distill() error = %v", err)
	}
	if len(app.Workflow) != 2 || app.Workflow[0].Kind != "tool" || app.Workflow[1].Kind != "tool" {
		t.Fatalf("workflow = %+v, want two tool steps", app.Workflow)
	}
	if app.Workflow[0].ID != "tool-1-search" || app.Workflow[1].ID != "tool-2-publish" {
		t.Fatalf("workflow IDs = %q, %q", app.Workflow[0].ID, app.Workflow[1].ID)
	}
	if len(app.Inputs) < 2 {
		t.Fatalf("inputs = %+v, want operator and secret/environment inputs", app.Inputs)
	}
	if evidence.ConfirmedScenario == nil || evidence.SanitizedTrace == nil {
		t.Fatalf("evidence = %+v, want confirmed scenario and trace", evidence)
	}
	if report := Validate(app); !report.Valid {
		t.Fatalf("generated app does not validate: %+v", report.Issues)
	}
	if strings.Contains(stringMustJSON(app), "secret-value") {
		t.Fatal("secret value leaked into generated MiniApp")
	}
}

func TestDistillDoesNotCompileFailedAttempts(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Write the report."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "failed", Name: "write_file", InputJSON: `{"path":"report.md"}`},
			{ID: "passed", Name: "write_file", InputJSON: `{"path":"report.md","content":"complete"}`},
		}},
		{Role: llm.RoleTool, ToolCallID: "failed", Content: "error: permission denied"},
		{Role: llm.RoleTool, ToolCallID: "passed", Content: "created"},
		{Role: llm.RoleAssistant, Content: "Report written."},
	}
	trace, err := NormalizeSessionTrace("session-distill-3", messages, nil)
	if err != nil {
		t.Fatalf("NormalizeSessionTrace() error = %v", err)
	}
	candidates := GenerateScenarioCandidates(trace)
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		t.Fatalf("ConfirmScenario() error = %v", err)
	}
	app, _, err := Distill(DistillInput{SessionID: "session-distill-3", Messages: messages, Scenario: &confirmed})
	if err != nil {
		t.Fatalf("Distill() error = %v", err)
	}
	if len(app.Workflow) != 1 || app.Workflow[0].Tool != "write_file" {
		t.Fatalf("workflow = %+v, want only successful write_file", app.Workflow)
	}
}

func TestDistillUsesUnambiguousPriorToolResultReference(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Find Ada's city and write it to city.txt."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "lookup", Name: "lookup", InputJSON: `{"query":"Ada"}`}}},
		{Role: llm.RoleTool, ToolCallID: "lookup", Content: "Paris"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "write", Name: "write", InputJSON: `{"path":"city.txt","content":"Paris"}`}}},
		{Role: llm.RoleTool, ToolCallID: "write", Content: "wrote city.txt"},
		{Role: llm.RoleAssistant, Content: "City written."},
	}
	trace, err := NormalizeSessionTrace("session-prior", messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidates := GenerateScenarioCandidates(trace)
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := Distill(DistillInput{SessionID: "session-prior", Title: "City file", Messages: messages, Scenario: &confirmed})
	if err != nil {
		t.Fatal(err)
	}
	arguments, ok := app.Workflow[1].Arguments.(map[string]any)
	if !ok {
		t.Fatalf("write arguments = %#v", app.Workflow[1].Arguments)
	}
	content, ok := arguments["content"].(Ref)
	if !ok || content.Ref != "steps.tool-1-lookup.outputs.result" {
		t.Fatalf("prior result reference = %#v", arguments["content"])
	}
	if strings.Contains(stringMustJSON(app), `"content":"Paris"`) {
		t.Fatalf("prior result was retained as a literal: %s", stringMustJSON(app))
	}
}

func TestDistillStructuralAuthBecomesSecretReference(t *testing.T) {
	const secret = "short-auth-value"
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Publish the build."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "publish", Name: "publish", InputJSON: `{"auth":"` + secret + `"}`}}},
		{Role: llm.RoleTool, ToolCallID: "publish", Content: "published"},
		{Role: llm.RoleAssistant, Content: "Published."},
	}
	trace, err := NormalizeSessionTrace("session-auth-distill", messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidates := GenerateScenarioCandidates(trace)
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	app, evidence, err := Distill(DistillInput{SessionID: "session-auth-distill", Title: "Auth publish", Messages: messages, Scenario: &confirmed})
	if err != nil {
		t.Fatal(err)
	}
	portable, private := stringMustJSON(app), stringMustJSON(evidence)
	if strings.Contains(portable, secret) || strings.Contains(private, secret) {
		t.Fatalf("secret leaked: app=%s evidence=%s", portable, private)
	}
	if len(app.Inputs) != 1 || app.Inputs[0].Type != "secret" {
		t.Fatalf("secret inputs = %+v", app.Inputs)
	}
	arguments := app.Workflow[0].Arguments.(map[string]any)
	if ref, ok := arguments["auth"].(Ref); !ok || ref.Ref != "inputs."+app.Inputs[0].ID {
		t.Fatalf("auth argument = %#v", arguments["auth"])
	}
}

func TestDistillKeepsDeclaredFixtureFilesPrivate(t *testing.T) {
	messages, confirmed := distillFixtureScenario(t)
	files := map[string][]byte{"fixtures/../source/input.txt": []byte("fixture-only-payload\n")}
	app, evidence, err := Distill(DistillInput{
		SessionID: "session-fixture", Title: "Fixture reader", Messages: messages,
		Scenario: confirmed, FixtureFiles: files,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stringMustJSON(app), "fixture-only-payload") {
		t.Fatalf("fixture content entered portable app: %s", stringMustJSON(app))
	}
	content, ok := evidence.FixtureFiles["source/input.txt"]
	if !ok || string(content) != "fixture-only-payload\n" {
		t.Fatalf("private fixture files = %#v", evidence.FixtureFiles)
	}
	files["fixtures/../source/input.txt"][0] = 'X'
	if string(evidence.FixtureFiles["source/input.txt"]) != "fixture-only-payload\n" {
		t.Fatal("fixture evidence aliases caller-owned bytes")
	}
}

func TestDistillRejectsUnsafeFixtureFiles(t *testing.T) {
	messages, confirmed := distillFixtureScenario(t)
	for name, files := range map[string]map[string][]byte{
		"traversal": {"../secret.txt": []byte("safe")},
		"secret":    {"source.txt": []byte("api_key=abcdefghijk")},
	} {
		t.Run(name, func(t *testing.T) {
			_, evidence, err := Distill(DistillInput{
				SessionID: "session-fixture", Messages: messages, Scenario: confirmed, FixtureFiles: files,
			})
			if err == nil {
				t.Fatal("Distill() error = nil")
			}
			if len(evidence.FixtureFiles) != 0 {
				t.Fatalf("unsafe fixture was retained: %#v", evidence.FixtureFiles)
			}
		})
	}
}

func distillFixtureScenario(t *testing.T) ([]llm.Message, *TraceConfirmedScenario) {
	t.Helper()
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Read source/input.txt."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "read", Name: "read", InputJSON: `{"path":"source/input.txt"}`}}},
		{Role: llm.RoleTool, ToolCallID: "read", Content: "source input"},
		{Role: llm.RoleAssistant, Content: "Read source input."},
	}
	trace, err := NormalizeSessionTrace("session-fixture", messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidates := GenerateScenarioCandidates(trace)
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	return messages, &confirmed
}

func stringMustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
