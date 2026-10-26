package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func TestSpawnAgentWakesByDefaultUnlessExplicitlyDisabled(t *testing.T) {
	for _, tc := range []struct {
		name string
		args string
		wake bool
	}{
		{"omitted", `{"agent":"general","prompt":"inspect","background":true}`, true},
		{"explicit false", `{"agent":"general","prompt":"inspect","background":true,"notify_on_finish":false}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got tooling.SpawnRequest
			_, err := SpawnAgentTool().Execute(context.Background(), tc.args, &tooling.Env{
				SpawnAgent: func(_ context.Context, req tooling.SpawnRequest) (string, error) {
					got = req
					return "started", nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.NotifyOnFinish != tc.wake {
				t.Fatalf("notify_on_finish = %v, want %v", got.NotifyOnFinish, tc.wake)
			}
		})
	}
}

// A run that stopped before finishing is continued in its own session rather
// than started over (issue #389): the tool takes the run to resume, by its task
// id or its child session id, and hands it to the runtime.
func TestSpawnAgentPassesTheRunToResume(t *testing.T) {
	var got tooling.SpawnRequest
	_, err := SpawnAgentTool().Execute(context.Background(), `{"agent":"general","prompt":"go on","resume":" bg_5 "}`, &tooling.Env{
		SpawnAgent: func(_ context.Context, req tooling.SpawnRequest) (string, error) {
			got = req
			return "started", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Resume != "bg_5" || got.Agent != "general" || got.Prompt != "go on" {
		t.Fatalf("request = %+v, want the run bg_5 resumed with the prompt", got)
	}
	schema, _ := SpawnAgentTool().Definition.InputSchema.(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})
	resume, _ := props["resume"].(map[string]interface{})
	desc, _ := resume["description"].(string)
	for _, want := range []string{"task id", "session id", "transcript"} {
		if !strings.Contains(desc, want) {
			t.Errorf("the resume argument does not mention %q: %q", want, desc)
		}
	}
}

// A detached child wakes the parent by default, and the tool says so: text that
// only tells the model to collect the report later keeps it waiting for a run
// it could leave to wake it.
func TestSpawnAgentDescribesTheDefaultWake(t *testing.T) {
	def := SpawnAgentTool().Definition
	schema, _ := def.InputSchema.(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})
	background, _ := props["background"].(map[string]interface{})
	desc, _ := background["description"].(string)
	for name, text := range map[string]string{"description": def.Description, "background": desc} {
		if !strings.Contains(text, "wakes you") {
			t.Errorf("spawn_agent %s does not say a detached run wakes the parent: %q", name, text)
		}
	}
}
