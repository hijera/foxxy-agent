package session

// This file tests transcript pairing validation, deterministic repair, and
// preservation of malformed history.

import (
	"reflect"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func toolCallMessage(ids ...string) llm.Message {
	calls := make([]llm.ToolCall, 0, len(ids))
	for _, id := range ids {
		calls = append(calls, llm.ToolCall{ID: id, Name: "test"})
	}
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: calls}
}

func toolResultMessage(id string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: id, Content: "result:" + id}
}

func assertToolPairingIssues(t *testing.T, got []ToolPairingIssue, want ...ToolPairingIssue) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("issues = %#v, want %#v", got, want)
	}
}

func TestRepairMissingToolResultsCompleteBatchUnchanged(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "start"},
		toolCallMessage("call_1", "call_2"),
		toolResultMessage("call_1"),
		toolResultMessage("call_2"),
		{Role: llm.RoleAssistant, Content: "done"},
	}

	got, issues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(got, msgs) {
		t.Fatalf("messages changed: got %#v, want %#v", got, msgs)
	}
	assertToolPairingIssues(t, issues)
	assertToolPairingIssues(t, ValidateToolPairing(got))
}

func TestRepairMissingToolResultsInsertsAfterExistingToolResults(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "start"},
		toolCallMessage("call_1", "call_2", "call_3"),
		toolResultMessage("call_1"),
		toolResultMessage("call_3"),
		{Role: llm.RoleUser, Content: "next"},
	}
	want := []llm.Message{
		{Role: llm.RoleUser, Content: "start"},
		toolCallMessage("call_1", "call_2", "call_3"),
		toolResultMessage("call_1"),
		toolResultMessage("call_3"),
		{Role: llm.RoleTool, ToolCallID: "call_2", Content: interruptedToolCallResult},
		{Role: llm.RoleUser, Content: "next"},
	}

	got, issues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
	if len(issues) != 1 || string(issues[0].Kind) != "missing_result" {
		t.Fatalf("issues = %#v, want one missing_result issue", issues)
	}
	if issues[0].ToolCallID != "call_2" || issues[0].MessageIndex != 1 {
		t.Fatalf("missing issue = %#v, want call_2 at assistant index 1", issues[0])
	}
	assertToolPairingIssues(t, ValidateToolPairing(got))
}

func TestRepairMissingToolResultsIsIdempotent(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("call_1", "call_2"),
		toolResultMessage("call_1"),
		{Role: llm.RoleAssistant, Content: "after"},
	}

	once, firstIssues := RepairMissingToolResults(msgs)
	twice, secondIssues := RepairMissingToolResults(once)
	if !reflect.DeepEqual(twice, once) {
		t.Fatalf("second repair changed messages: got %#v, want %#v", twice, once)
	}
	if len(firstIssues) != 1 || string(firstIssues[0].Kind) != "missing_result" {
		t.Fatalf("first repair issues = %#v, want one missing_result issue", firstIssues)
	}
	assertToolPairingIssues(t, secondIssues)
}

func TestRepairMissingToolResultsPreservesFollowingUserMessage(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "before"},
		toolCallMessage("call_1"),
		{Role: llm.RoleUser, Content: "following"},
	}

	got, _ := RepairMissingToolResults(msgs)
	if len(got) != 4 {
		t.Fatalf("message count = %d, want 4", len(got))
	}
	if got[2].Role != llm.RoleTool || got[2].ToolCallID != "call_1" {
		t.Fatalf("inserted message = %#v, want tool result for call_1", got[2])
	}
	if !reflect.DeepEqual(got[3], msgs[2]) {
		t.Fatalf("following user message = %#v, want %#v", got[3], msgs[2])
	}
}

func TestValidateToolPairingReportsOrphanResult(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "start"},
		toolResultMessage("orphan"),
	}

	issues := ValidateToolPairing(msgs)
	if len(issues) != 1 || string(issues[0].Kind) != "orphan_result" {
		t.Fatalf("issues = %#v, want one orphan_result issue", issues)
	}
	if issues[0].ToolCallID != "orphan" || issues[0].MessageIndex != 1 {
		t.Fatalf("orphan issue = %#v, want orphan at index 1", issues[0])
	}
}

func TestValidateToolPairingReportsDuplicateResult(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("call_1"),
		toolResultMessage("call_1"),
		toolResultMessage("call_1"),
	}

	issues := ValidateToolPairing(msgs)
	if len(issues) != 1 || string(issues[0].Kind) != "duplicate_result" {
		t.Fatalf("issues = %#v, want one duplicate_result issue", issues)
	}
	if issues[0].ToolCallID != "call_1" || issues[0].MessageIndex != 2 {
		t.Fatalf("duplicate issue = %#v, want call_1 at index 2", issues[0])
	}
}

func TestRepairMissingToolResultsDoesNotDuplicateLateResult(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("call_1"),
		{Role: llm.RoleUser, Content: "following"},
		toolResultMessage("call_1"),
	}

	got, issues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(got, msgs) {
		t.Fatalf("messages changed: got %#v, want %#v", got, msgs)
	}
	found := false
	for _, issue := range issues {
		if string(issue.Kind) == "misplaced_result" && issue.ToolCallID == "call_1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want misplaced_result for call_1", issues)
	}
}

func TestValidateToolPairingReportsInvalidAssistantCallsWithoutRewriting(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("", "call_1", "call_1"),
		toolResultMessage("call_1"),
	}
	original := append([]llm.Message(nil), msgs...)

	issues := ValidateToolPairing(msgs)
	if !reflect.DeepEqual(msgs, original) {
		t.Fatalf("validation rewrote messages: got %#v, want %#v", msgs, original)
	}
	want := []ToolPairingIssue{
		{Kind: "empty_call_id", MessageIndex: 0},
		{Kind: "duplicate_call_id", ToolCallID: "call_1", MessageIndex: 0},
	}
	assertToolPairingIssues(t, issues, want...)

	repaired, repairIssues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(repaired, original) {
		t.Fatalf("repair rewrote invalid calls: got %#v, want %#v", repaired, original)
	}
	if len(repairIssues) != len(issues) {
		t.Fatalf("repair issues = %#v, want validation issues %#v", repairIssues, issues)
	}
}
