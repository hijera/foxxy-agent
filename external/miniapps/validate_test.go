//go:build miniapps

package miniapps

import (
	"strings"
	"testing"
)

func TestValidateMinimalWorkflowAndReferences(t *testing.T) {
	app := coreValidApp()
	app.Outputs = []Output{{ID: "result", Type: "string", Value: Ref{Ref: "steps.read.outputs.result"}}}
	report := Validate(app)
	if !report.Valid {
		t.Fatalf("valid app rejected: %+v", report.Issues)
	}

	withUnknownRef := coreValidApp()
	withUnknownRef.Workflow[0].Arguments = map[string]any{
		"path": Ref{Ref: "inputs.missing"},
	}
	report = Validate(withUnknownRef)
	if report.Valid || !hasIssueText(report, "unknown input missing") {
		t.Fatalf("unknown reference accepted: %+v", report.Issues)
	}
}

func TestValidateRejectsDeferredAndMalformedStepKinds(t *testing.T) {
	deferred := coreValidApp()
	deferred.Workflow[0].Kind = "script"
	deferred.Workflow[0].Tool = ""
	deferred.Workflow[0].Prompt = "echo unsafe"
	report := Validate(deferred)
	if report.Valid || !hasIssuePath(report, "workflow[0].kind") {
		t.Fatalf("deferred script kind accepted: %+v", report.Issues)
	}

	badAgent := coreValidApp()
	badAgent.Workflow[0] = Step{
		ID: "agent", Kind: "agent", Title: "Agent", Prompt: "do work",
		ModelBinding: "missing", MaxTurns: 100,
	}
	report = Validate(badAgent)
	if report.Valid || !hasIssuePath(report, "workflow[0].max_turns") || !hasIssuePath(report, "workflow[0].model_binding") {
		t.Fatalf("malformed agent accepted: %+v", report.Issues)
	}
}

func TestValidateRejectsForwardAndDuplicateReferences(t *testing.T) {
	app := coreValidApp()
	app.Workflow = []Step{
		{
			ID: "first", Kind: "tool", Title: "First", Tool: "files.read",
			Arguments: map[string]any{"value": Ref{Ref: "steps.second.outputs.result"}},
		},
		{ID: "second", Kind: "tool", Title: "Second", Tool: "files.read"},
	}
	report := Validate(app)
	if report.Valid || !hasIssueText(report, "has not executed yet: second") {
		t.Fatalf("forward step reference accepted: %+v", report.Issues)
	}

	app = coreValidApp()
	app.Workflow = append(app.Workflow, Step{ID: "prepare", Kind: "tool", Title: "Duplicate", Tool: "files.read"})
	app.Workflow[0].ID = "prepare"
	report = Validate(app)
	if report.Valid || !hasIssuePath(report, "workflow[1].id") {
		t.Fatalf("duplicate step id accepted: %+v", report.Issues)
	}
}

func TestValidateRejectsReferencesAcrossMutuallyExclusiveBranchArms(t *testing.T) {
	app := coreValidApp()
	app.Workflow = []Step{{
		ID: "choose", Kind: "branch", Title: "Choose", If: &Condition{Op: "exists", Value: Ref{Ref: "inputs.source"}},
		Then: []Step{{ID: "then-step", Kind: "tool", Title: "Then", Tool: "files.read", Arguments: map[string]any{"path": Ref{Ref: "inputs.source"}}}},
		Else: []Step{{ID: "else-step", Kind: "tool", Title: "Else", Tool: "files.read", Arguments: map[string]any{"path": Ref{Ref: "steps.then-step.outputs.result"}}}},
	}}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "choose", Status: string(RunSucceeded)}}
	app.Outputs = []Output{{ID: "result", Type: "string", Value: Ref{Ref: "steps.then-step.outputs.result"}}}
	report := Validate(app)
	if report.Valid || !hasIssueText(report, "has not executed yet: then-step") {
		t.Fatalf("cross-branch reference accepted: %+v", report.Issues)
	}
}

func TestValidateConditionsAndModelCapabilities(t *testing.T) {
	app := coreValidApp()
	app.Workflow[0].When = &Condition{Op: "matches", Left: Ref{Ref: "inputs.source"}, Right: "["}
	report := Validate(app)
	if report.Valid || !hasIssuePath(report, "workflow[0].when") {
		t.Fatalf("invalid condition accepted: %+v", report.Issues)
	}

	app = coreValidApp()
	app.Workflow[0].When = &Condition{Op: "eq", Left: Ref{Ref: "inputs.source"}, Right: "x"}
	report = ValidateWithCapabilities(app, CapabilitySet{Tools: map[string]bool{"other": true}})
	if report.Valid || !hasIssuePath(report, "workflow[0].tool") {
		t.Fatalf("unknown tool capability accepted: %+v", report.Issues)
	}
}

