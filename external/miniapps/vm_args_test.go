//go:build miniapps

package miniapps

import (
	"context"
	"testing"
)

// Every opcode that reads a name out of its argument must have that argument
// verified before execution: the interpreter asserts the argument is a string
// while it runs, so an unverified non-string argument would panic mid-run.
func TestVMRejectsNonStringInstructionArguments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		program Program
	}{
		{
			name: "host.call",
			program: Program{
				Language: VMVersion, Entry: "main",
				// An empty import name is what lets a numeric argument slip
				// through a lookup that falls back to the zero string.
				Imports: map[string]ProgramImport{"": {Capability: "noop"}},
				Functions: map[string][]Instruction{
					"main": {{Op: "const", Arg: nil}, {Op: "host.call", Arg: float64(1)}},
				},
				Limits: ProgramLimits{Instructions: 10, StackDepth: 8, CallDepth: 2},
			},
		},
		{
			name: "jump",
			program: Program{
				Language: VMVersion, Entry: "main",
				Functions: map[string][]Instruction{
					"main": {{Op: "jump", Arg: float64(1)}},
				},
				Limits: ProgramLimits{Instructions: 10, StackDepth: 8, CallDepth: 2},
			},
		},
		{
			name: "call",
			program: Program{
				Language: VMVersion, Entry: "main",
				Functions: map[string][]Instruction{
					"main": {{Op: "call", Arg: map[string]any{}}},
				},
				Limits: ProgramLimits{Instructions: 10, StackDepth: 8, CallDepth: 2},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ExecuteProgramWithHost(context.Background(), tc.program, nil, noopHost{}); err == nil {
				t.Fatal("ExecuteProgram accepted a non-string instruction argument")
			}
		})
	}
}

func TestVMRejectsEmptyImportName(t *testing.T) {
	t.Parallel()

	program := Program{
		Language: VMVersion, Entry: "main",
		Imports: map[string]ProgramImport{"": {Capability: "noop"}},
		Functions: map[string][]Instruction{
			"main": {{Op: "const", Arg: nil}, {Op: "host.call", Arg: ""}},
		},
		Limits: ProgramLimits{Instructions: 10, StackDepth: 8, CallDepth: 2},
	}
	if _, err := ExecuteProgramWithHost(context.Background(), program, nil, noopHost{}); err == nil {
		t.Fatal("ExecuteProgram accepted an empty import name")
	}
}

type noopHost struct{}

func (noopHost) Call(context.Context, string, any) (any, error) { return nil, nil }
