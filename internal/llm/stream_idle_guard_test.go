package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedStream is a Provider whose Stream runs a script against onChunk and
// the call's context.
type scriptedStream func(ctx context.Context, onChunk func(StreamChunk)) (*Response, error)

func (s scriptedStream) Complete(context.Context, []Message, []ToolDefinition) (*Response, error) {
	return &Response{Content: "blocking"}, nil
}

func (s scriptedStream) Stream(ctx context.Context, _ []Message, _ []ToolDefinition, onChunk func(StreamChunk)) (*Response, error) {
	return s(ctx, onChunk)
}

func streamGuarded(t *testing.T, p Provider, idle time.Duration) (*Response, time.Duration, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := WithStreamIdleGuard(p, idle).Stream(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	return resp, time.Since(start), err
}

// A stream that delivered text and then went silent is cut after idle, and
// the text the caller already saw comes back with the stall.
func TestStreamIdleGuardCutsAStreamThatWentSilent(t *testing.T) {
	const idle = 60 * time.Millisecond
	p := scriptedStream(func(ctx context.Context, onChunk func(StreamChunk)) (*Response, error) {
		onChunk(StreamChunk{TextDelta: "half an ans"})
		<-ctx.Done()
		return nil, ctx.Err()
	})
	resp, took, err := streamGuarded(t, p, idle)
	if !IsStreamStalled(err) {
		t.Fatalf("err = %v, want a stall", err)
	}
	if StreamStalledIdle(err) != idle {
		t.Fatalf("stall idle = %v, want %v", StreamStalledIdle(err), idle)
	}
	if took < idle {
		t.Fatalf("cut after %v, before the %v idle bound", took, idle)
	}
	if resp == nil || resp.Content != "half an ans" {
		t.Fatalf("partial answer = %+v, want the delivered text", resp)
	}
}

// Every chunk re-arms the guard, a Progress frame included: a model writing
// one large tool call delivers nothing else for as long as that takes.
func TestStreamIdleGuardRearmsOnEveryChunk(t *testing.T) {
	const idle = 80 * time.Millisecond
	p := scriptedStream(func(ctx context.Context, onChunk func(StreamChunk)) (*Response, error) {
		for i := 0; i < 15; i++ {
			onChunk(StreamChunk{Progress: true})
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
		}
		return &Response{Content: "done"}, nil
	})
	resp, _, err := streamGuarded(t, p, idle)
	if err != nil {
		t.Fatalf("a stream that kept delivering progress was cut: %v", err)
	}
	if resp == nil || resp.Content != "done" {
		t.Fatalf("resp = %+v", resp)
	}
}

// Silence before the first chunk is the first-token guard's to judge, not
// this one's: a reasoning model can take minutes of prefill.
func TestStreamIdleGuardIsNotArmedBeforeTheFirstChunk(t *testing.T) {
	p := scriptedStream(func(ctx context.Context, onChunk func(StreamChunk)) (*Response, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		onChunk(StreamChunk{TextDelta: "late"})
		return &Response{Content: "late"}, nil
	})
	if _, _, err := streamGuarded(t, p, 40*time.Millisecond); err != nil {
		t.Fatalf("the guard cut a stream that had not started: %v", err)
	}
}

// A cancel that came from above - the user's Stop, the loop guard - is not a
// stall and must not be reported as one.
func TestStreamIdleGuardLeavesACallerCancelAlone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := scriptedStream(func(ctx context.Context, onChunk func(StreamChunk)) (*Response, error) {
		onChunk(StreamChunk{TextDelta: "x"})
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	})
	_, err := WithStreamIdleGuard(p, time.Hour).Stream(ctx, nil, nil, func(StreamChunk) {})
	if IsStreamStalled(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the caller's cancel", err)
	}
}

// fork(stall-guard-layer) guard: a gateway that keeps sending SSE comments
// while the model behind it is dead must still be cut. Upstream's byte-level
// guard counts those comments as life and never fires.
func TestStreamIdleGuardIgnoresKeepaliveComments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"content\":\"hi\"}}],\"id\":\"c1\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n")
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
				_, _ = io.WriteString(w, ": keep-alive\n\ndata:\n\n")
				fl.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)

	const idle = 250 * time.Millisecond
	p, err := NewProvider(ProviderInput{Type: "openai", Model: "test-model", BaseURL: srv.URL, StreamIdleTimeout: idle})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := p.Stream(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if !IsStreamStalled(err) {
		t.Fatalf("err = %v, want a stall despite the keep-alive comments", err)
	}
	if resp == nil || resp.Content != "hi" {
		t.Fatalf("partial answer = %+v, want the delivered text", resp)
	}
}

// fork(stall-guard-layer) guard: the guard sits outside the retry wrapper, so
// a call that delivered progress, died and was replayed silently is cut on the
// same clock rather than waiting for the turn to end.
func TestStreamIdleGuardSpansRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		if n == 1 {
			// One argument fragment - progress, not output - then the
			// connection dies: a transport failure the wrapper replays.
			_, _ = io.WriteString(w, toolArgFrame(`{"q":`))
			fl.Flush()
			panic(http.ErrAbortHandler)
		}
		fl.Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	const idle = 300 * time.Millisecond
	in := WithAgentResilience(ProviderInput{Type: "openai", Model: "test-model", BaseURL: srv.URL, StreamIdleTimeout: idle}, 2, 10, 0)
	p, err := NewProvider(in)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	_, err = p.Stream(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if !IsStreamStalled(err) {
		t.Fatalf("err = %v after %v, want the replayed attempt cut as a stall", err, time.Since(start))
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("requests = %d, want the first attempt and one replay", got)
	}
}
