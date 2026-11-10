package session

// This file covers batch-local call-ID reuse and results that belong to a
// different or misplaced assistant batch.

import (
	"reflect"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestToolPairingAllowsCompletedBatchIDReuse(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("call_a"),
		toolResultMessage("call_a"),
		{Role: llm.RoleUser, Content: "next"},
		toolCallMessage("call_a"),
		toolResultMessage("call_a"),
	}

	if issues := ValidateToolPairing(msgs); len(issues) != 0 {
		t.Fatalf("issues = %#v, want clean pairing", issues)
	}
	got, issues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(got, msgs) {
		t.Fatalf("repair changed messages: got %#v, want %#v", got, msgs)
	}
	if len(issues) != 0 {
		t.Fatalf("repair issues = %#v, want none", issues)
	}
}

func TestToolPairingDuplicateCallIDWithinBatchIsNotRepaired(t *testing.T) {
	msgs := []llm.Message{toolCallMessage("call_a", "call_a")}

	issues := ValidateToolPairing(msgs)
	if len(issues) != 1 || issues[0].Kind != toolPairingDuplicateCallID {
		t.Fatalf("issues = %#v, want one duplicate_call_id issue", issues)
	}
	if issues[0].ToolCallID != "call_a" || issues[0].MessageIndex != 0 {
		t.Fatalf("duplicate issue = %#v, want call_a at assistant index 0", issues[0])
	}

	got, repairIssues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(got, msgs) {
		t.Fatalf("repair changed duplicate calls: got %#v, want %#v", got, msgs)
	}
	if !reflect.DeepEqual(repairIssues, issues) {
		t.Fatalf("repair issues = %#v, want %#v", repairIssues, issues)
	}
}

func TestToolPairingRepeatedIDAcrossBatchesGetsBatchLocalRepair(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("call_a"),
		{Role: llm.RoleUser, Content: "next"},
		toolCallMessage("call_a"),
		toolResultMessage("call_a"),
	}

	issues := ValidateToolPairing(msgs)
	want := []ToolPairingIssue{{Kind: toolPairingMissingResult, ToolCallID: "call_a", MessageIndex: 0}}
	assertToolPairingIssues(t, issues, want...)
	got, repairIssues := RepairMissingToolResults(msgs)
	if len(got) != len(msgs)+1 || got[1].Role != llm.RoleTool || got[1].ToolCallID != "call_a" {
		t.Fatalf("repair did not close the first batch locally: got %#v", got)
	}
	assertToolPairingIssues(t, repairIssues, want...)
}

func TestToolPairingUniqueResultAfterUserIsMisplacedAndNotRepaired(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("call_a"),
		{Role: llm.RoleUser, Content: "next"},
		toolResultMessage("call_a"),
	}

	want := []ToolPairingIssue{
		{Kind: toolPairingMissingResult, ToolCallID: "call_a", MessageIndex: 0},
		{Kind: toolPairingMisplacedResult, ToolCallID: "call_a", MessageIndex: 2},
	}
	assertToolPairingIssues(t, ValidateToolPairing(msgs), want...)
	got, issues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(got, msgs) {
		t.Fatalf("repair moved misplaced result: got %#v, want %#v", got, msgs)
	}
	assertToolPairingIssues(t, issues, want...)
}

func TestToolPairingDoesNotMoveResultsOwnedBySeparateBatches(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("call_a"),
		{Role: llm.RoleUser, Content: "next"},
		toolCallMessage("call_b"),
		toolResultMessage("call_a"),
		toolResultMessage("call_b"),
	}

	want := []ToolPairingIssue{
		{Kind: toolPairingMissingResult, ToolCallID: "call_a", MessageIndex: 0},
		{Kind: toolPairingMisplacedResult, ToolCallID: "call_a", MessageIndex: 3},
	}
	assertToolPairingIssues(t, ValidateToolPairing(msgs), want...)
	got, issues := RepairMissingToolResults(msgs)
	if !reflect.DeepEqual(got, msgs) {
		t.Fatalf("repair moved results between batches: got %#v, want %#v", got, msgs)
	}
	assertToolPairingIssues(t, issues, want...)
}

// A call id pending in two batches is not ambiguous: each batch is closed with
// its own synthetic result, batch-locally, the way a completed reuse already
// is.
func TestToolPairingRepairsPendingBatchesWithReusedID(t *testing.T) {
	msgs := []llm.Message{
		toolCallMessage("a"),
		{Role: llm.RoleUser, Content: "next"},
		toolCallMessage("a"),
	}
	got, issues := RepairMissingToolResults(msgs)
	if len(got) != len(msgs)+2 || len(issues) == 0 {
		t.Fatalf("repair = %d messages, issues=%+v; want two synthetic results", len(got), issues)
	}
	if got[1].ToolCallID != "a" || got[4].ToolCallID != "a" ||
		got[1].Content != interruptedToolCallResult || got[4].Content != interruptedToolCallResult {
		t.Fatalf("synthetic results misplaced: %#v", got)
	}
	if remaining := ValidateToolPairing(got); len(remaining) != 0 {
		t.Fatalf("repaired history still invalid: %+v", remaining)
	}
}

// A stray result carrying the id of pending calls cannot be attributed to one
// batch, so nothing is repaired on a guess. A batch repeating an id inside a
// single assistant message cannot tell one call's result from the other's
// either.
func TestToolPairingAmbiguousPendingBatchesAreNotRepaired(t *testing.T) {
	for _, msgs := range [][]llm.Message{
		{
			toolCallMessage("a"),
			{Role: llm.RoleUser, Content: "next"},
			toolCallMessage("a"),
			{Role: llm.RoleUser, Content: "more"},
			toolResultMessage("a"),
		},
		{toolCallMessage("a", "a", "b")},
	} {
		got, issues := RepairMissingToolResults(msgs)
		if !reflect.DeepEqual(got, msgs) || len(issues) == 0 || len(ValidateToolPairing(got)) == 0 {
			t.Fatalf("ambiguous history repaired or accepted: got=%+v issues=%+v", got, issues)
		}
	}
}
