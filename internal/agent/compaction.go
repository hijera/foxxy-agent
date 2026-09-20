package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

//go:embed prompts/compaction.md
var compactionSystemPrompt string

// summaryPrefix precedes the generated summary in the synthetic message sent to the model. It is
// deliberately short and in English (the summary body follows in the conversation's language).
const summaryPrefix = "Summary of the earlier conversation (older turns were compacted to save context):\n\n"

// resolveContextWindow returns the model's context window and per-completion output cap.
//
// The window is the one the whole product measures against (contextWindow in
// context_usage.go): max_context_tokens, else what the provider's model listing
// reports, else config.DefaultContextWindowTokens. This engine used to keep a
// ladder of its own, which is how the ring in the web UI and the trigger here
// could disagree about how full the same session was (upstream issue #245).
func (a *Agent) resolveContextWindow() (maxContext, maxOutput int) {
	if rm, err := a.cfg.ResolveLLM(a.state.EffectiveModelID(a.cfg)); err == nil && rm != nil {
		maxOutput = rm.MaxTokens
	}
	maxContext, _ = a.contextWindow()
	if maxContext <= 0 {
		maxContext = config.DefaultContextWindowTokens
	}
	return maxContext, maxOutput
}

// compactionBoundary returns the index in history at which to cut: messages [0..boundary) are
// summarized and marked compacted, [boundary..) are kept verbatim. It cuts on a RoleUser turn
// boundary so an assistant's tool_calls are never split from their RoleTool results, and keeps
// the last keepLastTurns user turns. Returns 0 when there is nothing worth compacting.
func compactionBoundary(history []llm.Message, keepLastTurns int) int {
	if keepLastTurns < 1 {
		keepLastTurns = 1
	}
	var userIdx []int
	for i := range history {
		if history[i].Role == llm.RoleUser {
			userIdx = append(userIdx, i)
		}
	}
	if len(userIdx) <= keepLastTurns {
		return 0
	}
	boundary := userIdx[len(userIdx)-keepLastTurns]
	// Require at least one not-yet-compacted, non-summary message before the boundary; otherwise
	// there is nothing new to fold in and re-summarizing would only churn.
	hasFresh := false
	for i := 0; i < boundary; i++ {
		if !history[i].Compacted && !history[i].CompactionSummary {
			hasFresh = true
			break
		}
	}
	if !hasFresh {
		return 0
	}
	return boundary
}

// maybeCompact is the opencode engine's trigger. It checks whether the conversation is close
// enough to the context window to summarize older turns, against the provider's real input-token
// count of the previous call where there is one, and if so compacts. Returns true when a
// compaction happened. Failures are non-fatal - the caller continues with the full history.
//
// The trigger is this engine's own; the compaction is not. It goes through the same door as the
// coddy engine's (compactSession in compact.go), which is what gives this engine the PreCompact /
// PostCompact hooks, the summarizer chain with its fallbacks, the fold in passes for a head that
// outgrew the summarizer's window, and the live row. provider is the turn's provider: it stands in
// for the session model's summarizer.
func (a *Agent) maybeCompact(ctx context.Context, provider llm.Provider, lastInputTokens int) (bool, error) {
	if !a.cfg.Compaction.CompactionEnabled() {
		return false, nil
	}
	history := a.state.GetMessages()
	if len(history) == 0 {
		return false, nil
	}

	maxContext, maxOutput := a.resolveContextWindow()
	usable := maxContext - maxOutput
	if usable <= 0 {
		usable = maxContext
	}

	// Current prompt size: prefer the provider's real InputTokens from the previous turn, else the
	// estimated breakdown total (covers resumed sessions before any LLM call this run).
	current := lastInputTokens
	if current <= 0 {
		if rs, ok := a.state.(rulesState); ok {
			if b := rs.GetLastContextBreakdown(); b != nil {
				current = b.EstimatedTotal
			}
		}
	}
	if current <= 0 {
		current = session.EstimateTokens(conversationText(history)) +
			session.EstimateTokens(compactionSummaryText(history))
	}

	threshold := usable * a.cfg.Compaction.EffectiveThresholdPercent() / 100
	if threshold <= 0 || current < threshold {
		return false, nil
	}
	if compactionBoundary(history, a.cfg.Compaction.EffectiveKeepRecentTurns()) <= 0 {
		return false, nil
	}

	_, err := a.compactSession(ctx, "", false, compactionRun{opencode: true, turnProvider: provider})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrNothingToCompact), errors.Is(err, ErrEmptyCompactionSummary), errors.Is(err, ErrCompactionBlocked):
		// Nothing was discarded and nothing is wrong with the turn: it goes on
		// with the full window and the trigger asks again next step.
		return false, nil
	default:
		return false, err
	}
}

