package llm

// Coverage for StreamChunk.Progress: the per-frame "the model is still working"
// signal a mid-stream stall watchdog needs. Tool-call argument deltas are
// accumulated inside the stream readers and never handed to onChunk, so without
// this signal a model writing one large tool call looks identical to a hung
// connection - one such call was measured at 984 consecutive frames with zero
// onChunk calls.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// toolArgFrame is one SSE frame carrying nothing but a fragment of a tool call's
// JSON arguments - the shape that delivers nothing to the caller.
func toolArgFrame(fragment string) string {
	return fmt.Sprintf(
		"data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"search\",\"arguments\":%q}}]}}],\"id\":\"c1\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n",
		fragment)
}

func sseStub(t *testing.T, script string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, script)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func streamChunks(t *testing.T, p Provider) ([]StreamChunk, *Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var got []StreamChunk
	resp, err := p.Stream(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil, func(c StreamChunk) {
		got = append(got, c)
	})
	return got, resp, err
}

// TestProgressFiresOnToolCallArgumentDeltas pins the signal itself: frames that
// only advance a tool call's arguments must reach the caller as Progress, and
// must not masquerade as content.
func TestProgressFiresOnToolCallArgumentDeltas(t *testing.T) {
	const frames = 100
	var script strings.Builder
	for i := 0; i < frames; i++ {
		script.WriteString(toolArgFrame("a"))
	}
	script.WriteString("data: {\"choices\":[{\"finish_reason\":\"tool_calls\",\"index\":0,\"delta\":{}}],\"id\":\"c1\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n")
	script.WriteString("data: [DONE]\n\n")

	srv := sseStub(t, script.String(), nil)
	p, err := NewProvider(ProviderInput{Type: "openai", Model: "test-model", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	got, resp, err := streamChunks(t, p)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var progress, delivered int
	for _, c := range got {
		switch {
		case c.Progress:
			progress++
		case c.TextDelta != "" || c.ReasoningDelta != "" || c.ToolCall != nil:
			delivered++
		}
	}
	// One per argument frame plus one for the finish_reason frame, which also
	// advances generation without delivering anything.
	if progress != frames+1 {
		t.Errorf("progress chunks = %d, want %d", progress, frames+1)
	}
	// Exactly one delivery: the assembled tool call, handed over after the frame
	// loop ends. Every argument fragment before it was invisible to the caller.
	if delivered != 1 {
		t.Errorf("delivered %d chunks, want 1 (the assembled tool call only)", delivered)
	}
	if resp == nil || len(resp.ToolCalls) != 1 {
		t.Fatalf("expected the assembled tool call in the response, got %+v", resp)
	}
}

// TestProgressNotFiredOnKeepaliveFrames is the property that stops a chatty but
// stalled gateway from holding a turn open forever: a proxy emitting comments
// while the upstream model is dead must not look like progress.
func TestProgressNotFiredOnKeepaliveFrames(t *testing.T) {
	script := ": keep-alive\n\n" + ": keep-alive\n\ndata:\n\n" +
		"data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"content\":\"hi\"}}],\"id\":\"c1\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n" +
		": keep-alive\n\n" +
		"data: {\"choices\":[{\"finish_reason\":\"stop\",\"index\":0,\"delta\":{}}],\"id\":\"c1\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n" +
		"data: [DONE]\n\n"

	srv := sseStub(t, script, nil)
	p, err := NewProvider(ProviderInput{Type: "openai", Model: "test-model", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	got, _, err := streamChunks(t, p)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var progress int
	for _, c := range got {
		if c.Progress {
			progress++
		}
	}
	// Only the finish_reason frame advances generation; the comments and the
	// empty data: line are dropped by the scanner before they become frames.
	if progress != 1 {
		t.Errorf("keepalive frames produced %d progress chunks, want 1 (the finish_reason frame)", progress)
	}
}

// TestProgressChunkDoesNotSetEmitted is the guard on the invariant the whole
// retry layer rests on: a chunk no caller could see must not cost the request
// its replay. Routing Progress through openai_stream.go's emit closure, or
// widening resilient.go's guarded callback, would silently disable retries for
// every truncated or transport-failed stream.
func TestProgressChunkDoesNotSetEmitted(t *testing.T) {
	// Tool-argument frames only, then the connection closes with neither a
	// finish_reason nor [DONE]: a truncated stream that delivered nothing.
	script := toolArgFrame("{\"q\":") + toolArgFrame("\"go\"}")

	var hits atomic.Int32
	srv := sseStub(t, script, &hits)
	p, err := NewProvider(ProviderInput{
		Type: "openai", Model: "test-model", BaseURL: srv.URL,
		RetryMax: 1, RetryBase: time.Millisecond, RetryMaxDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	if _, _, err = streamChunks(t, p); err == nil {
		t.Fatal("expected a truncation error from a stream with no finish_reason and no [DONE]")
	}
	if !IsStreamTruncated(err) {
		t.Fatalf("error = %v, want a truncation error", err)
	}
	// RetryMax 1 means two attempts. Both must happen: nothing reached the
	// caller, so the request is still safely replayable.
	if got := hits.Load(); got != 2 {
		t.Errorf("stub received %d requests, want 2 (progress must not disable retry)", got)
	}
}
