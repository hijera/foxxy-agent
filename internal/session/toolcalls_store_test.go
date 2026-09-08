package session

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
)

func TestMarkToolCallFinishedPreservesStartedAt(t *testing.T) {
	t.Parallel()
	sd := t.TempDir()
	id := "call_test_1"
	if err := MarkToolCallStarted(sd, id, "grep", "tool", "in_progress"); err != nil {
		t.Fatalf("MarkToolCallStarted: %v", err)
	}
	before, err := ReadToolCallMeta(sd, id)
	if err != nil {
		t.Fatalf("ReadToolCallMeta after start: %v", err)
	}
	if before.StartedAt == "" {
		t.Fatal("expected StartedAt after MarkToolCallStarted")
	}
	time.Sleep(2 * time.Millisecond)
	if err := MarkToolCallFinished(sd, id, "grep", "tool", "completed"); err != nil {
		t.Fatalf("MarkToolCallFinished: %v", err)
	}
	after, err := ReadToolCallMeta(sd, id)
	if err != nil {
		t.Fatalf("ReadToolCallMeta after finish: %v", err)
	}
	if after.StartedAt != before.StartedAt {
		t.Fatalf("StartedAt changed: before %q after %q", before.StartedAt, after.StartedAt)
	}
	if after.FinishedAt == "" {
		t.Fatal("expected FinishedAt after MarkToolCallFinished")
	}
	st0, err0 := time.Parse(time.RFC3339, after.StartedAt)
	st1, err1 := time.Parse(time.RFC3339, after.FinishedAt)
	if err0 != nil || err1 != nil {
		t.Fatalf("parse RFC3339: started %v finished %v", err0, err1)
	}
	if !st1.After(st0) && !st1.Equal(st0) {
		t.Fatalf("FinishedAt should be >= StartedAt: %v %v", st0, st1)
	}
}

func TestWriteToolCallPlanSnapshotPersistsFinalTodoState(t *testing.T) {
	t.Parallel()
	sd := t.TempDir()
	entries := []acp.PlanEntry{
		{Content: "Inspect tool cards", Status: "completed"},
		{Content: "Render todo preview", Status: "in_progress"},
	}
	if err := MarkToolCallFinished(sd, "todo-1", "foxxycode_todo_item_update", "todo", "completed"); err != nil {
		t.Fatalf("MarkToolCallFinished: %v", err)
	}
	if err := WriteToolCallPlanSnapshot(sd, "todo-1", entries); err != nil {
		t.Fatalf("WriteToolCallPlanSnapshot: %v", err)
	}

	meta, err := ReadToolCallMeta(sd, "todo-1")
	if err != nil {
		t.Fatalf("ReadToolCallMeta: %v", err)
	}
	if meta.Status != "completed" || meta.Name != "foxxycode_todo_item_update" {
		t.Fatalf("tool metadata was overwritten: %+v", meta)
	}
	if len(meta.PlanSnapshot) != len(entries) || meta.PlanSnapshot[1].Status != "in_progress" {
		t.Fatalf("PlanSnapshot = %+v, want %+v", meta.PlanSnapshot, entries)
	}
}

func TestMarkToolCallFinishedPreservesPlanSnapshot(t *testing.T) {
	t.Parallel()
	entries := []acp.PlanEntry{
		{Content: "Inspect tool cards", Status: "completed"},
		{Content: "Render todo preview", Status: "in_progress"},
	}
	for _, status := range []string{"completed", "failed", "cancelled"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			sd := t.TempDir()
			id := "todo-" + status
			if err := MarkToolCallStarted(sd, id, "foxxycode_todo_item_update", "todo", "in_progress"); err != nil {
				t.Fatalf("MarkToolCallStarted: %v", err)
			}
			started, err := ReadToolCallMeta(sd, id)
			if err != nil {
				t.Fatalf("ReadToolCallMeta after start: %v", err)
			}
			if err := WriteToolCallPlanSnapshot(sd, id, entries); err != nil {
				t.Fatalf("WriteToolCallPlanSnapshot: %v", err)
			}
			if err := MarkToolCallFinished(sd, id, "foxxycode_todo_item_update", "todo", status); err != nil {
				t.Fatalf("MarkToolCallFinished: %v", err)
			}
			meta, err := ReadToolCallMeta(sd, id)
			if err != nil {
				t.Fatalf("ReadToolCallMeta after finish: %v", err)
			}
			if meta.Status != status {
				t.Fatalf("Status = %q, want %q", meta.Status, status)
			}
			if len(meta.PlanSnapshot) != len(entries) || meta.PlanSnapshot[1].Status != "in_progress" {
				t.Fatalf("PlanSnapshot lost across MarkToolCallFinished: %+v", meta.PlanSnapshot)
			}
			if meta.StartedAt != started.StartedAt {
				t.Fatalf("StartedAt changed: %q -> %q", started.StartedAt, meta.StartedAt)
			}
			if meta.FinishedAt == "" {
				t.Fatal("expected FinishedAt after MarkToolCallFinished")
			}
		})
	}
}

