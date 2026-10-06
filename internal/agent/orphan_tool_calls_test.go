package agent

// Every tool_call_id an assistant message announces must get a result, or
// OpenAI-compatible endpoints reject the next request in the conversation. A
// cancelled stream hands back every accumulated tool-call builder, including one
// whose JSON arguments were cut mid-write, so a partial persist must never carry
// them - and a session already holding such a message has to keep working.

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func orphanToolResult(id string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: id, Content: "done"}
}

func assistantWithCalls(text string, ids ...string) llm.Message {
	m := llm.Message{Role: llm.RoleAssistant, Content: text}
	for _, id := range ids {
		m.ToolCalls = append(m.ToolCalls, llm.ToolCall{ID: id, Name: "read", InputJSON: `{"path":"a.go"}`})
	}
	return m
}

func callIDs(msgs []llm.Message) []string {
	var out []string
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			out = append(out, tc.ID)
		}
	}
	return out
}

// TestDropUnansweredToolCallsHealsAPoisonedHistory is the repair for sessions
// already on disk: an announced call with no result is removed from the payload
// rather than sent and rejected.
func TestDropUnansweredToolCallsHealsAPoisonedHistory(t *testing.T) {
	in := []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
		assistantWithCalls("looking", "call_ok", "call_orphan"),
		orphanToolResult("call_ok"),
		{Role: llm.RoleUser, Content: "and again"},
	}
	got := dropUnansweredToolCalls(in)

	if ids := callIDs(got); len(ids) != 1 || ids[0] != "call_ok" {
		t.Errorf("surviving tool calls = %v, want [call_ok]", ids)
	}
	if len(got) != len(in) {
		t.Errorf("message count = %d, want %d (only the call is dropped)", len(got), len(in))
	}
	if got[1].Content != "looking" {
		t.Errorf("assistant text = %q, want it preserved", got[1].Content)
	}
	// Copy-on-write: the caller's slice must be untouched.
	if len(in[1].ToolCalls) != 2 {
		t.Error("the input history was mutated")
	}
}

// TestDropUnansweredToolCallsRemovesAnEmptiedMessage covers the worst shape: a
// cancelled stream that produced only a half-written call. Left behind, it is an
// assistant message with no text, no reasoning and no valid calls.
func TestDropUnansweredToolCallsRemovesAnEmptiedMessage(t *testing.T) {
	in := []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
		assistantWithCalls("", "call_orphan"),
		{Role: llm.RoleUser, Content: "still there?"},
	}
	got := dropUnansweredToolCalls(in)

	for _, m := range got {
		if m.Role == llm.RoleAssistant {
			t.Fatalf("an assistant message left with nothing must be dropped, got %+v", m)
		}
	}
	if len(got) != 2 {
		t.Errorf("message count = %d, want 2", len(got))
	}
}

// TestDropUnansweredToolCallsLeavesAHealthyHistoryAlone guards against the repair
// firing during a normal turn, where every call is answered.
func TestDropUnansweredToolCallsLeavesAHealthyHistoryAlone(t *testing.T) {
	in := []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
		assistantWithCalls("looking", "a", "b"),
		orphanToolResult("a"),
		orphanToolResult("b"),
		{Role: llm.RoleAssistant, Content: "done"},
	}
	got := dropUnansweredToolCalls(in)
	if len(got) != len(in) || len(callIDs(got)) != 2 {
		t.Errorf("a healthy history was altered: %d msgs, calls %v", len(got), callIDs(got))
	}
}

// TestCancelledStreamPersistsNoToolCalls stops the poison at the source: the
// arguments of a call cut mid-write are invalid JSON, and replaying an invalid
// call is worse than losing it - the rule openai_stream.go already applies to a
// truncated stream.
func TestCancelledStreamPersistsNoToolCalls(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{deltaThenErr: "About to call a tool", err: nil, cancelWithToolCall: true},
	}}
	h := newStallHarness(t, p, nil)

	if _, err := h.run(t); err != nil {
		t.Logf("run ended with %v (expected for a cancelled stream)", err)
	}
	for _, m := range h.st.GetMessages() {
		if m.Role == llm.RoleAssistant && len(m.ToolCalls) > 0 {
			t.Fatalf("a cancelled stream persisted %d tool call(s) with no result: %+v",
				len(m.ToolCalls), m.ToolCalls)
		}
	}
	// The text the user watched arrive still survives.
	var sawText bool
	for _, m := range h.st.GetMessages() {
		if m.Role == llm.RoleAssistant && strings.Contains(m.Content, "About to call a tool") {
			sawText = true
		}
	}
	if !sawText {
		t.Error("the partial answer was lost along with the tool call")
	}
}
