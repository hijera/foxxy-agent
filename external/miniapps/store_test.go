//go:build miniapps

package miniapps

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDraftRevisionConflictAndAtomicPersistence(t *testing.T) {
	store := NewStore(t.TempDir())
	app := coreValidApp()
	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}
	draft, err := store.GetDraft(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Revision == "" {
		t.Fatal("draft revision is empty")
	}
	if _, err := os.Stat(filepath.Join(store.Root(), app.ID, "catalog.json")); err != nil {
		t.Fatalf("catalog was not persisted: %v", err)
	}

	stale := draft
	stale.Metadata.Description = "first edit"
	if err := store.PutDraft(app.ID, stale); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetDraft(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision == draft.Revision {
		t.Fatal("draft revision did not change")
	}
	stale.Metadata.Description = "stale edit"
	if err := store.PutDraft(app.ID, stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale save error = %v, want revision conflict", err)
	}
	if _, err := store.UpdateDraft(app.ID, "wrong", updated); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("explicit stale save error = %v, want revision conflict", err)
	}
}

func TestStoreReleaseRequiresPassingCurrentRevisionAndApproval(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.CreateDraft(coreValidApp(), nil); err != nil {
		t.Fatal(err)
	}
	draft, err := store.GetDraft("file-transform")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Release("file-transform", "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: draft.Revision}); !errors.Is(err, ErrReleaseGate) {
		t.Fatalf("release without test error = %v, want release gate", err)
	}
	passing := Run{ID: "test-run", AppID: draft.ID, Revision: draft.Revision, Test: true, Status: RunSucceeded}
	if err := store.RecordPassingTest(draft.ID, passing); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Release(draft.ID, "1.0.0"); !errors.Is(err, ErrReleaseApproval) {
		t.Fatalf("unapproved release error = %v, want approval error", err)
	}
	if _, err := store.Release(draft.ID, "1.0.0", ReleaseOptions{Approved: true}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("release without expected revision error = %v, want revision conflict", err)
	}
	released, err := store.Release(draft.ID, "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: draft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), draft.ID, "draft", "authoring", "sanitization.json")); err != nil {
		t.Fatalf("sanitization report was not persisted: %v", err)
	}
	if released.State != StateReleased || released.Version != "1.0.0" {
		t.Fatalf("released = %#v", released)
	}
	if _, err := store.Release(draft.ID, "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: draft.Revision}); !errors.Is(err, ErrVersionExists) {
		t.Fatalf("duplicate release error = %v, want version exists", err)
	}
	if _, err := store.Release(draft.ID, "0.9.0", ReleaseOptions{Approved: true, ExpectedRevision: draft.Revision}); !errors.Is(err, ErrReleaseGate) {
		t.Fatalf("decreasing release error = %v, want release gate", err)
	}
}

func TestStorePassingTestMustMatchCurrentDraft(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.CreateDraft(coreValidApp(), nil); err != nil {
		t.Fatal(err)
	}
	draft, err := store.GetDraft("file-transform")
	if err != nil {
		t.Fatal(err)
	}
	bad := Run{ID: "run-1", AppID: draft.ID, Revision: "stale", Test: true, Status: RunSucceeded}
	if err := store.RecordPassingTest(draft.ID, bad); !errors.Is(err, ErrReleaseGate) {
		t.Fatalf("stale passing test error = %v, want release gate", err)
	}
	bad.Revision = draft.Revision
	bad.Test = false
	if err := store.RecordPassingTest(draft.ID, bad); !errors.Is(err, ErrReleaseGate) {
		t.Fatalf("non-test passing run error = %v, want release gate", err)
	}
}

func TestStorePersistsRevisionBoundRepairProposals(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.CreateDraft(coreValidApp(), nil); err != nil {
		t.Fatal(err)
	}
	draft, err := store.GetDraft("file-transform")
	if err != nil {
		t.Fatal(err)
	}
	proposal := RepairProposal{
		ID: "patch-fix-output", AppID: draft.ID, BaseRevision: draft.Revision,
		Summary: "Fix output", Operations: []RepairOperation{{Path: "/metadata/description", Value: "updated"}},
	}
	if err := store.SaveRepairProposal(draft.ID, proposal); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetRepairProposal(draft.ID, proposal.ID)
	if err != nil || loaded.BaseRevision != draft.Revision {
		t.Fatalf("loaded proposal = %+v, error=%v", loaded, err)
	}
	items, err := store.ListRepairProposals(draft.ID)
	if err != nil || len(items) != 1 || items[0].ID != proposal.ID {
		t.Fatalf("proposal list = %+v, error=%v", items, err)
	}
	for index := 2; index <= MaxRepairProposals; index++ {
		proposal.ID = fmt.Sprintf("patch-fix-output-%d", index)
		if err := store.SaveRepairProposal(draft.ID, proposal); err != nil {
			t.Fatalf("save repair proposal %d: %v", index, err)
		}
	}
	proposal.ID = "patch-over-budget"
	if err := store.SaveRepairProposal(draft.ID, proposal); !errors.Is(err, ErrReleaseGate) {
		t.Fatalf("repair proposal over budget error = %v, want release gate", err)
	}

	draft.Metadata.Description = "new revision"
	updated, err := store.UpdateDraft(draft.ID, draft.Revision, draft)
	if err != nil {
		t.Fatal(err)
	}
	proposal.BaseRevision = draft.Revision
	if err := store.SaveRepairProposal(updated.ID, proposal); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale proposal save error = %v, want revision conflict", err)
	}
}

func TestStoreReleaseRejectsUnredactedPrivateEvidence(t *testing.T) {
	store := NewStore(t.TempDir())
	evidence := &SourceEvidence{SessionID: "source-session", SourceFixture: map[string]any{"token": "actual-secret-value"}}
	if err := store.CreateDraft(coreValidApp(), evidence); err != nil {
		t.Fatal(err)
	}
	draft, err := store.GetDraft("file-transform")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPassingTest(draft.ID, Run{ID: "test-secret-evidence", AppID: draft.ID, Revision: draft.Revision, Test: true, Status: RunSucceeded}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Release(draft.ID, "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: draft.Revision}); !errors.Is(err, ErrReleaseGate) {
		t.Fatalf("release error = %v, want evidence sanitization gate", err)
	}
}

func TestStoreDraftEditPreservesReleasedCatalogVersion(t *testing.T) {
	store := NewStore(t.TempDir())
	app := coreValidApp()
	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}
	draft, err := store.GetDraft(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPassingTest(app.ID, Run{ID: "catalog-test", AppID: app.ID, Revision: draft.Revision, Test: true, Status: RunSucceeded}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReleaseWithOptions(app.ID, "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: draft.Revision}); err != nil {
		t.Fatal(err)
	}
	draft.Metadata.Description = "edited after release"
	if _, err := store.UpdateDraft(app.ID, draft.Revision, draft); err != nil {
		t.Fatal(err)
	}
	items, err := store.List("", true)
	if err != nil || len(items) != 1 {
		t.Fatalf("catalog = %+v, error=%v", items, err)
	}
	if items[0].State != StateReleased || items[0].Version != "1.0.0" {
		t.Fatalf("released catalog metadata lost after draft edit: %+v", items[0])
	}
}
