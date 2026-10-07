package agent

import (
	"context"
	"errors"
	"time"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// The causes the turn's own guards cancel a model call with. They never reach the
// user: the stream still ends in context.Canceled, and the branches after the call
// read the guards' flags. They exist for the log, where the network trace of the
// request (debug.enable) prints them, so a request cut by a timer is not mistaken
// for one the connection dropped. The stall guard is the provider's
// (llm.WithStreamIdleGuard) and cancels with an llm stall error of its own.
var (
	errFirstTokenCut = errors.New("agent: no first token within agent.llm_first_token_timeout_ms")
	errLoopGuardCut  = errors.New("agent: loop guard cut a repeating response")
)

// logLLMCallFinished records how one model call ended: how long it took, how long
// until the first sign of life, and what ended it. Together with the network trace
// it is what a debug log of a turn that hung for an hour is read from.
func (a *Agent) logLLMCallFinished(sessionID string, turn, call int, start time.Time, firstProgressNS int64, output bool, cause, err error, response *llm.Response, retryDetails ...any) {
	kv := []any{
		"session", sessionID, "turn", turn, "call", call,
		"took", time.Since(start).Round(time.Millisecond),
		"output", output,
	}
	kv = append(kv, retryDetails...)
	if firstProgressNS != 0 {
		kv = append(kv, "first_progress_after", time.Unix(0, firstProgressNS).Sub(start).Round(time.Millisecond))
	}
	// Upstream's account of the answer (1.1.47 logLLMCall): how it stopped and
	// how much of it there was.
	if response != nil {
		kv = append(kv, "stop_reason", response.StopReason,
			"content_bytes", len(response.Content), "tool_calls", len(response.ToolCalls))
	}
	if cutBy := llmCallCutBy(cause, err, a.state.IsUserCancelledTurn()); cutBy != "" {
		kv = append(kv, "cut_by", cutBy)
	}
	if err != nil {
		kv = append(kv, "error", err.Error())
	}
	a.log.Debug("llm call finished", kv...)
}

// llmCallCutBy names who cancelled a call, or "" when nothing did before it ended.
// The stream context is always cancelled once the call returns, with no cause of
// its own, so only a guard's cause, the stall the provider's guard reported, or
// the user's Stop counts.
func llmCallCutBy(cause, err error, userCancelled bool) string {
	switch {
	case errors.Is(cause, errFirstTokenCut):
		return "first_token_timeout"
	case llm.IsStreamStalled(err):
		return "stall_timeout"
	case errors.Is(cause, errLoopGuardCut):
		return "loop_guard"
	case userCancelled:
		return "user"
	case cause != nil && !errors.Is(cause, context.Canceled):
		return cause.Error()
	}
	return ""
}
