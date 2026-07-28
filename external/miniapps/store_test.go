//go:build miniapps

package miniapps

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDraftTestReleaseAndRun(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	app := greetingApp()
	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(store, nil)
	run, err := runner.RunDraft(context.Background(), app.ID, map[string]any{"name": "Foxxy"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunSucceeded || run.Outputs["text"] != "Hello, Foxxy!" {
		t.Fatalf("run = %#v", run)
	}
	if err := store.RecordPassingTest(app.ID, run); err != nil {
		t.Fatal(err)
	}
	released, err := store.Release(app.ID, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if released.State != StateReleased || released.Version != "1.0.0" {
		t.Fatalf("released = %#v", released)
	}

	releasePath := filepath.Join(store.Root(), app.ID, "releases", "1.0.0", "miniapp.json")
	if _, err := os.Stat(releasePath); err != nil {
		t.Fatalf("release file: %v", err)
	}

	second, err := runner.RunRelease(context.Background(), app.ID, "1.0.0", map[string]any{"name": "Operator"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outputs["text"] != "Hello, Operator!" {
		t.Fatalf("released output = %#v", second.Outputs)
	}
}

func TestReleaseRequiresCurrentPassingTest(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	app := greetingApp()
	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Release(app.ID, "1.0.0"); err == nil {
		t.Fatal("Release succeeded without a passing draft test")
	}
}

func greetingApp() MiniApp {
	return MiniApp{
		SchemaVersion: "1.0.0",
		Kind:          "foxxycode.miniapp",
		ID:            "greeting",
		State:         StateDraft,
		Metadata: Metadata{
			Name:        "Greeting",
			Description: "Formats a greeting.",
			Goal:        "Return a greeting for the supplied name.",
		},
		Inputs: []Input{{
			ID:       "name",
			Type:     "string",
			Title:    "Name",
			Required: true,
			UI:       InputUI{Control: "text", Order: 10},
		}},
		Workflow: []Step{{
			ID:       "format",
			Kind:     "program",
			Title:    "Format greeting",
			Language: "foxxy-vm/1",
			Entry:    "main",
			Functions: map[string][]Instruction{
				"main": {
					{Op: "const", Arg: "Hello, "},
					{Op: "ref.get", Arg: "inputs.name"},
					{Op: "string.concat"},
					{Op: "const", Arg: "!"},
					{Op: "string.concat"},
					{Op: "return"},
				},
			},
			Limits: ProgramLimits{Instructions: 100, StackDepth: 16, CallDepth: 4},
		}},
		Success: SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
			Kind: "step", Step: "format", Status: "succeeded",
		}}},
		Outputs: []Output{{
			ID:       "text",
			Type:     "text",
			Value:    Ref{Ref: "steps.format.outputs.result"},
			Renderer: "text",
		}},
		Runtime: RuntimePolicy{
			LogScope:              "global",
			OperatorEventLevel:    "status",
			DiagnosticToolEvents:  "sanitized",
			PersistAgentReasoning: false,
		},
	}
}

func TestRunnerUsesSelectedLocalRunRoot(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	app := greetingApp()
	app.ID = "local-run-app"
	app.Runtime.LogScope = "local"
	store := NewStoreWithRunRoot(filepath.Join(root, "miniapps"), filepath.Join(root, "global-apps"))
	runner := NewRunner(store, nil).WithLocalWorkspace(workspace)
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"name": "Local"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(workspace, ".foxxycode", "apps", app.ID, "runs", run.ID, "run.json")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("local run was not persisted at %s: %v", expected, err)
	}
}