func TestValidateRejectsSecretDefaultsAndInputCycles(t *testing.T) {
	app := coreValidApp()
	app.Inputs = []Input{
		{ID: "a", Type: "string", Title: "A", UI: InputUI{Control: "text"},
			VisibleWhen: &Condition{Op: "exists", Value: Ref{Ref: "inputs.b"}}},
		{ID: "b", Type: "secret", Title: "B", Default: "persisted", UI: InputUI{Control: "password"},
			VisibleWhen: &Condition{Op: "exists", Value: Ref{Ref: "inputs.a"}}},
	}
	report := Validate(app)
	if report.Valid || !hasIssuePath(report, "inputs") {
		t.Fatalf("cyclic inputs accepted: %+v", report.Issues)
	}
	if !hasIssuePath(report, "inputs[1].default") {
		t.Fatalf("secret default was not rejected: %+v", report.Issues)
	}
}

func TestRuntimeInputValidationEnforcesNumericBounds(t *testing.T) {
	minimum, maximum := 2.0, 4.0
	inputs := []Input{{ID: "count", Type: "number", Title: "Count", Required: true, UI: InputUI{Control: "number"}, Validation: Validation{Minimum: &minimum, Maximum: &maximum}}}
	refs := map[string]any{"inputs": map[string]any{"count": 1.0}}
	if err := validateInputs(inputs, refs["inputs"].(map[string]any), refs); err == nil {
		t.Fatal("value below minimum was accepted")
	}
	refs = map[string]any{"inputs": map[string]any{"count": 3.0}}
	if err := validateInputs(inputs, refs["inputs"].(map[string]any), refs); err != nil {
		t.Fatalf("bounded value rejected: %v", err)
	}
}

func TestValidateRejectsUnimplementedFileConstraints(t *testing.T) {
	app := coreValidApp()
	app.Inputs[0].Validation.MustExist = true
	report := Validate(app)
	if report.Valid || !hasIssueText(report, "file constraints are not supported") {
		t.Fatalf("unimplemented file validation accepted: %+v", report.Issues)
	}
}

func TestValidateRejectsUnknownOutputSchemaKeywords(t *testing.T) {
	app := coreValidApp()
	app.Workflow = []Step{{ID: "classify", Kind: "llm", Title: "Classify", Prompt: "classify", ModelBinding: "model", OutputSchema: map[string]any{"type": "object", "oneOf": []any{}}}}
	app.Requirements.ModelBindings = []ModelBinding{{ID: "model", Selection: "fixed", Model: "configured/model"}}
	app.Permissions.Models = []string{"model"}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "classify", Status: string(RunSucceeded)}}
	report := Validate(app)
	if report.Valid || !hasIssueText(report, "not supported in schema v1") {
		t.Fatalf("unknown output schema keyword accepted: %+v", report.Issues)
	}
}

func coreValidApp() MiniApp {
	return MiniApp{
		SchemaVersion: SchemaVersion,
		Kind:          KindMiniApp,
		ID:            "file-transform",
		State:         StateDraft,
		Metadata: Metadata{
			Name: "File transform", Goal: "Transform a source file.",
		},
		Permissions: Permissions{Tools: []string{"files.read"}},
		Inputs: []Input{{
			ID: "source", Type: "file", Title: "Source", Required: true,
			UI: InputUI{Control: "file"},
		}},
		Workflow: []Step{{
			ID: "read", Kind: "tool", Title: "Read source", Tool: "files.read",
			Arguments: map[string]any{"path": Ref{Ref: "inputs.source"}},
		}},
		Success: SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
			Kind: "step", Step: "read", Status: string(RunSucceeded),
		}}},
		Runtime: RuntimePolicy{
			LogScope: "global", OperatorEventLevel: "status",
			DiagnosticToolEvents: "sanitized", PersistAgentReasoning: false,
		},
	}
}

func hasIssuePath(report ValidationReport, fragment string) bool {
	for _, issue := range report.Issues {
		if strings.Contains(issue.Path, fragment) {
			return true
		}
	}
	return false
}

func hasIssueText(report ValidationReport, fragment string) bool {
	for _, issue := range report.Issues {
		if strings.Contains(issue.Path, fragment) || strings.Contains(issue.Message, fragment) {
			return true
		}
	}
	return false
}