// compactOpenCode is the opencode engine's half of compactSession: it folds history[:boundary]
// and writes the result back by flagging those messages Compacted and inserting one
// CompactionSummary message in front of the kept tail. The persisted transcript keeps every
// original row; the model payload drops the flagged ones (isLLMHistoryMessage).
func (a *Agent) compactOpenCode(
	ctx context.Context,
	history []llm.Message,
	instructions string,
	run compactionRun,
	mode, trigger string,
) (*CompactionResult, error) {
	// compactionBoundary never keeps fewer than one turn, so the floor is the same for the
	// manual command and the trigger: this engine has no "summarize everything".
	keep := a.cfg.Compaction.EffectiveKeepRecentTurns()
	boundary := compactionBoundary(history, keep)
	for k := keep - 1; boundary <= 0 && k >= 1; k-- {
		boundary = compactionBoundary(history, k)
	}
	if boundary <= 0 {
		return nil, ErrNothingToCompact
	}

	// What is folded is what the model could still see before the boundary: the messages an
	// earlier compaction flagged are gone from the payload already, and the summary that
	// replaced them is part of the head - it is carried into the new summary instead of being
	// dropped with the rest, which is how a second compaction used to forget the first.
	visible := a.llmVisibleMessages()
	headLen := 0
	for i := 0; i < boundary; i++ {
		if isLLMHistoryMessage(history[i]) {
			headLen++
		}
	}
	projected := a.prunedForSummary(visible)
	if headLen > len(projected) {
		headLen = len(projected)
	}
	head := projected[:headLen]

	chain, err := a.compactionChainFor(run)
	if err != nil {
		return nil, fmt.Errorf("compaction model: %w", err)
	}
	if deduped, dropped := dedupeCompactionHead(head); dropped > 0 {
		a.log.Info("compaction dropped repeated lines from the history it is folding",
			"lines", dropped, "messages", len(head))
		head = deduped
	}
	window, _ := a.contextWindowFor(chain[0].modelID)
	budget := compactionInputBudget(window, session.EstimateTokens(instructions))

	sessionID := a.state.GetID()
	before := session.EstimateTokens(conversationText(history)) + session.EstimateTokens(compactionSummaryText(history))
	_ = a.server.SendSessionUpdate(sessionID, acp.CompactionUpdate{
		SessionUpdate: acp.UpdateTypeCompaction,
		Phase:         acp.CompactionPhaseStart,
		TokensBefore:  before,
	})
	// A client that was told a compaction started is always told how it ended, whatever
	// happened in between: the start frame is what raises its "compacting" state.
	done := acp.CompactionUpdate{
		SessionUpdate: acp.UpdateTypeCompaction,
		Phase:         acp.CompactionPhaseDone,
		TokensBefore:  before,
		TokensAfter:   before,
	}
	defer func() { _ = a.server.SendSessionUpdate(sessionID, done) }()

	row := a.newCompactionRow()
	summary, modelID, steps, err := a.foldCompactionHead(ctx, chain, head, instructions, budget, row.step)
	if err != nil {
		row.failed(err)
		return nil, err
	}

	newHistory := buildCompactedHistory(history, boundary, summary)
	a.state.ReplaceMessagesAndPersist(newHistory)
	// Republish the shrunken window so the context HUD drops without waiting for the next
	// system-prompt rebuild.
	a.refreshConversationContextUsage(true)
	a.runPostCompactHooks(ctx, mode, trigger, summary)

	done.RemovedMessages = boundary
	done.TokensAfter = session.EstimateTokens(conversationText(newHistory)) +
		session.EstimateTokens(compactionSummaryText(newHistory))
	a.log.Info("context compacted", "engine", "opencode", "removed_messages", boundary,
		"tokens_before", before, "tokens_after", done.TokensAfter, "steps", steps)

	res := &CompactionResult{
		Summary:           summary,
		CompactedMessages: len(head),
		KeptMessages:      len(history) - boundary,
		Model:             modelID,
		Steps:             steps,
	}
	row.done(compactionOutcomeText(res))
	return res, nil
}

// buildCompactedHistory marks [0..boundary) as Compacted, inserts one CompactionSummary message,
// and appends the retained tail. Original order is preserved so UI/replay stays consistent.
func buildCompactedHistory(history []llm.Message, boundary int, summary string) []llm.Message {
	out := make([]llm.Message, 0, boundary+1+len(history)-boundary)
	for i := 0; i < boundary; i++ {
		m := history[i]
		m.Compacted = true
		out = append(out, m)
	}
	out = append(out, llm.Message{
		Role:              llm.RoleUser,
		Content:           summaryPrefix + summary,
		CompactionSummary: true,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
	})
	out = append(out, history[boundary:]...)
	return out
}
