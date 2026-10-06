package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResilientStreamDoesNotRetryAfterOutput(t *testing.T) {
	for _, chunk := range []StreamChunk{{TextDelta: "Starting"}, {ReasoningDelta: "Thinking"}, {ToolCall: &ToolCall{ID: "call_1", Name: "read"}}} {
		t.Run(chunk.TextDelta+chunk.ReasoningDelta, func(t *testing.T) {
			calls, delivered := 0, 0
			failure := errors.New("503 Service Unavailable")
			p := wrapResilient(&stubProvider{streamFn: func(_ context.Context, _ []Message, _ []ToolDefinition, send func(StreamChunk)) (*Response, error) {
				calls++
				send(chunk)
				return &Response{Content: "partial"}, failure
			}}, ResilientOptions{RetryBase: time.Millisecond, RetryMaxDelay: time.Millisecond})
			resp, err := p.Stream(context.Background(), nil, nil, func(StreamChunk) { delivered++ })
			if calls != 1 || delivered != 1 || !errors.Is(err, failure) || resp == nil || resp.Content != "partial" {
				t.Fatalf("calls=%d delivered=%d response=%+v error=%v", calls, delivered, resp, err)
			}
		})
	}
}

func TestResilientStreamRetainsChunksWhenProviderReturnsNoPartialResponse(t *testing.T) {
	calls := 0
	failure := errors.New("503 Service Unavailable")
	p := wrapResilient(&stubProvider{streamFn: func(_ context.Context, _ []Message, _ []ToolDefinition, send func(StreamChunk)) (*Response, error) {
		calls++
		send(StreamChunk{TextDelta: "first "})
		send(StreamChunk{TextDelta: "step", ReasoningDelta: "reason"})
		return nil, failure
	}}, ResilientOptions{RetryBase: time.Millisecond})
	resp, err := p.Stream(context.Background(), nil, nil, nil)
	if calls != 1 || !errors.Is(err, failure) || resp == nil || resp.Content != "first step" || resp.Reasoning != "reason" {
		t.Fatalf("calls=%d response=%+v error=%v", calls, resp, err)
	}
}
