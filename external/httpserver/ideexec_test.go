//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
)

type ideExecOutcome struct {
	res acp.ToolExecutionResult
	err error
}

func TestIdeExecuteToolCallRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	ch := ideEvents.SubscribeHandled([]string{"write"})
	defer ideEvents.Unsubscribe(ch)

	s := NewSender(&config.Config{}, nil, false, "m")
	s.SetCWD(dir)

	done := make(chan ideExecOutcome, 1)
	go func() {
		res, err := s.ExecuteToolCall(context.Background(), acp.ToolExecutionRequest{
			SessionID:  "sess1",
			ToolCallID: "call1",
			ToolName:   "write",
			ArgsJSON:   `{"path":"f.txt","content":"hello"}`,
		})
		done <- ideExecOutcome{res, err}
	}()

	ev := recvIdeEvent(t, ch)
	if ev.Type != "tool_execute" || ev.SessionID != "sess1" || ev.ToolCallID != "call1" {
		t.Fatalf("meta wrong: %+v", ev)
	}
	if ev.Nonce == "" {
		t.Fatal("tool_execute carries no nonce")
	}
	if ev.ToolName != "write" {
		t.Fatalf("tool = %q", ev.ToolName)
	}
	if !strings.Contains(ev.ArgsJSON, "hello") {
		t.Fatalf("args = %q", ev.ArgsJSON)
	}
	if ev.Path != path || ev.After != "hello" {
		t.Fatalf("preview wrong: path=%q after=%q", ev.Path, ev.After)
	}
	// Offering the call must not write anything: the client (or the local
	// fallback) performs it.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("ExecuteToolCall touched the file: %v", err)
	}

	// A result that does not echo the nonce must not resolve the call.
	if CompleteToolExecResult("call1", "wrong", acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "x"}) {
		t.Fatal("wrong nonce resolved the pending call")
	}
	select {
	case out := <-done:
		t.Fatalf("call resolved by a wrong-nonce result: %+v", out)
	default:
	}

	if !CompleteToolExecResult("call1", ev.Nonce, acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "applied in editor"}) {
		t.Fatal("valid result did not resolve the pending call")
	}
	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("ExecuteToolCall: %v", out.err)
		}
		if out.res.Status != acp.ToolExecHandled || out.res.Output != "applied in editor" {
			t.Fatalf("result = %+v", out.res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the delegated result")
	}
}

func TestIdeExecuteToolCallNotHandledWithoutCapableClient(t *testing.T) {
	// A plain subscriber watches events but executes nothing.
	ch := ideEvents.Subscribe()
	defer ideEvents.Unsubscribe(ch)

	s := NewSender(&config.Config{}, nil, false, "m")
	res, err := s.ExecuteToolCall(context.Background(), acp.ToolExecutionRequest{
		SessionID: "sess1", ToolCallID: "call1", ToolName: "write", ArgsJSON: `{}`,
	})
	if err != nil {
		t.Fatalf("ExecuteToolCall: %v", err)
	}
	if res.Status != acp.ToolExecNotHandled {
		t.Fatalf("status = %q, want not_handled", res.Status)
	}
	select {
	case ev := <-ch:
		t.Fatalf("broadcast to a client that cannot execute: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestIdeExecuteToolCallTimeoutFallsBack(t *testing.T) {
	ch := ideEvents.SubscribeHandled([]string{"*"})
	defer ideEvents.Unsubscribe(ch)

	s := NewSender(&config.Config{}, nil, false, "m")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	res, err := s.ExecuteToolCall(ctx, acp.ToolExecutionRequest{
		SessionID: "sess1", ToolCallID: "call1", ToolName: "run_command", ArgsJSON: `{"command":"ls"}`,
	})
	if err == nil {
		t.Fatal("expected the deadline error")
	}
	if res.Status != acp.ToolExecNotHandled {
		t.Fatalf("status = %q, want not_handled on timeout", res.Status)
	}
	// Non-write tools carry no diff preview.
	ev := recvIdeEvent(t, ch)
	if ev.Type != "tool_execute" || ev.Path != "" {
		t.Fatalf("got %+v", ev)
	}
}

func TestIdeToolResultRouteRejectsUnknownCall(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"toolCallId": "nope", "nonce": "nope", "status": "handled", "output": "x",
	})
	rec := httptest.NewRecorder()
	foxxycodeIdeToolResult(rec, httptest.NewRequest("POST", "/foxxycode/ide/tool-result", strings.NewReader(string(body))))
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestIdeToolResultRouteValidatesStatus(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"toolCallId": "c", "nonce": "n", "status": "maybe",
	})
	rec := httptest.NewRecorder()
	foxxycodeIdeToolResult(rec, httptest.NewRequest("POST", "/foxxycode/ide/tool-result", strings.NewReader(string(body))))
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestIdeToolResultRouteResolvesPendingCall(t *testing.T) {
	ch := registerToolExecWait("call2", "nonce2")
	defer unregisterToolExecWait("call2")

	body, _ := json.Marshal(map[string]string{
		"toolCallId": "call2", "nonce": "nonce2", "status": "handled", "output": "done in editor",
	})
	rec := httptest.NewRecorder()
	foxxycodeIdeToolResult(rec, httptest.NewRequest("POST", "/foxxycode/ide/tool-result", strings.NewReader(string(body))))
	if rec.Code != 204 {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	select {
	case res := <-ch:
		if res.Status != acp.ToolExecHandled || res.Output != "done in editor" {
			t.Fatalf("result = %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the route did not resolve the pending call")
	}
}
