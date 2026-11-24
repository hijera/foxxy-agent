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
			Cwd:        dir,
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
	if ev.Cwd != dir {
		t.Fatalf("cwd = %q, want the session workspace", ev.Cwd)
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

func TestIdeExecuteToolCallFullQueueFallsBack(t *testing.T) {
	ch := ideEvents.SubscribeHandled([]string{"write"})
	defer ideEvents.Unsubscribe(ch)
	for i := 0; i < cap(ch); i++ {
		ch <- ideEvent{Type: "edit_applied"}
	}
	s := NewSender(&config.Config{}, nil, false, "m")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := s.ExecuteToolCall(ctx, acp.ToolExecutionRequest{
		SessionID: "full-queue", ToolCallID: "full-call", ToolName: "write", ArgsJSON: `{}`,
	})
	if err != nil || res.Status != acp.ToolExecNotHandled {
		t.Fatalf("undelivered call should decline without waiting for the deadline: %+v, %v", res, err)
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

func TestIdeExecuteToolCallDeclinesAnotherWorkspace(t *testing.T) {
	ch := ideEvents.SubscribeHandled([]string{"write"})
	defer ideEvents.Unsubscribe(ch)

	dir := t.TempDir()
	s := NewSender(&config.Config{}, nil, false, "m")
	s.SetCWD(dir)
	res, err := s.ExecuteToolCall(context.Background(), acp.ToolExecutionRequest{
		SessionID: "sess1", ToolCallID: "call1", ToolName: "write", ArgsJSON: `{}`,
		Cwd: t.TempDir(), // a session that moved to another checkout (a worktree)
	})
	if err != nil {
		t.Fatalf("ExecuteToolCall: %v", err)
	}
	if res.Status != acp.ToolExecNotHandled {
		t.Fatalf("status = %q, want not_handled for a foreign workspace", res.Status)
	}
	select {
	case ev := <-ch:
		t.Fatalf("a call from another workspace was offered: %+v", ev)
	case <-time.After(150 * time.Millisecond):
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
	defer unregisterToolExecWait("call2", ch)

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

func TestIdeExecuteToolCallCarriesTheDeadline(t *testing.T) {
	ch := ideEvents.SubscribeHandled([]string{"write"})
	defer ideEvents.Unsubscribe(ch)

	s := NewSender(&config.Config{}, nil, false, "m")
	s.SetCWD(t.TempDir())
	want := time.Now().Add(500 * time.Millisecond).Truncate(time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), want)
	defer cancel()
	go func() {
		_, _ = s.ExecuteToolCall(ctx, acp.ToolExecutionRequest{
			SessionID: "sess1", ToolCallID: "call1", ToolName: "write", ArgsJSON: `{}`,
		})
	}()
	ev := recvIdeEvent(t, ch)
	if ev.Deadline != want.UnixMilli() {
		t.Fatalf("deadline = %d, want the context deadline %d", ev.Deadline, want.UnixMilli())
	}
}

func TestIdeExecuteToolCallAcceptsAnotherSpellingOfTheSameWorkspace(t *testing.T) {
	ch := ideEvents.SubscribeHandled([]string{"write"})
	defer ideEvents.Unsubscribe(ch)

	dir := t.TempDir()
	s := NewSender(&config.Config{}, nil, false, "m")
	s.SetCWD(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resc := make(chan acp.ToolExecutionResult, 1)
	go func() {
		res, _ := s.ExecuteToolCall(ctx, acp.ToolExecutionRequest{
			SessionID: "sess1", ToolCallID: "call1", ToolName: "write", ArgsJSON: `{}`,
			// Same folder, different spelling: the workspace identity rule compares
			// canonical paths, not raw strings.
			Cwd: dir + string(filepath.Separator),
		})
		resc <- res
	}()
	ev := recvIdeEvent(t, ch)
	if ev.Type != "tool_execute" {
		t.Fatalf("event = %+v", ev)
	}
	if !CompleteToolExecResult("call1", ev.Nonce, acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "ok"}) {
		t.Fatal("the offered call did not resolve")
	}
	select {
	case res := <-resc:
		if res.Status != acp.ToolExecHandled || res.Output != "ok" {
			t.Fatalf("the same workspace under another spelling must still be offered, got %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ExecuteToolCall did not return")
	}
}

func TestIdeToolCallsSharingAnIDResolveByNonce(t *testing.T) {
	ch := ideEvents.SubscribeHandled([]string{"write"})
	defer ideEvents.Unsubscribe(ch)

	s := NewSender(&config.Config{}, nil, false, "m")
	s.SetCWD(t.TempDir())
	// Provider tool-call ids repeat across sessions ("call_0" per answer on
	// several OpenAI-compatible backends), so two sessions delegating under one
	// id must keep their own waits: each result goes to the call whose nonce it
	// echoes, and neither answer is lost to the other's unregister.
	type outcome struct {
		res acp.ToolExecutionResult
		err error
	}
	outc := make(chan outcome, 2)
	call := func(sessionID string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := s.ExecuteToolCall(ctx, acp.ToolExecutionRequest{
			SessionID: sessionID, ToolCallID: "call1", ToolName: "write", ArgsJSON: `{}`,
		})
		outc <- outcome{res, err}
	}
	go call("sess1")
	ev1 := recvIdeEvent(t, ch)
	go call("sess2")
	ev2 := recvIdeEvent(t, ch)
	if ev1.Nonce == ev2.Nonce {
		t.Fatal("the two offers must carry different nonces")
	}
	if !CompleteToolExecResult("call1", ev2.Nonce, acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "two"}) {
		t.Fatal("the second offer did not resolve")
	}
	if !CompleteToolExecResult("call1", ev1.Nonce, acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: "one"}) {
		t.Fatal("the first offer did not resolve")
	}
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case o := <-outc:
			if o.err != nil {
				t.Fatalf("ExecuteToolCall: %v", o.err)
			}
			if o.res.Status != acp.ToolExecHandled {
				t.Fatalf("result = %+v, want handled", o.res)
			}
			seen[o.res.Output] = true
		case <-time.After(2 * time.Second):
			t.Fatal("only one of the two calls returned")
		}
	}
	if !seen["one"] || !seen["two"] {
		t.Fatalf("results = %v, want one and two to their own calls", seen)
	}
}
