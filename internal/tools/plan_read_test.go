package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/plans"
	"github.com/EvilFreelancer/coddy-agent/internal/tools"
)

func TestPlanReadReturnsDesignPlanContent(t *testing.T) {
	dir := t.TempDir()
	slug := "my-plan"
	body := plans.DefaultContent(slug, "My plan")
	if _, err := plans.Write(dir, slug, body); err != nil {
		t.Fatal(err)
	}
	tool := tools.PlanReadTool()
	raw, _ := json.Marshal(map[string]string{"slug": slug})
	out, err := tool.Execute(context.Background(), string(raw), &tools.Env{
		SessionID:  "sess_test",
		SessionDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != body {
		t.Fatalf("content mismatch:\ngot: %q\nwant: %q", out, body)
	}
}

func TestPlanReadMissingSlug(t *testing.T) {
	tool := tools.PlanReadTool()
	_, err := tool.Execute(context.Background(), `{}`, &tools.Env{
		SessionDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for empty slug")
	}
}

func TestPlanReadNotFound(t *testing.T) {
	tool := tools.PlanReadTool()
	_, err := tool.Execute(context.Background(), `{"slug":"missing"}`, &tools.Env{
		SessionDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
