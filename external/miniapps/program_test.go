//go:build miniapps

package miniapps

import (
	"context"
	"testing"
)

func TestInputDependenciesDriveVisibilityAndRequiredValidation(t *testing.T) {
	inputs := []Input{
		{ID: "mode", Type: "string", Title: "Mode", Required: true, UI: InputUI{Control: "text"}},
		{
			ID: "details", Type: "string", Title: "Details", UI: InputUI{Control: "text"},
			VisibleWhen:  &Condition{Op: "eq", Left: Ref{Ref: "inputs.mode"}, Right: "advanced"},
			RequiredWhen: &Condition{Op: "eq", Left: Ref{Ref: "inputs.mode"}, Right: "advanced"},
		},
	}
	basic := map[string]any{"mode": "basic"}
	if err := validateInputs(inputs, basic, map[string]any{"inputs": basic}); err != nil {
		t.Fatalf("hidden dependent input should not be required: %v", err)
	}
	advanced := map[string]any{"mode": "advanced"}
	if err := validateInputs(inputs, advanced, map[string]any{"inputs": advanced}); err == nil {
		t.Fatal("visible dependent input should be required")
	}

	app := greetingApp()
	app.Inputs = []Input{
		{
			ID: "first", Type: "string", Title: "First", UI: InputUI{Control: "text"},
			VisibleWhen: &Condition{Op: "exists", Value: Ref{Ref: "inputs.second"}},
		},
		{
			ID: "second", Type: "string", Title: "Second", UI: InputUI{Control: "text"},
			VisibleWhen: &Condition{Op: "exists", Value: Ref{Ref: "inputs.first"}},
		},
	}
	if report := Validate(app); report.Valid {
		t.Fatal("cyclic input dependency graph must be rejected")
	}
}

func TestVMExecutesBoundedLoop(t *testing.T) {
	t.Parallel()

	program := Program{
		Language: "foxxy-vm/1",
		Entry:    "main",
		Functions: map[string][]Instruction{
			"main": {
				{Op: "ref.get", Arg: "inputs.items"},
				{Op: "local.set", Arg: "items"},
				{Op: "const", Arg: float64(0)},
				{Op: "local.set", Arg: "index"},
				{Op: "const", Arg: []any{}},
				{Op: "local.set", Arg: "result"},
				{Op: "label", Arg: "loop"},
				{Op: "local.get", Arg: "index"},
				{Op: "local.get", Arg: "items"},
				{Op: "array.len"},
				{Op: "lt"},
				{Op: "jump_if_false", Arg: "done"},
				{Op: "local.get", Arg: "result"},
				{Op: "local.get", Arg: "items"},
				{Op: "local.get", Arg: "index"},
				{Op: "array.get"},
				{Op: "array.push"},
				{Op: "local.set", Arg: "result"},
				{Op: "local.get", Arg: "index"},
				{Op: "const", Arg: float64(1)},
				{Op: "add"},
				{Op: "local.set", Arg: "index"},
				{Op: "jump", Arg: "loop"},
				{Op: "label", Arg: "done"},
				{Op: "local.get", Arg: "result"},
				{Op: "return"},
			},
		},
		Limits: ProgramLimits{Instructions: 1_000, StackDepth: 64, CallDepth: 8},
	}

	got, err := ExecuteProgram(context.Background(), program, map[string]any{
		"inputs": map[string]any{"items": []any{"a", "b", "c"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := got.([]any)
	if !ok || len(items) != 3 || items[2] != "c" {
		t.Fatalf("result = %#v, want copied array", got)
	}
}

func TestVMRejectsUnknownOpcodeBeforeExecution(t *testing.T) {
	t.Parallel()

	program := Program{
		Language:  "foxxy-vm/1",
		Entry:     "main",
		Functions: map[string][]Instruction{"main": {{Op: "eval"}}},
		Limits:    ProgramLimits{Instructions: 10, StackDepth: 8, CallDepth: 2},
	}
	if _, err := ExecuteProgram(context.Background(), program, nil); err == nil {
		t.Fatal("ExecuteProgram accepted an unknown opcode")
	}
}
