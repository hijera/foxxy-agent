package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// stallRetry paces re-issuing an LLM call that produced no output at all.
//
// A saturated gateway answers nothing for minutes: the first-token watchdog cuts
// the stream, and without this the turn dies and the user restarts it by hand.
// internal/llm's own retry layer cannot help here - it classifies the watchdog's
// cancellation as a user Stop (it cannot tell the two apart) and its ladder is
// seconds, not minutes.
//
// The two layers are complementary, not redundant: llm_retry_max absorbs a
// hiccup in seconds, this absorbs an outage in minutes.
type stallRetry struct {
	enabled bool
	// delays is the pause before each attempt; the last entry repeats for every
	// attempt past the end of the slice, which is what makes "then every five
	// minutes" a schedule rather than an extra knob.
	delays  []time.Duration
	maxWait time.Duration // 0 = unbounded
	waited  time.Duration
	retries int
}

func newStallRetry(a *config.Agent) stallRetry {
	return stallRetry{
		enabled: a.LLMStallRetryEnabled(),
		delays:  a.EffectiveLLMStallRetryDelays(),
		maxWait: a.EffectiveLLMStallRetryMaxWait(),
	}
}

// next reports the pause before the next attempt, and whether the budget allows
// one at all. The budget is checked against the pause about to be taken, so the
// loop gives up now rather than sleeping into a deadline it has already passed.
func (s *stallRetry) next() (time.Duration, bool) {
	if !s.enabled || len(s.delays) == 0 {
		return 0, false
	}
	i := s.retries
	if i >= len(s.delays) {
		i = len(s.delays) - 1
	}
	d := s.delays[i]
	if s.maxWait > 0 && s.waited+d > s.maxWait {
		return 0, false
	}
	s.retries++
	s.waited += d
	return d, true
}

// wait pauses for d, returning an error the moment the turn is cancelled. The
// pause is minutes long, so a user Stop must end the turn rather than sit it out:
// that is why this selects on the context instead of sleeping.
func (s *stallRetry) wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// attempts is how many retries have been handed out, for logging and for the
// give-up notice.
func (s *stallRetry) attempts() int { return s.retries }

// spent is the total pause taken so far, for the give-up notice.
func (s *stallRetry) spent() time.Duration { return s.waited }

// stallRetryableError reports whether a failed LLM call is worth waiting out.
//
// It is only ever consulted once the caller has established that the turn context
// is alive, the user did not stop the turn, and nothing reached the transcript -
// so replaying is always *safe* by that point. This decides whether it is
// *useful*, and the default leans towards trying again: a saturated gateway
// fails in many shapes, and sitting out a pause costs far less than handing the
// user an error they will retype "continue" at.
func stallRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if llm.IsRetryableProviderError(err) {
		return true
	}
	// internal/llm refuses to retry a deadline because from inside a provider a
	// deadline is indistinguishable from the caller giving up. Here it is not: the
	// turn context was checked and is alive, so this deadline belongs to the
	// provider - providers[].timeout_ms, or the transport's response-header
	// timeout - and is exactly the "Client.Timeout ... while reading body" failure
	// a struggling hub produces.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// A refused request stays refused. A bad key, an unknown model or a malformed
	// payload will fail identically in five minutes, so those end the turn at once
	// instead of burning the retry budget. 408 and 429 are timing, not refusal,
	// and are already retryable above.
	if code := llm.HTTPStatus(err); code >= 400 && code < 500 {
		return false
	}
	return true
}

// maxStallContinuations bounds how many times one turn may be asked to carry on
// after the provider stopped sending data mid-answer. Deliberately not shared
// with the loop guard's nudge budget: a saturated hub is an infrastructure
// fault, not model misbehaviour, and letting it spend that budget would disarm
// the runaway-loop protection. Measured stall rates on the reported hub reach
// roughly half of long generations, so one answer can plausibly stall twice.
const maxStallContinuations = 3

// streamStallNudge asks the model to resume a response the transport cut short.
// LLM-facing only; never persisted to the transcript, the same contract as
// streamLoopNudge and emptyAssistantContinuationNudge.
const streamStallNudge = "Your previous response was cut off part-way through: the connection stopped delivering data before you finished. Everything you had produced is in the message above. Continue from exactly where it stops - do not repeat, restate or summarise anything you already wrote, and do not start over."

// stallAbortError is the notice surfaced when the provider keeps cutting the
// answer short and the continuation budget runs out.
func stallAbortError(idle time.Duration, continued int) error {
	return fmt.Errorf("stopped: the provider stopped sending data mid-answer (no progress for %v) and did not finish after %d continuation attempts", idle, continued)
}

