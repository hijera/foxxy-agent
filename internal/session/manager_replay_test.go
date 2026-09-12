package session

import (
	"context"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type replayCaptureSender struct {
	mu  sync.Mutex
	ups []interface{}
}

func (c *replayCaptureSender) SendSessionUpdate(_ string, u interface{}) error {
	c.mu.Lock()
	c.ups = append(c.ups, u)
	c.mu.Unlock()
	return nil
}

func (c *replayCaptureSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow"}, nil
}

func (c *replayCaptureSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

// A reopened ACP session replays its transcript; a todo card only keeps its
// plan rows when the replayed tool_call_update carries the persisted snapshot.
func TestReplayConversationAttachesTodoPlanMetaFromToolCallMeta(t *testing.T) {
	sd := t.TempDir()
	snapshot := []acp.PlanEntry{
		{Content: "first", Status: "completed"},
		{Content: "second", Status: "pending"},
	}
	if err := MarkToolCallStarted(sd, "todo-1", "foxxycode_todo_item_update", "todo", "in_progress"); err != nil {
		t.Fatalf("MarkToolCallStarted: %v", err)
	}
	if err := WriteToolCallPlanSnapshot(sd, "todo-1", snapshot); err != nil {
		t.Fatalf("WriteToolCallPlanSnapshot: %v", err)
	}
	if err := MarkToolCallFinished(sd, "todo-1", "foxxycode_todo_item_update", "todo", "completed"); err != nil {
		t.Fatalf("MarkToolCallFinished: %v", err)
	}
	if err := MarkToolCallStarted(sd, "grep-1", "grep", "tool", "in_progress"); err != nil {
		t.Fatalf("MarkToolCallStarted grep: %v", err)
	}
	if err := MarkToolCallFinished(sd, "grep-1", "grep", "tool", "completed"); err != nil {
		t.Fatalf("MarkToolCallFinished grep: %v", err)
	}
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "plan it"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "todo-1", Name: "foxxycode_todo_item_update", InputJSON: `{"index":0,"status":"completed"}`}}},
		{Role: llm.RoleTool, ToolCallID: "todo-1", Content: "updated item 0"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "grep-1", Name: "grep", InputJSON: `{"pattern":"x"}`}}},
		{Role: llm.RoleTool, ToolCallID: "grep-1", Content: "no matches"},
	}

	sender := &replayCaptureSender{}
	m := &Manager{server: sender}
	if err := m.replayConversation("sess_replay_todo", msgs, sd); err != nil {
		t.Fatalf("replayConversation: %v", err)
	}

	var todoUpdate, grepUpdate *acp.ToolCallStatusUpdate
	for _, u := range sender.ups {
		upd, ok := u.(acp.ToolCallStatusUpdate)
		if !ok {
			continue
		}
		switch upd.ToolCallID {
		case "todo-1":
			todoUpdate = &upd
		case "grep-1":
			grepUpdate = &upd
		}
	}
	if todoUpdate == nil || grepUpdate == nil {
		t.Fatalf("missing replayed tool_call_update: todo=%v grep=%v", todoUpdate, grepUpdate)
	}
	fox, _ := todoUpdate.Meta["foxxycode"].(map[string]interface{})
	got, _ := fox["todoPlan"].([]acp.PlanEntry)
	if len(got) != 2 || got[1].Content != "second" {
		t.Fatalf("replayed todoPlan = %#v (meta %+v)", fox["todoPlan"], todoUpdate.Meta)
	}
	if fox, _ := grepUpdate.Meta["foxxycode"].(map[string]interface{}); fox != nil {
		if _, has := fox["todoPlan"]; has {
			t.Fatalf("grep update must not carry todoPlan: %+v", fox)
		}
	}
}
