package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// delegateFailSender fails the delegate round trip the way a broken transport
// does, or answers with a handled error like a client that tried and failed.
type delegateFailSender struct {
	resumePermissionSender
	res acp.ToolExecutionResult
	err error
}

func (s *delegateFailSender) ExecuteToolCall(context.Context, acp.ToolExecutionRequest) (acp.ToolExecutionResult, error) {
	return s.res, s.err
}

// delegateHangSender blocks like a client that never answers until the
// context ends.
type delegateHangSender struct {
	resumePermissionSender
}

func (delegateHangSender) ExecuteToolCall(ctx context.Context, _ acp.ToolExecutionRequest) (acp.ToolExecutionResult, error) {
	<-ctx.Done()
	return acp.ToolExecutionResult{Status: acp.ToolExecNotHandled}, ctx.Err()
}

func newDelegateTestAgent(t *testing.T, sender acp.UpdateSender) *Agent {
	t.Helper()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 6},
	}
	st := &session.State{ID: "sess_delegate", CWD: t.TempDir(), Mode: session.ModeAgent}
	return NewAgent(cfg, st, sender, nil)
}

func TestTryDelegateToolCallFallsBackWhenTheDelegateFails(t *testing.T) {
	ag := newDelegateTestAgent(t, &delegateFailSender{err: errors.New("stream is gone")})
	handled, _, err := ag.tryDelegateToolCall(context.Background(), llm.ToolCall{ID: "c1", Name: "write", InputJSON: "{}"}, nil, "sess")
	if handled || err != nil {
		t.Fatalf("handled=%v err=%v, want a quiet fallback", handled, err)
	}
}

func TestTryDelegateToolCallSurfacesAClientError(t *testing.T) {
	ag := newDelegateTestAgent(t, &delegateFailSender{res: acp.ToolExecutionResult{Status: acp.ToolExecHandled, Err: "conflict: buffer changed"}})
	handled, result, err := ag.tryDelegateToolCall(context.Background(), llm.ToolCall{ID: "c1", Name: "write", InputJSON: "{}"}, nil, "sess")
	if !handled {
		t.Fatal("a handled error must not fall back to a local run")
	}
	if result != "" || err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("result=%q err=%v, want the client's error", result, err)
	}
}

func TestTryDelegateToolCallPropagatesTurnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ag := newDelegateTestAgent(t, delegateHangSender{})
	handled, _, err := ag.tryDelegateToolCall(ctx, llm.ToolCall{ID: "c1", Name: "write", InputJSON: "{}"}, nil, "sess")
	if !handled {
		t.Fatal("a cancelled turn must not fall back to a local run")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestTryDelegateToolCallSkipsSubagentChildren(t *testing.T) {
	sender := &delegateFailSender{res: acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "from client"}}
	ag := newDelegateTestAgent(t, sender)
	ag.subagent = &session.SubagentMeta{}
	handled, _, err := ag.tryDelegateToolCall(context.Background(), llm.ToolCall{ID: "c1", Name: "write", InputJSON: "{}"}, nil, "sess")
	if handled || err != nil {
		t.Fatalf("handled=%v err=%v, want the child's call to run locally", handled, err)
	}
}

// A delegated filesystem write must fire the same OnFileEdit hook the local
// write would have, so the transcript row and edit_applied event are identical
// - with the preview computed before the client ran the call.
func TestTryDelegateToolCallFiresOnFileEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("old body"), 0o644); err != nil {
		t.Fatal(err)
	}
	sender := &delegateFailSender{res: acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "applied"}}
	ag := newDelegateTestAgent(t, sender)

	var gotTool, gotPath string
	var gotBefore, gotAfter string
	env := &tooling.Env{
		CWD: dir,
		OnFileEdit: func(toolName, absPath string, before, after []byte) {
			gotTool, gotPath = toolName, absPath
			gotBefore, gotAfter = string(before), string(after)
		},
	}
	args := `{"path":"f.txt","oldString":"old","newString":"new"}`
	handled, _, err := ag.tryDelegateToolCall(context.Background(), llm.ToolCall{ID: "c1", Name: "edit", InputJSON: args}, env, "sess")
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if gotTool != "edit" || gotPath != path || gotBefore != "old body" || gotAfter != "new body" {
		t.Fatalf("OnFileEdit got tool=%q path=%q before=%q after=%q", gotTool, gotPath, gotBefore, gotAfter)
	}
}
