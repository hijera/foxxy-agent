//go:build http

package httpserver

import (
	"sync"
	"sync/atomic"
)

// autocompleteState is the per-process memory behind inline completion: the counters that say
// whether the feature is worth keeping, and which models turned out not to serve raw FIM.
//
// The counters exist because "is the suggestion good enough" cannot be judged by feel. The
// server knows latency, token cost and how often the model returned nothing; the editors know
// how often a suggestion was shown, accepted, dismissed, or served from their prefix cache. The
// two halves meet in GET /foxxycode/completion/stats.
type autocompleteState struct {
	requests    atomic.Int64 // completion requests that reached the model path (enabled, non-empty context)
	served      atomic.Int64 // requests answered with a non-empty suggestion
	empty       atomic.Int64 // requests answered with an empty suggestion (model had nothing, or filters dropped it)
	errors      atomic.Int64 // requests that failed on the model side
	cancelled   atomic.Int64 // requests the client abandoned (typed on) before an answer
	timeouts    atomic.Int64 // requests the model did not answer within autocomplete.timeout_ms
	rateLimited atomic.Int64 // requests the provider refused with 429, passed on to the client
	fim         atomic.Int64 // answered through native fill-in-the-middle
	chat        atomic.Int64 // answered through a chat prompt
	fimFallback atomic.Int64 // raw FIM calls that failed and were re-issued as chat
	fimEmpty    atomic.Int64 // raw FIM calls that returned nothing and were re-issued as chat
	// reasoningRetries counts chat calls re-issued without stop sequences because the model
	// reasons before answering and the stops cut the reasoning short (see markThinking).
	reasoningRetries atomic.Int64
	latencyMS        atomic.Int64 // total model latency, milliseconds
	latencyMax       atomic.Int64 // slowest single model call, milliseconds
	promptTok        atomic.Int64
	outputTok        atomic.Int64

	// Editor-reported outcomes (POST /foxxycode/completion/feedback).
	shown     atomic.Int64
	accepted  atomic.Int64
	dismissed atomic.Int64
	cacheHits atomic.Int64

	// fimBroken remembers model ids whose raw completion call failed in auto mode, so every later
	// keystroke goes straight to chat instead of paying for the failure again.
	fimBroken sync.Map
	// fimEmptyStreak counts consecutive empty raw answers per model. A gateway can serve
	// /v1/completions and still hand the FIM tokens to a model that was never trained on them,
	// which shows as 200 with nothing in it; a run of those is as good as an error.
	fimEmptyStreak sync.Map
	// thinking remembers models that returned reasoning text in chat mode. Servers match stop
	// sequences against the whole generation, reasoning included, so a "\n" stop fires on the
	// first line of the model's thoughts and the answer never comes; such models get no stop
	// sequences and rely on the streamed cutoff instead.
	thinking sync.Map
}

func (st *autocompleteState) markThinking(model string) { st.thinking.Store(model, true) }

func (st *autocompleteState) isThinking(model string) bool {
	_, ok := st.thinking.Load(model)
	return ok
}

// fimEmptyStreakLimit is how many empty raw answers in a row retire FIM for a model.
const fimEmptyStreakLimit = 3

func (st *autocompleteState) markFIMBroken(model string) { st.fimBroken.Store(model, true) }

// noteFIMEmpty records an empty raw answer and reports whether the model has now produced enough
// of them in a row to be treated as FIM-incapable.
func (st *autocompleteState) noteFIMEmpty(model string) bool {
	v, _ := st.fimEmptyStreak.LoadOrStore(model, new(atomic.Int64))
	n := v.(*atomic.Int64).Add(1)
	if n >= fimEmptyStreakLimit {
		st.markFIMBroken(model)
		return true
	}
	return false
}

func (st *autocompleteState) noteFIMServed(model string) {
	if v, ok := st.fimEmptyStreak.Load(model); ok {
		v.(*atomic.Int64).Store(0)
	}
}

func (st *autocompleteState) isFIMBroken(model string) bool {
	_, broken := st.fimBroken.Load(model)
	return broken
}

func (st *autocompleteState) recordLatency(ms int64) {
	st.latencyMS.Add(ms)
	for {
		cur := st.latencyMax.Load()
		if ms <= cur || st.latencyMax.CompareAndSwap(cur, ms) {
			return
		}
	}
}

// feedback applies one editor-reported outcome; unknown events are ignored.
func (st *autocompleteState) feedback(event string) bool {
	switch event {
	case "shown":
		st.shown.Add(1)
	case "accepted":
		st.accepted.Add(1)
	case "dismissed":
		st.dismissed.Add(1)
	case "cache_hit":
		st.cacheHits.Add(1)
	default:
		return false
	}
	return true
}

func (st *autocompleteState) snapshot() map[string]interface{} {
	requests := st.requests.Load()
	served := st.served.Load()
	shown := st.shown.Load()
	accepted := st.accepted.Load()
	avg := int64(0)
	if requests > 0 {
		avg = st.latencyMS.Load() / requests
	}
	acceptance := 0.0
	if shown > 0 {
		acceptance = float64(accepted) / float64(shown)
	}
	return map[string]interface{}{
		"requests":          requests,
		"served":            served,
		"empty":             st.empty.Load(),
		"errors":            st.errors.Load(),
		"cancelled":         st.cancelled.Load(),
		"timeouts":          st.timeouts.Load(),
		"rate_limited":      st.rateLimited.Load(),
		"fim":               st.fim.Load(),
		"chat":              st.chat.Load(),
		"fim_fallback":      st.fimFallback.Load(),
		"fim_empty":         st.fimEmpty.Load(),
		"reasoning_retries": st.reasoningRetries.Load(),
		"latency_avg_ms":    avg,
		"latency_max_ms":    st.latencyMax.Load(),
		"prompt_tokens":     st.promptTok.Load(),
		"output_tokens":     st.outputTok.Load(),
		"shown":             shown,
		"accepted":          accepted,
		"dismissed":         st.dismissed.Load(),
		"cache_hits":        st.cacheHits.Load(),
		"acceptance_rate":   acceptance,
	}
}
