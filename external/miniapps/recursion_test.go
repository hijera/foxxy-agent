//go:build miniapps

package miniapps

import (
	"context"
	"strings"
	"testing"
)

// A released mini app may call another released mini app. Nothing stops that
// call from pointing back at the caller, so the interpreter needs its own depth
// limit: without one a self-referential release recurses until the goroutine
// stack is exhausted and the process dies.
func TestMiniAppStepStopsAtNestingLimit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewStore(root)

	app := greetingApp()
	app.ID = "self-caller"
	app.Inputs = nil
	app.Workflow = []Step{{
		ID: "recurse", Kind: "miniapp", Title: "Call myself",
		AppID: "self-caller", AppVersion: "1.0.0",
	}}
	app.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
		Kind: "step", Step: "recurse", Status: "succeeded",
	}}}
	app.Outputs = nil

	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	draft, err := store.GetDraft(app.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if err := store.RecordPassingTest(app.ID, Run{
		ID: "run-seed", AppID: app.ID, Revision: draft.Revision,
		Test: true, Status: RunSucceeded,
	}); err != nil {
		t.Fatalf("record passing test: %v", err)
	}
	if _, err := store.Release(app.ID, "1.0.0"); err != nil {
		t.Fatalf("release: %v", err)
	}

	runner := NewRunner(store, nil)
	_, err = runner.RunRelease(context.Background(), app.ID, "1.0.0", nil, nil)
	if err == nil {
		t.Fatal("a self-referential mini app must not run forever")
	}
	if !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("err = %v, want a nesting-limit error", err)
	}
}
