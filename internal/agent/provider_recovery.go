package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// Provider recovery, from upstream 1.2.9 (#351, issue #246): a failure of the
// provider's lane after the answer began - a 5xx the retry wrapper could not
// ride out, a stream that dropped mid-answer - does not end the turn. The text
// the user watched arrive is kept and the model is asked to go on from it.
//
// fork(continue-path): here it is one more way a cut answer is carried on,
// next to the stall continuation, not a mechanism of its own. It follows
// agent.llm_continue, spends the same per-turn budget (agent.llm_continue_max)
// outside max_turns, runs through the same repeat detector, and pauses by
// agent.llm_continue_error_delays_ms instead of multiples of llm_retry_base_ms.
// llm_retry_max: 0 does not turn it off, as it does upstream. A failure before
// any text is the llm_stall_retry ladder's, which the fork kept.

// providerRecoveryNudge asks the model to finish an answer a provider failure
// cut off. LLM-facing only, after the partial answer the transcript keeps.
const providerRecoveryNudge = "Your previous response was cut off by a provider error before it finished. Continue exactly where it stopped, without repeating what you already wrote."

// errorContinueDelay is the pause before the continuation after a provider
// failure, n continuations into the turn (0-based): the configured list, or the
// longer pause the provider asked for, up to llm_continue_retry_after_max_ms.
func errorContinueDelay(cfg *config.Agent, n int, err error) time.Duration {
	delay := continueDelay(cfg.EffectiveLLMContinueErrorDelays(), n)
	limit := cfg.EffectiveLLMContinueRetryAfterMax()
	if limit <= 0 {
		return delay
	}
	if asked, ok := llm.UpstreamRetryAfter(err); ok && asked > delay {
		delay = min(asked, max(limit, delay))
	}
	return delay
}

// providerGaveUpError ends a turn whose provider kept failing mid-answer after
// the continuation budget ran out.
func providerGaveUpError(err error, continued int) error {
	return fmt.Errorf("LLM error: %w (the provider failed mid-answer again after %d continuation attempts)", err, continued)
}

// keepInterruptedAnswer adds to the transcript the part of an answer a
// provider failure cut off: the text the user watched stream in and its
// reasoning, without tool calls, which never finished. It reports whether any
// answer text was kept, which is what the model is then asked to continue.
//
// The text is cut back to its last line end (trimToResumeBoundary), the rule
// the stall path applies: a model carrying on a cut line writes that line
// again from its start, so an unfinished one would reach the answer twice.
func (a *Agent) keepInterruptedAnswer(text, reasoning string, clockStart, clockEnd time.Time) bool {
	text = trimToResumeBoundary(text)
	reasonTrim := strings.TrimSpace(reasoning)
	if strings.TrimSpace(text) == "" && reasonTrim == "" {
		return false
	}
	var reasoningMs int64
	if reasonTrim != "" && !clockStart.IsZero() {
		end := clockEnd
		if end.IsZero() {
			end = time.Now()
		}
		reasoningMs = max(end.Sub(clockStart), 0).Milliseconds()
	}
	a.state.AddMessage(llm.Message{
		Role:                llm.RoleAssistant,
		Content:             text,
		Reasoning:           reasonTrim,
		ReasoningDurationMs: reasoningMs,
		Model:               a.state.EffectiveModelID(a.cfg),
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
	})
	a.refreshConversationContextUsage(true)
	return strings.TrimSpace(text) != ""
}
