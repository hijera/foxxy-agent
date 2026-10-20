package agent

import (
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// fork(stop-notice-transcript): why a turn stopped before its answer.
//
// Upstream 1.2.9 (#351, issue #255) records the notice in the session's UI log
// and hands it to the caller as SessionPromptResult.StopNotice, and every
// surface prints it apart from the answer. Here the notice is streamed into
// the answer and stored in the transcript - what the max_tokens notice already
// did (max_tokens_notice.go) - so it survives a reload the way the answer does
// and every surface that shows the stream shows it once. StopNotice carries the
// same text for the callers that read the result rather than the stream: the
// HTTP API's meta.stop_notice, the remote client, the subagent report.

// noteStopNotice says why the turn stopped before its answer, on the stream,
// in the transcript and to the session manager.
func (a *Agent) noteStopNotice(text string) {
	a.persistTruncationNotice(text)
	a.state.SetTurnStopNotice(text)
}

// maxTurnsNotice names the step limit a turn ran into and the key that set it.
func (a *Agent) maxTurnsNotice(maxTurns int) string {
	if a.subagent != nil {
		// A child's transcript takes no prompt: the way on is a higher limit
		// or a new run, not a message.
		key := "agent.max_turns"
		switch {
		case a.subagent.MaxTurns > 0:
			key = "the max_turns of the subagent definition"
		case a.cfg.Subagents.MaxTurns > 0:
			key = "subagents.max_turns"
		}
		return fmt.Sprintf("The subagent stopped after %d steps, the step limit set by %s. Its report may be incomplete: raise the limit or run it again.", maxTurns, key)
	}
	return fmt.Sprintf("Stopped after %d steps, the step limit set by agent.max_turns. The task may be unfinished: send a message to let the agent continue, or raise the limit.", maxTurns)
}

// subagentReport is what a child hands its parent: its last answer, and when
// it stopped short, why. The stop notice is an assistant message of its own in
// the child's transcript, so the last assistant text alone would be the notice
// instead of the answer.
func subagentReport(msgs []llm.Message, res *acp.SessionPromptResult) string {
	notice := ""
	if res != nil {
		notice = strings.TrimSpace(res.StopNotice)
	}
	if notice == "" {
		return lastAssistantPlainText(msgs)
	}
	for len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		if last.Role != llm.RoleAssistant || strings.TrimSpace(last.Content) != notice {
			break
		}
		msgs = msgs[:len(msgs)-1]
	}
	body := strings.TrimSpace(lastAssistantPlainText(msgs))
	if body == "" {
		return notice
	}
	return body + "\n\n" + notice
}