// stallGaveUpSuffix explains a wait the user sat through, so the error that ends
// a turn says why it took so long. Empty when nothing was ever retried, which
// keeps the message identical to the pre-retry wording.
func stallGaveUpSuffix(s *stallRetry) string {
	if s.attempts() == 0 {
		return ""
	}
	return fmt.Sprintf("; gave up after %d retries over %v", s.attempts(), s.spent())
}

// waitForStalledProvider pauses before re-issuing a call the provider answered
// with nothing at all. Only such calls reach here: nothing was delivered, so the
// replay is exact and cannot duplicate anything in the transcript.
//
// It reports whether the caller should retry. A false retry with cancelled set
// means the turn was stopped during the pause and the caller must return a clean
// cancellation rather than an error.
func (a *Agent) waitForStalledProvider(ctx context.Context, s *stallRetry, sessionID, reason string) (retry bool, cancelled bool) {
	d, ok := s.next()
	if !ok {
		return false, false
	}
	a.log.Warn("provider produced no output; waiting before retry",
		"reason", reason, "attempt", s.attempts(), "delay", d, "waited", s.spent())
	_ = a.server.SendSessionUpdate(sessionID, acp.LLMRetryUpdate{
		SessionUpdate: acp.UpdateTypeLLMRetry,
		Phase:         acp.LLMRetryPhaseWaiting,
		Attempt:       s.attempts(),
		DelayMS:       d.Milliseconds(),
	})
	if err := s.wait(ctx, d); err != nil {
		return false, true
	}
	if a.state.IsUserCancelledTurn() {
		return false, true
	}
	_ = a.server.SendSessionUpdate(sessionID, acp.LLMRetryUpdate{
		SessionUpdate: acp.UpdateTypeLLMRetry,
		Phase:         acp.LLMRetryPhaseRetrying,
		Attempt:       s.attempts(),
	})
	return true, false
}

// trimToResumeBoundary cuts a stalled partial answer back to a point the model can
// resume from cleanly.
//
// A stream can stop anywhere - mid-word, mid-list-item - and a model asked to
// carry on restarts the line it was interrupted in rather than continuing the
// fragment. Measured live against qwen3.6 on api.neuraldeep.ru: a partial ending
// "1. Use the" came back as "1. Use the1. Use the active voice ...", duplicating
// the fragment at the seam no matter how firmly the nudge says not to repeat.
//
// Dropping the unfinished trailing line makes both model behaviours safe: whether
// it repeats that line or starts the next one, the text joins up exactly once.
// Prose with no line breaks falls back to the last sentence end. A partial with
// neither is left alone - keeping text the user watched arrive beats discarding
// all of it to avoid a seam.
func trimToResumeBoundary(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[:i+1]
	}
	if i := strings.LastIndexAny(s, ".!?"); i >= 0 && i+1 < len(s) {
		return s[:i+1]
	}
	return s
}

// persistStalledMessage stores the partial assistant message from a stream the
// stall guard cut, and reports whether anything was worth keeping.
//
// Tool calls are deliberately dropped. On a cancelled stream the readers hand
// back every builder they had, including one whose JSON arguments were cut
// mid-write; and every tool_call_id an assistant message announces must get a
// result, or OpenAI-compatible endpoints reject the next request in the
// conversation. Replaying an invalid call is worse than losing it - the same
// choice openai_stream.go already makes for a truncated stream.
func (a *Agent) persistStalledMessage(
	response *llm.Response,
	reasoningBuf *strings.Builder,
	reasonClockStart, reasonClockEnd time.Time,
) bool {
	content := ""
	if response != nil {
		content = trimToResumeBoundary(response.Content)
	}
	reasonRaw := reasoningBuf.String()
	reasonTrim := strings.TrimSpace(reasonRaw)
	reasonStore, reasonSig := reasoningForStorage(reasonTrim, reasonRaw, response)

	if strings.TrimSpace(content) == "" && strings.TrimSpace(reasonStore) == "" {
		return false
	}

	var reasoningMs int64
	if reasonTrim != "" && !reasonClockStart.IsZero() {
		end := reasonClockEnd
		if end.IsZero() {
			end = time.Now()
		}
		if d := end.Sub(reasonClockStart); d > 0 {
			reasoningMs = d.Milliseconds()
		}
	}

	a.state.AddMessage(llm.Message{
		Role:                llm.RoleAssistant,
		Content:             content,
		Reasoning:           reasonStore,
		ReasoningSignature:  reasonSig,
		ReasoningDurationMs: reasoningMs,
		Model:               a.state.EffectiveModelID(a.cfg),
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
	})
	a.refreshConversationContextUsage(true)
	return true
}
