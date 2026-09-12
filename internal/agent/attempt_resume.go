package agent

// Cross-restart repeat protection: what the next turn is told about a turn that
// died in a restart loop.
//
// attempt_repeat.go catches a model rewriting its opening while the turn is still
// running, and everything it learns is gone the moment the process is. Nudges are
// LLM-facing and never persisted, the quarantine resets each turn, the eviction
// pins live in this process, and the guard's verdict goes to ui_log.json, which
// the session package documents as excluded from the prompt. The transcript that
// provoked the loop, on the other hand, is on disk.
//
// So a restarted app opens the same session, replays the same history, and hands
// the model several of its own attempts at one answer with nothing saying they
// were failures. It obliges by writing that answer again - the "after a restart it
// starts the same dialogue over" the operator reports.
//
// The state is re-derived from the transcript rather than read from a marker,
// deliberately: the process that would have written the marker is often the one
// that was killed mid-loop, and the evidence - the repeated attempts themselves -
// is what actually survives.

import (
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// lastPromptIndex is the index of the newest message the user actually typed.
//
// The compaction summary is stored with the user role so every provider replays
// it as ordinary input (session.NewCompactionSummaryMessage), which means a plain
// scan for "the last user message" can stop on a message nobody wrote. Excluding
// it here matches what session/compaction.go already does when it looks for turn
// boundaries.
func lastPromptIndex(history []llm.Message) int {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleUser && !history[i].CompactionSummary {
			return i
		}
	}
	return -1
}

// priorTurnRestarts reports how many times the turn before the current prompt
// wrote the same answer again, and which of its steps actually ran.
//
// Two things narrow it to the turns worth flagging. Only consecutive attempts
// count - a turn that comes back to a phrasing several steps later is working
// through a list, while a turn stuck on one answer produces the same opening twice
// in a row, because between the two there is nothing but a connection that
// dropped. And only the run at the *end* of the turn counts: a turn that stumbled
// and then went on to answer is not the failure this warns about, so it stays
// quiet even though a repeat happened somewhere inside it.
//
// Attempts that produced nothing fingerprint as the empty string and are skipped,
// the same rule attemptRepeatDetector applies live: a provider that delivered
// nothing is an outage, not a model repeating itself.
func priorTurnRestarts(history []llm.Message) (restarts int, steps []string) {
	current := lastPromptIndex(history)
	if current <= 0 {
		return 0, nil
	}
	start := lastPromptIndex(history[:current])
	if start < 0 {
		start = 0
	}
	segment := history[start:current]

	var attempts []string
	for _, m := range segment {
		if m.Role != llm.RoleAssistant {
			continue
		}
		if fp := attemptFingerprint(m.Reasoning, m.Content, m.ToolCalls); fp != "" {
			attempts = append(attempts, fp)
		}
	}
	for i := len(attempts) - 1; i > 0; i-- {
		if !sameAttempt(attempts[i-1], attempts[i]) {
			break
		}
		restarts++
	}
	if restarts == 0 {
		return 0, nil
	}
	return restarts, executedSteps(segment, alreadyRanStepsMax)
}

// resumedRestartNudge opens a turn whose predecessor died in a restart loop. It
// says what the transcript cannot: that the attempts above are one answer written
// over and over, not progress. LLM-facing only, never persisted - the same
// contract as streamStallNudge and repeatedAttemptNudge.
func resumedRestartNudge(restarts int, done []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Before this message, your previous turn was cut off by the connection and you began the same response again %d more time(s) rather than carrying it on. ", restarts)
	b.WriteString("Those attempts are in the history above: that answer is already written, so do not start it over. ")
	if len(done) > 0 {
		b.WriteString("These steps already ran in that turn and their results are above - do not run them again: ")
		b.WriteString(strings.Join(done, ", "))
		b.WriteString(". ")
	}
	b.WriteString("Work out what is genuinely still missing, then take the single next concrete step - one tool call, or the reply itself - and keep it short enough to survive the connection.")
	return b.String()
}
