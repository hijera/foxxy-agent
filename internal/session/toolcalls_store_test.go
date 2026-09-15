package session

import (
	"os"
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

// The persisted arguments keep their number literals: a permission resume
// binds to this file, so an integer past 2^53 must not come back rounded.
func TestWriteToolCallArgsKeepsLargeIntegers(t *testing.T) {
	dir := t.TempDir()
	if err := WriteToolCallArgs(dir, "call_big", `{"n":9007199254740993,"command":"echo x"}`); err != nil {
		t.Fatal(err)
	}
	got, err := ReadToolCallArgs(dir, "call_big")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "9007199254740993") {
		t.Fatalf("the persisted arguments must keep the literal, got %q", got)
	}
	if !strings.Contains(got, "\n  \"n\": ") {
		t.Fatalf("the persisted arguments are pretty-printed, got %q", got)
	}
}

// A tool call id arrives from the model's provider, so it is never trusted as a
// path: an id carrying a traversal or a separator must keep its record inside the
// bundle instead of writing a folder next to it.
func TestToolCallStoreKeepsUnsafeIDsInsideTheBundle(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"../../escaped", "../escaped", "a/b", `a\b`, ".hidden", "..", strings.Repeat("z", 300)} {
		root := t.TempDir()
		sd := filepath.Join(root, "store", "bundle")
		if err := os.MkdirAll(sd, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := MarkToolCallStarted(sd, id, "read", "tool", "in_progress"); err != nil {
			t.Fatalf("MarkToolCallStarted(%q): %v", id, err)
		}
		if err := WriteToolCallArgs(sd, id, `{"path":"note.txt"}`); err != nil {
			t.Fatalf("WriteToolCallArgs(%q): %v", id, err)
		}
		if err := WriteToolCallResult(sd, id, "PAYLOAD"); err != nil {
			t.Fatalf("WriteToolCallResult(%q): %v", id, err)
		}
		if err := MarkToolCallFinished(sd, id, "read", "tool", "completed"); err != nil {
			t.Fatalf("MarkToolCallFinished(%q): %v", id, err)
		}

		// Nothing outside the bundle: the only new entry under the store root
		// is the bundle itself, and the bundle holds exactly one tool call.
		entries, err := os.ReadDir(filepath.Join(root, "store"))
		if err != nil {
			t.Fatalf("ReadDir(store): %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "bundle" {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Fatalf("id %q wrote outside the bundle: store holds %v", id, names)
		}
		if rootEntries, err := os.ReadDir(root); err != nil {
			t.Fatalf("ReadDir(root): %v", err)
		} else if len(rootEntries) != 1 || rootEntries[0].Name() != "store" {
			t.Fatalf("id %q wrote above the store: root holds %d entries", id, len(rootEntries))
		}
		dirs, err := ListToolCalls(sd)
		if err != nil {
			t.Fatalf("ListToolCalls(%q): %v", id, err)
		}
		if len(dirs) != 1 {
			t.Fatalf("id %q produced %d tool call folders, want 1: %v", id, len(dirs), dirs)
		}
		if err := ValidateToolCallID(dirs[0]); err != nil {
			t.Fatalf("id %q produced an unsafe folder name %q: %v", id, dirs[0], err)
		}

		// The record reads back under the id the provider sent, and the raw id
		// is what meta.json carries.
		if got, err := ReadToolCallResult(sd, id); err != nil || strings.TrimSpace(got) != "PAYLOAD" {
			t.Fatalf("ReadToolCallResult(%q) = %q, %v", id, got, err)
		}
		if got, err := ReadToolCallArgs(sd, id); err != nil || !strings.Contains(got, "note.txt") {
			t.Fatalf("ReadToolCallArgs(%q) = %q, %v", id, got, err)
		}
		meta, err := ReadToolCallMeta(sd, id)
		if err != nil {
			t.Fatalf("ReadToolCallMeta(%q): %v", id, err)
		}
		if meta.ToolCallID != id {
			t.Fatalf("meta.json lost the raw id: got %q, want %q", meta.ToolCallID, id)
		}
		if meta.Status != "completed" || strings.TrimSpace(meta.StartedAt) == "" {
			t.Fatalf("id %q lost its bookkeeping: %+v", id, meta)
		}

		// A folder name handed back by the listing resolves to the same folder,
		// so reading a bundle through ListToolCalls is not a second mapping.
		if got, err := ReadToolCallResult(sd, dirs[0]); err != nil || strings.TrimSpace(got) != "PAYLOAD" {
			t.Fatalf("ReadToolCallResult(%q) through the folder name = %q, %v", dirs[0], got, err)
		}
	}
}

// An empty id is refused outright: there is no folder to put the record in, and
// the callers already treat the error as "this call was not persisted".
func TestToolCallStoreRefusesEmptyID(t *testing.T) {
	t.Parallel()
	sd := t.TempDir()
	for _, id := range []string{"", "   "} {
		if err := WriteToolCallResult(sd, id, "x"); err == nil {
			t.Fatalf("WriteToolCallResult(%q): expected an error", id)
		}
		if _, err := ReadToolCallArgs(sd, id); err == nil {
			t.Fatalf("ReadToolCallArgs(%q): expected an error", id)
		}
	}
}

// Releases before the safe-segment rule stored an id with path-unsafe characters
// under the id rewritten to [A-Za-z0-9._-] plus the first four bytes of its SHA-256.
// NeuralDeep-hosted models answer with such ids ("functions.<tool>:<n>"), so bundles
// on disk are full of those folders: the rule must resolve an id to the very folder
// an earlier release wrote, or every todo-plan snapshot of those turns goes missing
// on reload.
func TestToolCallDirNameResolvesTheFolderEarlierReleasesWrote(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"functions.foxxycode_todo_plan_replace:0": "functions.foxxycode_todo_plan_replace_0-ac70644e",
		"functions.foxxycode_todo_item_update:3":  "functions.foxxycode_todo_item_update_3-14ec3709",
	}
	for id, folder := range cases {
		if got := ToolCallDirName(id); got != folder {
			t.Errorf("ToolCallDirName(%q) = %q, want the folder earlier releases wrote, %q", id, got, folder)
		}
	}

	// A bundle laid out by such a release reads back through the raw id.
	sd := t.TempDir()
	const id = "functions.foxxycode_todo_plan_replace:0"
	dir := filepath.Join(sd, "tool_calls", cases[id])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.md"), []byte("PLAN\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadToolCallResult(sd, id); err != nil || strings.TrimSpace(got) != "PLAN" {
		t.Fatalf("ReadToolCallResult(%q) = %q, %v; the folder an earlier release wrote is not found", id, got, err)
	}
}

// A provider id that is already a safe folder name keeps it, so bundles written
// before the mapping existed read back unchanged.
func TestToolCallDirNameKeepsSafeIDsVerbatim(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"call_abc123", "toolu_01A09q90qw90lq917835lq9", "chatcmpl-tool.9f0e", "0"} {
		if got := ToolCallDirName(id); got != id {
			t.Fatalf("ToolCallDirName(%q) = %q, want the id itself", id, got)
		}
	}
	// The same unusable id always maps to the same folder, and two of them do
	// not share one.
	a, b := ToolCallDirName("../../x"), ToolCallDirName("../../y")
	if a == "" || a == b {
		t.Fatalf("derived names must be stable and distinct: %q, %q", a, b)
	}
	if got := ToolCallDirName("../../x"); got != a {
		t.Fatalf("derived name is not stable: %q then %q", a, got)
	}
	if err := ValidateToolCallID(a); err != nil {
		t.Fatalf("derived name %q is not a safe folder name: %v", a, err)
	}
	if got := ToolCallDirName(a); got != a {
		t.Fatalf("resolving a derived name again must be a no-op: %q -> %q", a, got)
	}
}