func TestWriteToolCallPlanSnapshotBeforeAnyMetaCreatesIt(t *testing.T) {
	t.Parallel()
	sd := t.TempDir()
	entries := []acp.PlanEntry{{Content: "Only item", Status: "pending"}}
	if err := WriteToolCallPlanSnapshot(sd, "todo-fresh", entries); err != nil {
		t.Fatalf("WriteToolCallPlanSnapshot without meta: %v", err)
	}
	meta, err := ReadToolCallMeta(sd, "todo-fresh")
	if err != nil {
		t.Fatalf("ReadToolCallMeta: %v", err)
	}
	if meta.ToolCallID != "todo-fresh" || meta.Version != toolCallMetaVersion {
		t.Fatalf("meta not initialised: %+v", meta)
	}
	if len(meta.PlanSnapshot) != 1 || meta.PlanSnapshot[0].Content != "Only item" {
		t.Fatalf("PlanSnapshot = %+v", meta.PlanSnapshot)
	}
}

func TestAttachTodoPlanMetaKeepsExistingPreviewMeta(t *testing.T) {
	t.Parallel()
	entries := []acp.PlanEntry{{Content: "Only item", Status: "pending"}}
	base := map[string]interface{}{
		"foxxycode": map[string]interface{}{
			"toolResultPreview": map[string]interface{}{"truncated": true},
		},
	}
	out := AttachTodoPlanMeta(base, entries)
	fox, _ := out["foxxycode"].(map[string]interface{})
	if fox == nil {
		t.Fatalf("foxxycode meta missing: %+v", out)
	}
	if _, ok := fox["toolResultPreview"]; !ok {
		t.Fatalf("toolResultPreview dropped: %+v", fox)
	}
	got, ok := fox["todoPlan"].([]acp.PlanEntry)
	if !ok || len(got) != 1 || got[0].Content != "Only item" {
		t.Fatalf("todoPlan = %#v", fox["todoPlan"])
	}

	if out := AttachTodoPlanMeta(nil, nil); out != nil {
		t.Fatalf("empty entries should leave nil meta untouched, got %+v", out)
	}
	if out := AttachTodoPlanMeta(nil, entries); out == nil {
		t.Fatal("entries with nil meta should create the envelope")
	} else if fox, _ := out["foxxycode"].(map[string]interface{}); fox == nil || fox["todoPlan"] == nil {
		t.Fatalf("todoPlan missing from created envelope: %+v", out)
	}
}

// NeuralDeep-hosted models (kimi-k2.6 among them) return harmony-style ids such as
// "functions.foxxycode_todo_plan_replace:0". A colon is not a legal path character
// on Windows, so the per-call store used to fail silently for every such call.
func TestToolCallStoreAcceptsIdsWithPathUnsafeCharacters(t *testing.T) {
	t.Parallel()
	sd := t.TempDir()
	id := "functions.foxxycode_todo_item_update:3"
	if err := MarkToolCallStarted(sd, id, "foxxycode_todo_item_update", "todo", "in_progress"); err != nil {
		t.Fatalf("MarkToolCallStarted: %v", err)
	}
	if err := WriteToolCallArgs(sd, id, `{"index":1,"status":"completed"}`); err != nil {
		t.Fatalf("WriteToolCallArgs: %v", err)
	}
	if err := WriteToolCallPlanSnapshot(sd, id, []acp.PlanEntry{{Content: "one", Status: "completed"}}); err != nil {
		t.Fatalf("WriteToolCallPlanSnapshot: %v", err)
	}
	if err := MarkToolCallFinished(sd, id, "foxxycode_todo_item_update", "todo", "completed"); err != nil {
		t.Fatalf("MarkToolCallFinished: %v", err)
	}
	meta, err := ReadToolCallMeta(sd, id)
	if err != nil {
		t.Fatalf("ReadToolCallMeta: %v", err)
	}
	if meta.ToolCallID != id || meta.Status != "completed" || len(meta.PlanSnapshot) != 1 {
		t.Fatalf("meta = %+v", meta)
	}
	if args, err := ReadToolCallArgs(sd, id); err != nil || !strings.Contains(args, `"index"`) {
		t.Fatalf("args = %q err=%v", args, err)
	}
	// Two ids that differ only in unsafe characters must not share a directory.
	other := "functions.foxxycode_todo_item_update/3"
	if err := MarkToolCallStarted(sd, other, "grep", "tool", "in_progress"); err != nil {
		t.Fatalf("MarkToolCallStarted other: %v", err)
	}
	if again, err := ReadToolCallMeta(sd, id); err != nil || again.Name != "foxxycode_todo_item_update" {
		t.Fatalf("unsafe ids collided: meta=%+v err=%v", again, err)
	}
	// Plain ids keep their literal directory, so stores written before this
	// change stay readable.
	plainDir, err := toolCallDir(sd, "call_abc-123")
	if err != nil || filepath.Base(plainDir) != "call_abc-123" {
		t.Fatalf("plain id dir = %q err=%v", plainDir, err)
	}
	unsafeDir, _ := toolCallDir(sd, id)
	if strings.ContainsAny(filepath.Base(unsafeDir), `:/\<>"|?*`) {
		t.Fatalf("unsafe characters survived in dir name %q", unsafeDir)
	}
}
