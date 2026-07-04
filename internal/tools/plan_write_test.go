package tools_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

func TestPlanWritePublishesDesignPlan(t *testing.T) {
	dir := t.TempDir()
	var gotPlan acp.PlanUpdate
	var persisted bool
	sender := &planCaptureSender{}
	env := &tools.Env{
		SessionID:  "sess_test",
		SessionDir: dir,
		Sender:     sender,
		PersistPlanDocument: func(doc plans.Document) {
			persisted = doc.Slug == "demo"
		},
	}
	env.SendDesignPlanUpdate = func(doc plans.Document) {
		tools.SendDesignPlanUpdate(env, doc)
	}
	tool := tools.PlanWriteTool()
	content := plans.DefaultContent("demo", "Demo plan")
	raw, _ := json.Marshal(content)
	_, err := tool.Execute(context.Background(), `{"slug":"demo","content":`+string(raw)+`}`, env)
	if err != nil {
		t.Fatal(err)
	}
	sender.mu.Lock()
	gotPlan = sender.plan
	sender.mu.Unlock()
	if gotPlan.Meta[plans.MetaPlanKind] != plans.PlanKindDesign {
		t.Fatalf("meta: %+v", gotPlan.Meta)
	}
	if gotPlan.Meta[plans.MetaPlanSlug] != "demo" {
		t.Fatalf("slug meta: %+v", gotPlan.Meta)
	}
	if !persisted {
		t.Fatal("expected persist callback")
	}
	if _, err := plans.Read(dir, "demo"); err != nil {
		t.Fatal(err)
	}
}

type planCaptureSender struct {
	mu   sync.Mutex
	plan acp.PlanUpdate
}

func (c *planCaptureSender) SendSessionUpdate(_ string, u interface{}) error {
	if pu, ok := u.(acp.PlanUpdate); ok {
		c.mu.Lock()
		c.plan = pu
		c.mu.Unlock()
	}
	return nil
}

func (c *planCaptureSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow"}, nil
}

func (c *planCaptureSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}
