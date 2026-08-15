//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

type authoringModelStub struct {
	responses []*llm.Response
	calls     int
	tools     []llm.ToolDefinition
}

func (s *authoringModelStub) CompleteDraftAuthoring(
	_ context.Context,
	_ ModelBinding,
	_ []llm.Message,
	tools []llm.ToolDefinition,
) (*llm.Response, error) {
	s.tools = tools
	response := s.responses[s.calls]
	s.calls++
	return response, nil
}

func TestEditDraftWithAssistantAppliesBoundedToolCalls(t *testing.T) {
	inputArgs, _ := json.Marshal(map[string]any{"input": Input{
		ID: "style", Type: "string", Title: "Style",
		UI: InputUI{Control: "text", Order: 20},
	}})
	stepArgs, _ := json.Marshal(map[string]any{"step": Step{
		ID: "decorate", Kind: "program", Title: "Apply style",
		Language: VMVersion, Entry: "main",
		Functions: map[string][]Instruction{"main": {
			{Op: "ref.get", Arg: "inputs.style"},
			{Op: "return"},
		}},
		Limits: ProgramLimits{Instructions: 100, StackDepth: 16, CallDepth: 4},
	}})
	model := &authoringModelStub{responses: []*llm.Response{
		{
			ToolCalls: []llm.ToolCall{
				{ID: "input-call", Name: "miniapp_upsert_input", InputJSON: string(inputArgs)},
				{ID: "step-call", Name: "miniapp_upsert_step", InputJSON: string(stepArgs)},
			},
			StopReason: "tool_use",
		},
		{Content: "Added the requested input and step.", StopReason: "end_turn"},
	}}
	app := deterministicExpectationApp()
	binding := app.Requirements.ModelBindings[0]

	result, err := EditDraftWithAssistant(
		context.Background(),
		app,
		AuthoringRequest{Message: "Add a style input and decoration step."},
		binding,
		model,
	)
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 2 || len(model.tools) < 6 {
		t.Fatalf("authoring model calls=%d tools=%d", model.calls, len(model.tools))
	}
	if len(result.Operations) != 2 || result.Message == "" {
		t.Fatalf("unexpected authoring result: %#v", result)
	}
	if !hasInputID(result.App.Inputs, "style") || !hasStepID(result.App.Workflow, "decorate") {
		t.Fatalf("tool edits were not applied: %#v", result.App)
	}
}

func TestApplyPrimaryModelBindingUsesSelectedLogicalModelEverywhere(t *testing.T) {
	app := deterministicExpectationApp()
	app.Workflow = append(app.Workflow, Step{
		ID: "model-step", Kind: "agent", Title: "Model step",
		Prompt: "Do the task.", ModelBinding: "old-model",
	})
	app.Success.Checks = append(app.Success.Checks, SuccessCheck{
		Kind: "prompt", Value: Ref{Ref: "steps.model-step.outputs.result"},
		Prompt: "Verify it.", ModelBinding: "old-model",
	})
	binding := ModelBinding{
		ID: "primary", LogicalModel: "fake/reviewed-model", Selection: "fixed",
		Provider: ProviderIdentity{
			Type: "openai", BaseURL: "https://fake.invalid/v1", Scope: "remote",
		},
		Model: "reviewed-model",
	}

	app = ApplyPrimaryModelBinding(app, binding)

	if app.Workflow[len(app.Workflow)-1].ModelBinding != "primary" {
		t.Fatalf("agent step did not use primary binding: %#v", app.Workflow)
	}
	if app.Success.Checks[len(app.Success.Checks)-1].ModelBinding != "primary" {
		t.Fatalf("prompt check did not use primary binding: %#v", app.Success.Checks)
	}
}

func hasInputID(inputs []Input, id string) bool {
	for _, input := range inputs {
		if input.ID == id {
			return true
		}
	}
	return false
}

func hasStepID(steps []Step, id string) bool {
	for _, step := range steps {
		if step.ID == id {
			return true
		}
	}
	return false
}
