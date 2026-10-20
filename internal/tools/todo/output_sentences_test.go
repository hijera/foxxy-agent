package todo_test

import (
	"context"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	apptools "github.com/hijera/foxxycode-agent/internal/tools"
	"github.com/hijera/foxxycode-agent/internal/tools/todo"
)

// The web UI renders the todo tools as cards by recognising the exact sentences
// their success paths return (external/ui/src/ui/chat/workspaceToolDisplay.ts).
// A rewording silently degrades those cards to raw text, so the copy is pinned.
func TestSuccessSentences(t *testing.T) {
	twoEntries := func() []acp.PlanEntry {
		return []acp.PlanEntry{
			{Content: "Inspect", Status: "completed"},
			{Content: "Build", Status: "in_progress"},
		}
	}
	run := func(t *testing.T, plan []acp.PlanEntry, name, args string, tune func(*apptools.Env)) string {
		t.Helper()
		sender := &mockSender{}
		entries := plan
		env := envWithPlan(sender, &entries)
		if tune != nil {
			tune(env)
		}
		out, err := newRegistry().Execute(context.Background(), name, args, env)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}

	t.Run("item add", func(t *testing.T) {
		out := run(t, twoEntries(), todo.ToolNameItemAdd, `{"content":"Ship"}`, nil)
		if out != "added plan item (3 rows total)" {
			t.Fatalf("item_add = %q", out)
		}
	})

	t.Run("item remove", func(t *testing.T) {
		out := run(t, twoEntries(), todo.ToolNameItemRemove, `{"index":0}`, nil)
		if out != "removed item at index 0 (1 items remaining)" {
			t.Fatalf("item_remove = %q", out)
		}
	})

	t.Run("item move", func(t *testing.T) {
		out := run(t, twoEntries(), todo.ToolNameItemMove, `{"from_index":0,"to_index":1}`, nil)
		if out != "moved item from index 0 to index 1" {
			t.Fatalf("item_move = %q", out)
		}
	})

	t.Run("item move unchanged", func(t *testing.T) {
		out := run(t, twoEntries(), todo.ToolNameItemMove, `{"from_index":1,"to_index":1}`, nil)
		if out != "item 1 unchanged (from_index equals to_index)" {
			t.Fatalf("item_move unchanged = %q", out)
		}
	})

	t.Run("plan archive without a session dir", func(t *testing.T) {
		out := run(t, twoEntries(), todo.ToolNamePlanArchive, `{}`, nil)
		want := "archived plan state (2 items finalized); active plan cleared (no session dir for file archive)"
		if out != want {
			t.Fatalf("plan_archive = %q", out)
		}
	})

	t.Run("plan archive to a file", func(t *testing.T) {
		out := run(t, twoEntries(), todo.ToolNamePlanArchive, `{}`, func(env *apptools.Env) {
			env.SessionDir = "sessions/sess_test"
			env.WriteArchivedPlanMarkdown = func(string) (string, error) {
				return "todos/archive/plan_1.md", nil
			}
		})
		want := "archived plan (2 items) to todos/archive/plan_1.md; active plan cleared"
		if out != want {
			t.Fatalf("plan_archive = %q", out)
		}
	})

	t.Run("plan archive with no active items", func(t *testing.T) {
		out := run(t, nil, todo.ToolNamePlanArchive, `{}`, nil)
		if out != "plan archive: no active items (nothing to write)" {
			t.Fatalf("plan_archive = %q", out)
		}
	})
}
