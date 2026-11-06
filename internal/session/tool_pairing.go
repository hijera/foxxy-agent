package session

// This file validates assistant tool-call batches and builds a deterministic
// outbound repair for missing, unambiguous tool results.

import (
	"sort"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// ToolPairingIssueKind identifies a transcript tool-call pairing problem.
type ToolPairingIssueKind string

const (
	toolPairingMissingResult   ToolPairingIssueKind = "missing_result"
	toolPairingOrphanResult    ToolPairingIssueKind = "orphan_result"
	toolPairingDuplicateResult ToolPairingIssueKind = "duplicate_result"
	toolPairingEmptyCallID     ToolPairingIssueKind = "empty_call_id"
	toolPairingDuplicateCallID ToolPairingIssueKind = "duplicate_call_id"
	toolPairingMisplacedResult ToolPairingIssueKind = "misplaced_result"
)

// interruptedToolCallResult is deliberately stable: it is part of the
// provider-facing history projection and must not change between repairs.
const interruptedToolCallResult = "no result was recorded: the call may or may not have run; check the current state before running it again"

// ToolPairingIssue describes one malformed or incomplete tool-call pairing.
type ToolPairingIssue struct {
	Kind         ToolPairingIssueKind
	ToolCallID   string
	MessageIndex int
}

type toolCallOccurrence struct {
	id    string
	batch int
}

type toolBatch struct {
	assistant int
	end       int
	calls     []toolCallOccurrence
	results   map[string]int
}

type toolPairingAnalysis struct {
	batches      []toolBatch
	calls        map[string][]toolCallOccurrence
	resultBatch  map[int]int
	issues       []ToolPairingIssue
	validResults map[int]map[string]bool
	strayResults map[string]bool
}

func analyzeToolPairing(msgs []llm.Message) toolPairingAnalysis {
	a := toolPairingAnalysis{
		calls:        make(map[string][]toolCallOccurrence),
		resultBatch:  make(map[int]int),
		validResults: make(map[int]map[string]bool),
		strayResults: make(map[string]bool),
	}
	for i := 0; i < len(msgs); {
		if msgs[i].Role != llm.RoleAssistant || len(msgs[i].ToolCalls) == 0 {
			i++
			continue
		}
		batchIndex := len(a.batches)
		batch := toolBatch{assistant: i, end: i + 1, results: make(map[string]int)}
		seen := make(map[string]bool, len(msgs[i].ToolCalls))
		for _, call := range msgs[i].ToolCalls {
			id := call.ID
			if strings.TrimSpace(id) == "" {
				a.issues = append(a.issues, ToolPairingIssue{Kind: toolPairingEmptyCallID, MessageIndex: i})
				continue
			}
			occurrence := toolCallOccurrence{id: id, batch: batchIndex}
			if seen[id] {
				a.issues = append(a.issues, ToolPairingIssue{Kind: toolPairingDuplicateCallID, ToolCallID: id, MessageIndex: i})
			}
			seen[id] = true
			a.calls[id] = append(a.calls[id], occurrence)
			batch.calls = append(batch.calls, occurrence)
		}
		for batch.end < len(msgs) && msgs[batch.end].Role == llm.RoleTool {
			id := msgs[batch.end].ToolCallID
			a.resultBatch[batch.end] = batchIndex
			if _, duplicate := batch.results[id]; duplicate {
				a.issues = append(a.issues, ToolPairingIssue{Kind: toolPairingDuplicateResult, ToolCallID: id, MessageIndex: batch.end})
			} else {
				batch.results[id] = batch.end
			}
			batch.end++
		}
		if len(batch.results) > 0 {
			a.validResults[batchIndex] = make(map[string]bool, len(batch.results))
		}
		a.batches = append(a.batches, batch)
		i = batch.end
	}
	// Classify results in transcript order. A result outside its owning batch is
	// misplaced when its ID names a call in the window, otherwise it is orphaned.
	for index, msg := range msgs {
		if msg.Role != llm.RoleTool {
			continue
		}
		batchIndex, inBatch := a.resultBatch[index]
		if inBatch {
			batch := a.batches[batchIndex]
			if batch.results[msg.ToolCallID] != index {
				continue // duplicate_result was emitted while scanning the batch.
			}
			matched := false
			for _, call := range batch.calls {
				if call.id == msg.ToolCallID {
					matched = true
					break
				}
			}
			if matched {
				a.validResults[batchIndex][msg.ToolCallID] = true
				continue
			}
		}
		a.strayResults[msg.ToolCallID] = true
		kind := toolPairingOrphanResult
		if len(a.calls[msg.ToolCallID]) > 0 {
			kind = toolPairingMisplacedResult
		}
		a.issues = append(a.issues, ToolPairingIssue{Kind: kind, ToolCallID: msg.ToolCallID, MessageIndex: index})
	}
	for batchIndex, batch := range a.batches {
		for _, call := range batch.calls {
			if batchHasDuplicateCall(batch, call.id) {
				continue
			}
			if a.validResults[batchIndex][call.id] {
				continue
			}
			a.issues = append(a.issues, ToolPairingIssue{Kind: toolPairingMissingResult, ToolCallID: call.id, MessageIndex: batch.assistant})
		}
	}
	sort.SliceStable(a.issues, func(i, j int) bool {
		return a.issues[i].MessageIndex < a.issues[j].MessageIndex
	})
	return a
}

func batchHasDuplicateCall(batch toolBatch, id string) bool {
	seen := false
	for _, call := range batch.calls {
		if call.id != id {
			continue
		}
		if seen {
			return true
		}
		seen = true
	}
	return false
}

// ValidateToolPairing checks assistant tool-call batches and their contiguous
// tool results without changing the transcript.
func ValidateToolPairing(msgs []llm.Message) []ToolPairingIssue {
	return analyzeToolPairing(msgs).issues
}

// RepairMissingToolResults inserts stable synthetic results for calls that
// have no result of their own. A call stays unrepaired when a stray result
// carries its id (the real one cannot be attributed to a batch) or when a
// single batch repeats the id (one result cannot be told from the other).
// Existing malformed, orphan, and duplicate messages are kept in place and
// unchanged.
func RepairMissingToolResults(msgs []llm.Message) ([]llm.Message, []ToolPairingIssue) {
	a := analyzeToolPairing(msgs)
	missing := make(map[int][]string)
	for batchIndex, batch := range a.batches {
		ambiguous := false
		for _, call := range batch.calls {
			if batchHasDuplicateCall(batch, call.id) {
				ambiguous = true
				break
			}
		}
		if ambiguous {
			continue
		}
		for _, call := range batch.calls {
			if !a.validResults[batchIndex][call.id] && !a.strayResults[call.id] {
				missing[batchIndex] = append(missing[batchIndex], call.id)
			}
		}
	}
	if len(missing) == 0 {
		return msgs, a.issues
	}
	repaired := make([]llm.Message, 0, len(msgs)+len(missing))
	batchIndex := 0
	for i := 0; i < len(msgs); {
		if batchIndex < len(a.batches) && a.batches[batchIndex].assistant == i {
			batch := a.batches[batchIndex]
			repaired = append(repaired, msgs[i:batch.end]...)
			for _, id := range missing[batchIndex] {
				repaired = append(repaired, llm.Message{Role: llm.RoleTool, ToolCallID: id, Content: interruptedToolCallResult})
			}
			i = batch.end
			batchIndex++
			continue
		}
		repaired = append(repaired, msgs[i])
		i++
	}
	return repaired, a.issues
}
