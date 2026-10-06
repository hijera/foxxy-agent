package llm

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// fork(stall-guard-layer): the stall guard watches chunks, not bytes.
//
// Upstream (1.1.47) cuts a stalled stream in the transport: a response body
// that reads no bytes for agent.llm_stream_idle_timeout_ms. Here the guard
// wraps the provider and is re-armed by the chunks the stream readers hand
// on - text, reasoning, tool calls and the Progress frames that stand for a
// tool call's argument fragment, a signature or a usage-only frame. A byte
// counter cannot tell those from a keep-alive comment, an empty `data:` line
// or an Anthropic ping, all of which a gateway keeps sending while the model
// behind it is dead; counting them would hold a dead turn open forever
// (TestProgressNotFiredOnKeepaliveFrames, TestStreamIdleGuardIgnoresKeepaliveComments).
//
// The guard sits outside the retry wrapper, so its clock spans a replayed
// attempt: a call that delivered progress, died and was replayed silently is
// still cut. A stall before any chunk is left to the first-token guard of the
// caller.

// WithStreamIdleGuard returns p guarded against a stream that stops
// delivering chunks for idle after its first one. A non-positive idle returns
// p untouched. Complete is never guarded: a blocking answer arrives in one
// piece and has no gaps to watch.
func WithStreamIdleGuard(p Provider, idle time.Duration) Provider {
	if p == nil || idle <= 0 {
		return p
	}
	return &streamIdleGuard{inner: p, idle: idle}
}

type streamIdleGuard struct {
	inner Provider
	idle  time.Duration
}

// Unwrap exposes the wrapped provider so optional interfaces (RawCompleter)
// can still be found behind the guard.
func (g *streamIdleGuard) Unwrap() Provider { return g.inner }

func (g *streamIdleGuard) Complete(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error) {
	return g.inner.Complete(ctx, messages, tools)
}

func (g *streamIdleGuard) Stream(ctx context.Context, messages []Message, tools []ToolDefinition, onChunk func(StreamChunk)) (*Response, error) {
	gctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// The timer reschedules itself against an atomic stamp rather than being
	// Reset per chunk: one atomic store per chunk instead of the runtime's
	// timer lock, and no Reset-after-fire race.
	var (
		last    atomic.Int64
		armed   atomic.Bool
		stalled atomic.Bool
		timer   atomic.Pointer[time.Timer]
	)
	var arm func(time.Duration)
	arm = func(d time.Duration) {
		t := time.AfterFunc(d, func() {
			if since := time.Since(time.Unix(0, last.Load())); since < g.idle {
				arm(g.idle - since)
				return
			}
			stalled.Store(true)
			// The cause names the cut in the network trace (debug.enable).
			cancel(&streamStalledError{idle: g.idle})
		})
		if old := timer.Swap(t); old != nil {
			old.Stop()
		}
	}
	defer func() {
		if t := timer.Load(); t != nil {
			t.Stop()
		}
	}()

	// What reached the caller, kept for a stream whose reader returns no
	// response once it was cut.
	var (
		mu        sync.Mutex
		text      strings.Builder
		reasoning strings.Builder
	)
	resp, err := g.inner.Stream(gctx, messages, tools, func(c StreamChunk) {
		last.Store(time.Now().UnixNano())
		if armed.CompareAndSwap(false, true) {
			arm(g.idle)
		}
		if c.TextDelta != "" || c.ReasoningDelta != "" {
			mu.Lock()
			text.WriteString(c.TextDelta)
			reasoning.WriteString(c.ReasoningDelta)
			mu.Unlock()
		}
		if onChunk != nil {
			onChunk(c)
		}
	})
	// A cancel from above - the user's Stop, the first-token timer, the loop
	// guard - reaches the caller's context and wins: only the guard's own cut
	// is reported as a stall.
	if err == nil || !stalled.Load() || ctx.Err() != nil {
		return resp, err
	}
	stall := &streamStalledError{idle: g.idle}
	if resp != nil {
		// Tool calls of an answer that never finished may be cut mid-JSON;
		// replaying an invalid call is worse than losing it.
		kept := *resp
		kept.ToolCalls = nil
		return &kept, stall
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.TrimSpace(text.String()) == "" && strings.TrimSpace(reasoning.String()) == "" {
		return nil, stall
	}
	return &Response{Content: text.String(), Reasoning: reasoning.String()}, stall
}
