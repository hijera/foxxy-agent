package agent

import (
	"context"
	"errors"
	"time"
)

// The causes the turn's own guards cancel a model call with. They never reach the
// user: the stream still ends in context.Canceled, and the branches after the call
// read the guards' flags. They exist for the log, where the network trace of the
// request (debug.enable) prints them, so a request cut by a timer is not mistaken
// for one the connection dropped.
var (
	errFirstTokenCut = errors.New("agent: no first token within agent.llm_first_token_timeout_ms")
	errStallCut      = errors.New("agent: no progress within agent.llm_stall_timeout_ms")
	errLoopGuardCut  = errors.New("agent: loop guard cut a repeating response")
)

// logLLMCallFinished records how one model call ended: how long it took, how long
// until the first sign of life, and what ended it. Together with the network trace
// it is what a debug log of a turn that hung for an hour is read from.
func (a *Agent) logLLMCallFinished(sessionID string, turn, call int, start time.Time, firstProgressNS int64, output bool, cause, err error) {
	kv := []any{
		"session", sessionID, "turn", turn, "call", call,
		"took", time.Since(start).Round(time.Millisecond),
		"output", output,
	}
	if firstProgressNS != 0 {
		kv = append(kv, "first_progress_after", time.Unix(0, firstProgressNS).Sub(start).Round(time.Millisecond))
	}
	if cutBy := llmCallCutBy(cause, a.state.IsUserCancelledTurn()); cutBy != "" {
		kv = append(kv, "cut_by", cutBy)
	}
	if err != nil {
		kv = append(kv, "error", err.Error())
	}
	a.log.Debug("llm call finished", kv...)
}

// llmCallCutBy names who cancelled a call, or "" when nothing did before it ended.
// The stream context is always cancelled once the call returns, with no cause of
// its own, so only a guard's cause or the user's Stop counts.
func llmCallCutBy(cause error, userCancelled bool) string {
	switch {
	case errors.Is(cause, errFirstTokenCut):
		return "first_token_timeout"
	case errors.Is(cause, errStallCut):
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
