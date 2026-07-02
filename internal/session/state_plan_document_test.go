package session

import (
	"testing"

	"github.com/hijera/foxxy-agent/internal/plans"
)

func TestMarkPlanDocumentDiscardedPersistsInMessages(t *testing.T) {
	st := &State{SessionDir: t.TempDir()}
	st.AppendPlanDocument(plans.Document{
		Slug:    "old-plan",
		Name:    "Old",
		Content: "---\nname: Old\n---\nbody\n",
	})
	st.MarkPlanDocumentDiscarded("old-plan")
	msgs := st.GetMessages()
	if len(msgs) != 1 || msgs[0].PlanDocument == nil {
		t.Fatalf("messages: %+v", msgs)
	}
	if !msgs[0].PlanDocument.Discarded {
		t.Fatal("expected discarded flag on plan_document")
	}
	slugs := st.DiscardedPlanSlugs()
	if len(slugs) != 1 || slugs[0] != "old-plan" {
		t.Fatalf("DiscardedPlanSlugs: %v", slugs)
	}
}
