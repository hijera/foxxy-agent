package llm

// Tool calls on a stream the transport cut - fork(cut-keeps-complete-calls).
// A call whose arguments arrived as a complete JSON value is the step the
// model finished asking for and survives the cut next to the error, while a
// call cut mid-JSON never runs: replaying an invalid call is worse than
// losing it. The loop level (internal/agent) is what runs the kept calls.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// wantCall is a tool call the partial response must carry: a call whose
// arguments arrived whole is kept, one whose JSON was cut is not.
type wantCall struct{ name, args string }

func checkKeptCalls(t *testing.T, got []ToolCall, want []wantCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("partial tool calls = %+v, want %d of %+v", got, len(want), want)
	}
	for i, w := range want {
		if got[i].Name != w.name || got[i].InputJSON != w.args {
			t.Errorf("call %d = %q(%s), want %q(%s)", i, got[i].Name, got[i].InputJSON, w.name, w.args)
		}
	}
}

// Wire fragments of an OpenAI-compatible stream. A fragment ending in a blank
// line is a whole frame; one that does not is where the body ends.
const (
	cutCallWholeFrame = "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"run_command\",\"arguments\":\"{\\\"command\\\":\\\"ls\\\"}\"}}]}}]}\n\n"
	cutCallArgsFrame  = "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n"
	cutTailFrame      = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo"
)

// TestWholeToolCallsKeepsOnlyValidArguments pins the filter the cut paths
// share: a call runs only with an id, a name and arguments that parse. Empty
// arguments cannot be told from arguments that never started, so the call is
// dropped; a model that means "no arguments" sends {}.
func TestWholeToolCallsKeepsOnlyValidArguments(t *testing.T) {
	in := []ToolCall{
		{ID: "call_1", Name: "run_command", InputJSON: `{"command":"ls"}`},
		{ID: "call_2", Name: "read", InputJSON: `{"path":"a.g`},
		{ID: "call_3", Name: "read", InputJSON: ""},
		{ID: "call_4", Name: "write", InputJSON: `{"path":"a.txt","content":"x"}`},
		{ID: "", Name: "read", InputJSON: `{}`},
		{ID: "call_6", Name: "", InputJSON: `{}`},
		{ID: "call_7", Name: "background_list", InputJSON: `{}`},
		{ID: "call_8", Name: "background_reap", InputJSON: "null"},
	}
	checkKeptCalls(t, WholeToolCalls(in), []wantCall{
		{name: "run_command", args: `{"command":"ls"}`},
		{name: "write", args: `{"path":"a.txt","content":"x"}`},
		{name: "background_list", args: `{}`},
		{name: "background_reap", args: "null"},
	})
	if got := WholeToolCalls(nil); len(got) != 0 {
		t.Errorf("WholeToolCalls(nil) = %+v, want none", got)
	}
}

// A different replay response must not replace the whole call from the cut
// stream, even when no text or reasoning preceded its arguments.
func TestOpenAIStreamCutWithWholeCallIsNotReplayed(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			_, _ = io.WriteString(w, cutCallWholeFrame)
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	p, err := NewProvider(ProviderInput{
		Type: "openai", Model: "test-model", BaseURL: srv.URL,
		RetryMax: 1, RetryBase: time.Millisecond, RetryMaxDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if !IsStreamTruncated(err) || resp == nil {
		t.Fatalf("response = %+v, error = %v, want a partial response and truncation", resp, err)
	}
	checkKeptCalls(t, resp.ToolCalls, []wantCall{{name: "run_command", args: `{"command":"ls"}`}})
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

// cutStub serves body for every request and counts them.
func cutStub(t *testing.T, body string) (Provider, *atomic.Int32, func()) {
	t.Helper()
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, body)
	}))
	return newOpenAIProvider("test-model", "", srv.URL, nil, 0, 0, ""), calls, srv.Close
}

