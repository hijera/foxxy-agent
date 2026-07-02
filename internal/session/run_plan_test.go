package session_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxy-agent/internal/acp"
	"github.com/hijera/foxxy-agent/internal/plans"
	"github.com/hijera/foxxy-agent/internal/session"
)

func TestRunPlanSlugFromPromptMeta(t *testing.T) {
	meta := map[string]interface{}{plans.MetaRunPlanSlug: "auth-refactor"}
	if got := session.RunPlanSlugFromPromptMeta(meta); got != "auth-refactor" {
		t.Fatalf("got %q", got)
	}
	if got := session.RunPlanSlugFromPromptMeta(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractRunPlanSlugFromPromptText(t *testing.T) {
	got := session.ExtractRunPlanSlugFromPromptText("Please @plans/auth-refactor.plan.md")
	if got != "auth-refactor" {
		t.Fatalf("mention: %q", got)
	}
	got = session.ExtractRunPlanSlugFromPromptText("implement the plan my-feature")
	if got != "my-feature" {
		t.Fatalf("phrase: %q", got)
	}
}

func TestRunPlanDoesNotSetTodo(t *testing.T) {
	cfg := testConfig()
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	mgr := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), t.TempDir(), store)
	ctx := context.Background()
	st, err := mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := st.SessionID
	state := mgr.SessionByID(id)
	if state == nil {
		t.Fatal("no state")
	}
	state.SetPlan([]acp.PlanEntry{{Content: "keep me", Status: "pending"}})
	dir, err := store.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Write(dir, "run-me", plans.DefaultContent("run-me", "Run me")); err != nil {
		t.Fatal(err)
	}
	before := state.GetPlan()
	_, err = mgr.HandleSessionPrompt(ctx, acp.SessionPromptParams{
		SessionID: id,
		Meta:      map[string]interface{}{plans.MetaRunPlanSlug: "run-me"},
		Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "Implement the plan."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := state.GetPlan()
	if len(before) != len(after) || before[0].Content != after[0].Content {
		t.Fatalf("plan changed: before %+v after %+v", before, after)
	}
	if state.GetMode() != string(session.ModeAgent) {
		t.Fatalf("mode: %s", state.GetMode())
	}
}
