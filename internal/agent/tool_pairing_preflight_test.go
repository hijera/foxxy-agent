package agent

// This file tests provider preflight refusal, compaction recovery, and
// interruption finalization at the agent boundary.

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestToolPairingPreflightRefusesBeforeProviderAndCompactionRecovers(t *testing.T) {
	for _, history := range [][]llm.Message{
		{{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "a"}, {ID: "a"}}}},
		{{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: ""}}}},
		{{Role: llm.RoleTool, ToolCallID: "orphan", Content: "real result"}},
	} {
		st := &session.State{ID: "pairing-invalid", CWD: t.TempDir(), Mode: session.ModeAgent, Messages: history}
		provider := &pairingProvider{legacy: true}
		a := newPairingAgent(provider, st, resumePermissionSender{})
		stop, err := a.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "continue"}})
		if err == nil || stop != string(acp.StopReasonRefused) || !strings.Contains(err.Error(), "/compact") || !strings.Contains(err.Error(), st.ID) {
			t.Fatalf("malformed history not refused with recovery hint: %q, %v", stop, err)
		}
		if provider.calls != 0 {
			t.Fatalf("provider received %d invalid requests", provider.calls)
		}
		keep := 0
		summarizer := &compactCannedProvider{t: t, summary: "Earlier history had an interrupted tool batch."}
		compactor := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, summarizer)
		if _, err := compactor.CompactSession(context.Background(), CompactOptions{Force: true}); err != nil {
			t.Fatalf("compaction could not recover malformed history: %v", err)
		}
		if len(summarizer.requests) == 0 {
			t.Fatal("compaction did not call the text-only summarizer")
		}
		if issues := session.ValidateToolPairing(session.MessagesForLLM(st.GetMessages())); len(issues) != 0 {
			t.Fatalf("compaction left invalid outbound history: %+v", issues)
		}
	}
}

func TestToolPairingPermissionClosurePreservesLateRealSibling(t *testing.T) {
	real := llm.Message{Role: llm.RoleTool, ToolCallID: "sibling", Content: "write succeeded", Rules: "rule", CreatedAt: "stamp"}
	st := &session.State{ID: "late-sibling", CWD: t.TempDir(), Mode: session.ModeAgent, Messages: []llm.Message{
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "target"}, {ID: "sibling"}}},
		{Role: llm.RoleUser, Content: "next"},
		real,
		{Role: llm.RoleTool, ToolCallID: "target", Content: "approved result"},
	}}
	a := newPairingAgent(&pairingProvider{legacy: true}, st, resumePermissionSender{})
	a.closeUnexecutedPermissionBatch("target")
	count := 0
	for _, msg := range st.GetMessages() {
		if msg.Role == llm.RoleTool && msg.ToolCallID == "sibling" {
			count++
			if !reflect.DeepEqual(msg, real) {
				t.Fatalf("real result replaced: %+v", msg)
			}
		}
	}
	if count != 1 {
		t.Fatalf("sibling has %d results, want its real result only", count)
	}
	if issues := session.ValidateToolPairing(st.GetMessages()); len(issues) == 0 {
		t.Fatal("misplaced real result must remain visible to preflight, not be silently moved")
	}
}

func TestToolPairingLoopAbortClosesStoredCalls(t *testing.T) {
	st := &session.State{ID: "loop-abort", CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	a := newPairingAgent(&pairingProvider{legacy: true}, st, resumePermissionSender{})
	var reasoning strings.Builder
	a.persistLoopAbortedMessage("fake/model", &llm.Response{ToolCalls: []llm.ToolCall{{ID: "unstarted", Name: "read"}}}, &reasoning, time.Time{}, time.Time{}, 3)
	messages := st.GetMessages()
	if len(messages) != 2 || messages[1].Content != toolCallInterruptedResult || len(session.ValidateToolPairing(messages)) != 0 {
		t.Fatalf("loop abort left incomplete history: %+v", messages)
	}
	meta, err := session.ReadToolCallMeta(st.SessionDir, "unstarted")
	if err != nil || meta.Status != "cancelled" {
		t.Fatalf("cancelled tool metadata = %+v, %v", meta, err)
	}
}

func TestToolPairingSkippedCallWithoutIDDoesNotInventResult(t *testing.T) {
	st := &session.State{ID: "empty-id", CWD: t.TempDir(), Mode: session.ModeAgent}
	a := newPairingAgent(&pairingProvider{legacy: true}, st, resumePermissionSender{})
	a.recordSkippedToolCalls(nil, []llm.ToolCall{{Name: "read"}}, toolCallInterruptedResult)
	if len(st.GetMessages()) != 0 {
		t.Fatalf("empty ID received a fabricated result: %+v", st.GetMessages())
	}
}