// TestOpenAIStreamCutKeepsWholeToolCalls: the stream ends after one call
// arrived whole and another mid-arguments, with no terminal marker at all -
// the cut a proxy that drops the tail of the body produces. The whole call
// comes back next to the truncation error, the half-written one does not -
// the loop runs only calls that can execute.
func TestOpenAIStreamCutKeepsWholeToolCalls(t *testing.T) {
	p, calls, done := cutStub(t, cutCallWholeFrame+cutCallArgsFrame)
	defer done()

	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if err == nil {
		t.Fatalf("succeeded with %+v, want a truncation error", resp)
	}
	if !IsStreamTruncated(err) || !IsTransientProviderError(err) {
		t.Fatalf("err = %v, want a stream truncation the loop recovers from", err)
	}
	if calls.Load() != 1 {
		t.Errorf("requests = %d, want 1", calls.Load())
	}
	if resp == nil {
		t.Fatalf("no partial response next to %v; a whole tool call must survive the cut", err)
	}
	checkKeptCalls(t, resp.ToolCalls, []wantCall{{name: "run_command", args: `{"command":"ls"}`}})
	if resp.StopReason != "" {
		t.Errorf("partial carries stop reason %q", resp.StopReason)
	}
}

// TestOpenAIStreamCutMidFrameKeepsWholeToolCalls: the same, with the body cut
// inside a data: line instead of between frames. Whatever the error class the
// reader files that under, a whole call that arrived before the cut is not
// its casualty.
func TestOpenAIStreamCutMidFrameKeepsWholeToolCalls(t *testing.T) {
	p, _, done := cutStub(t, cutCallWholeFrame+cutTailFrame)
	defer done()

	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if err == nil {
		t.Fatalf("succeeded with %+v, want an error", resp)
	}
	if resp == nil {
		t.Fatalf("no partial response next to %v; a whole tool call must survive the cut", err)
	}
	checkKeptCalls(t, resp.ToolCalls, []wantCall{{name: "run_command", args: `{"command":"ls"}`}})
}

// TestOpenAIStreamTransportCutKeepsWholeToolCalls is the proxy cut shape: the
// connection dies mid-body after a tool call arrived whole (a response that
// declares more bytes than it sends). The call survives next to the transport
// error - dropping it used to kill the whole step and lose the call for good.
func TestOpenAIStreamTransportCutKeepsWholeToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the response writer cannot hijack; the test cannot cut mid-body")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		// Declare more bytes than are sent, then close abruptly: the client's
		// body read ends in the middle, the transport cut a proxy produces.
		_, _ = fmt.Fprintf(buf, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: %d\r\n\r\n%s",
			len(cutCallWholeFrame)+100, cutCallWholeFrame)
		_ = buf.Flush()
		_ = conn.Close()
	}))
	defer srv.Close()
	p := newOpenAIProvider("test-model", "", srv.URL, nil, 0, 0, "")

	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if err == nil {
		t.Fatalf("succeeded with %+v, want a transport error", resp)
	}
	if !IsTransientProviderError(err) {
		t.Errorf("IsTransientProviderError(%v) = false, want true", err)
	}
	if resp == nil {
		t.Fatalf("no partial response next to %v; a whole tool call must survive the cut", err)
	}
	checkKeptCalls(t, resp.ToolCalls, []wantCall{{name: "run_command", args: `{"command":"ls"}`}})
}

// TestOpenAIStreamCutWithNoWholeCallReturnsNothing: when the only call was cut
// mid-JSON there is nothing to run, and a response that carries neither text
// nor a whole call stays absent - the error goes back alone.
func TestOpenAIStreamCutWithNoWholeCallReturnsNothing(t *testing.T) {
	p, _, done := cutStub(t, cutCallArgsFrame)
	defer done()

	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if err == nil || !IsStreamTruncated(err) {
		t.Fatalf("err = %v, want a stream truncation", err)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want none: only half-written calls arrived", resp)
	}
}

// TestAnthropicStreamCutKeepsWholeToolCalls mirrors the contract on the
// anthropic path: a stream cut before its terminal message_delta keeps the
// tool_use block whose input JSON arrived whole and drops the one cut
// mid-JSON.
func TestAnthropicStreamCutKeepsWholeToolCalls(t *testing.T) {
	body := anthropicStreamPrefix +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"run_command\",\"input\":{}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"command\\\":\\\"ls\\\"}\"}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":2,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_2\",\"name\":\"read\",\"input\":{}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\"}}\n\n"
	p, done := anthropicStreamStub(t, body)
	defer done()

	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if !IsStreamTruncated(err) {
		t.Fatalf("err = %v, want a stream truncation", err)
	}
	if resp == nil {
		t.Fatalf("no partial response next to %v; a whole tool call must survive the cut", err)
	}
	checkKeptCalls(t, resp.ToolCalls, []wantCall{{name: "run_command", args: `{"command":"ls"}`}})
}
