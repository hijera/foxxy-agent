//go:build miniapps

package miniapps

import (
	"context"
	"strings"
	"testing"
)

type expectationModelStub struct {
	responses []any
	prompts   []string
}

func (s *expectationModelStub) ExecuteModelStep(_ context.Context, _ ModelBinding, prompt string) (any, error) {
	s.prompts = append(s.prompts, prompt)
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func TestGenerateExpectedResultParsesStructuredModelResponse(t *testing.T) {
	models := &expectationModelStub{responses: []any{
		"```json\n{\"expected_result\":\"A friendly greeting using the supplied name.\",\"acceptance_criterion\":\"The output is friendly and contains the supplied name.\"}\n```",
	}}
	app := deterministicExpectationApp()
	binding := app.Requirements.ModelBindings[0]

	suggestion, err := GenerateExpectedResult(
		context.Background(),
		app,
		"The greeting must address the supplied person.",
		binding,
		models,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := suggestion.ExpectedResult, "A friendly greeting using the supplied name."; got != want {
		t.Fatalf("expected result = %q, want %q", got, want)
	}
	if !strings.Contains(models.prompts[0], "The greeting must address the supplied person.") {
		t.Fatalf("generation prompt does not contain author expectations: %q", models.prompts[0])
	}
}

func TestGenerateExpectedResultRejectsEmptyAuthorExpectations(t *testing.T) {
	app := deterministicExpectationApp()
	_, err := GenerateExpectedResult(
		context.Background(),
		app,
		"  ",
		app.Requirements.ModelBindings[0],
		&expectationModelStub{},
	)
	if err == nil {
		t.Fatal("empty author expectations must be rejected before a model call")
	}
}

func TestApplyExpectedResultAddsExecutablePromptCheck(t *testing.T) {
	app := deterministicExpectationApp()
	binding := app.Requirements.ModelBindings[0]
	suggestion := ExpectedResultSuggestion{
		Expectations:        "Address the supplied person.",
		ExpectedResult:      "A friendly greeting using the supplied name.",
		AcceptanceCriterion: "The output is friendly and contains the supplied name.",
		ModelBinding:        binding.ID,
	}

	app = ApplyExpectedResult(app, suggestion, binding)

	if app.Success.ExpectedResult != suggestion.ExpectedResult {
		t.Fatalf("success expected result = %q", app.Success.ExpectedResult)
	}
	check := app.Success.Checks[len(app.Success.Checks)-1]
	if check.Kind != "prompt" || check.ModelBinding != binding.ID {
		t.Fatalf("prompt check was not added: %#v", check)
	}
}

func TestPromptSuccessCheckUsesHiddenModelVerdict(t *testing.T) {
	app := deterministicExpectationApp()
	app = ApplyExpectedResult(app, ExpectedResultSuggestion{
		Expectations:        "Address the supplied person.",
		ExpectedResult:      "Hello, Foxxy!",
		AcceptanceCriterion: "The output contains the supplied name.",
		ModelBinding:        "acceptance-model",
	}, app.Requirements.ModelBindings[0])
	models := &expectationModelStub{responses: []any{
		`{"passed":true,"reason":"The name is present."}`,
	}}
	runner := NewRunner(nil, models)
	refs := map[string]any{
		"steps": map[string]any{
			"format-step": map[string]any{
				"outputs": map[string]any{"result": "Hello, Foxxy!"},
			},
		},
	}
	steps := map[string]StepResult{
		"format-step": {ID: "format-step", Status: RunSucceeded},
	}

	if err := runner.evaluateSuccess(context.Background(), app, refs, steps); err != nil {
		t.Fatal(err)
	}
	if len(models.prompts) != 1 || !strings.Contains(models.prompts[0], "Hello, Foxxy!") {
		t.Fatalf("verifier did not receive the declared result: %#v", models.prompts)
	}
}

func TestPromptSuccessCheckRejectsFailedVerdict(t *testing.T) {
	app := deterministicExpectationApp()
	app = ApplyExpectedResult(app, ExpectedResultSuggestion{
		Expectations:        "Address the supplied person.",
		ExpectedResult:      "A friendly greeting.",
		AcceptanceCriterion: "The output contains the supplied name.",
		ModelBinding:        "acceptance-model",
	}, app.Requirements.ModelBindings[0])
	models := &expectationModelStub{responses: []any{
		`{"passed":false,"reason":"The supplied name is missing."}`,
	}}
	runner := NewRunner(nil, models)
	refs := map[string]any{
		"steps": map[string]any{
			"format-step": map[string]any{
				"outputs": map[string]any{"result": "Hello!"},
			},
		},
	}
	steps := map[string]StepResult{
		"format-step": {ID: "format-step", Status: RunSucceeded},
	}

	if err := runner.evaluateSuccess(context.Background(), app, refs, steps); err == nil {
		t.Fatal("a failed verifier verdict must fail the run")
	}
}

func deterministicExpectationApp() MiniApp {
	return MiniApp{
		SchemaVersion: SchemaVersion,
		Kind:          KindMiniApp,
		ID:            "greeting-app",
		State:         StateDraft,
		Metadata: Metadata{
			Name: "Greeting app", Description: "Formats a greeting.",
			Goal: "Return a friendly greeting.",
		},
		Requirements: Requirements{ModelBindings: []ModelBinding{{
			ID: "acceptance-model", Selection: "fixed",
			Provider: ProviderIdentity{
				Type: "openai", BaseURL: "https://example.invalid/v1", Scope: "remote",
			},
			Model: "reviewed-model",
		}}},
		Permissions: Permissions{Models: []string{"acceptance-model"}},
		Workflow: []Step{{
			ID: "format-step", Kind: "program", Title: "Format greeting",
			Language: VMVersion, Entry: "main",
			Functions: map[string][]Instruction{"main": {
				{Op: "const", Arg: "Hello, Foxxy!"},
				{Op: "return"},
			}},
			Limits: ProgramLimits{Instructions: 100, StackDepth: 16, CallDepth: 4},
		}},
		Success: SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
			Kind: "step", Step: "format-step", Status: "succeeded",
		}}},
		Outputs: []Output{{
			ID: "result", Type: "text",
			Value: Ref{Ref: "steps.format-step.outputs.result"},
		}},
		Runtime: RuntimePolicy{
			LogScope: "global", OperatorEventLevel: "status",
			DiagnosticToolEvents: "sanitized",
		},
	}
}
