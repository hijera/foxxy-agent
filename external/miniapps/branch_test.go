//go:build miniapps

package miniapps

import (
	"context"
	"strings"
	"testing"
)

// branchApp returns an app whose single workflow step selects between two
// nested programs, each writing a distinguishable result.
func branchApp(condition Condition) MiniApp {
	nested := func(id, text string) Step {
		return Step{
			ID: id, Kind: "program", Title: text, Language: VMVersion, Entry: "main",
			Functions: map[string][]Instruction{
				"main": {{Op: "const", Arg: text}, {Op: "return"}},
			},
			Limits: ProgramLimits{Instructions: 100, StackDepth: 16, CallDepth: 4},
		}
	}
	app := greetingApp()
	app.Inputs = []Input{{
		ID: "mode", Type: "string", Title: "Mode", Required: true,
		UI: InputUI{Control: "text"},
	}}
	app.Workflow = []Step{{
		ID: "pick", Kind: "branch", Title: "Pick a rendering",
		If:   &condition,
		Then: []Step{nested("render-then", "then")},
		Else: []Step{nested("render-else", "else")},
	}}
	app.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
		Kind: "step", Step: "pick", Status: "succeeded",
	}}}
	app.Outputs = nil
	return app
}

// The branch condition lives in "if". A false condition must run the "else"
// steps rather than skipping the whole step, which is what the shared "when"
// gate would do.
func TestBranchStepRunsElseWhenConditionIsFalse(t *testing.T) {
	t.Parallel()

	condition := Condition{Op: "eq", Left: Ref{Ref: "inputs.mode"}, Right: "markdown"}
	runner := NewRunner(NewStore(t.TempDir()), nil)

	run, err := runner.RunPortable(context.Background(), branchApp(condition),
		map[string]any{"mode": "html"}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, ok := run.Steps["render-else"]; !ok {
		t.Fatalf("else branch did not execute, steps = %v", stepIDsOf(run))
	}
	if _, ok := run.Steps["render-then"]; ok {
		t.Fatalf("then branch executed for a false condition, steps = %v", stepIDsOf(run))
	}

	run, err = runner.RunPortable(context.Background(), branchApp(condition),
		map[string]any{"mode": "markdown"}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, ok := run.Steps["render-then"]; !ok {
		t.Fatalf("then branch did not execute, steps = %v", stepIDsOf(run))
	}
	if _, ok := run.Steps["render-else"]; ok {
		t.Fatalf("else branch executed for a true condition, steps = %v", stepIDsOf(run))
	}
}

// "when" keeps its meaning on a branch step: it gates the step itself, so
// neither nested list runs.
func TestBranchStepWhenGateSkipsBothBranches(t *testing.T) {
	t.Parallel()

	app := branchApp(Condition{Op: "eq", Left: Ref{Ref: "inputs.mode"}, Right: "markdown"})
	app.Workflow[0].When = &Condition{Op: "eq", Left: Ref{Ref: "inputs.mode"}, Right: "never"}

	runner := NewRunner(NewStore(t.TempDir()), nil)
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"mode": "markdown"}, nil)
	if err == nil {
		t.Fatalf("skipped branch step should fail its success check, steps = %v", stepIDsOf(run))
	}
	if _, ok := run.Steps["render-then"]; ok {
		t.Fatalf("gated branch executed a nested step, steps = %v", stepIDsOf(run))
	}
}

func TestValidateRequiresBranchCondition(t *testing.T) {
	t.Parallel()

	app := branchApp(Condition{Op: "eq", Left: Ref{Ref: "inputs.mode"}, Right: "markdown"})
	app.Workflow[0].If = nil
	report := Validate(app)
	if report.Valid {
		t.Fatal("a branch step without a condition must be rejected")
	}
	if !strings.Contains(report.Issues[0].Path, "workflow[0]") {
		t.Fatalf("issue = %+v, want it anchored at the branch step", report.Issues[0])
	}
}

func stepIDsOf(run Run) []string {
	ids := make([]string, 0, len(run.Steps))
	for id := range run.Steps {
		ids = append(ids, id)
	}
	return ids
}
