package agent

import (
	"context"
	_ "embed"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

//go:embed prompts/compaction.md
var compactionSystemPrompt string

// compactionDefaultContextWindow is the assumed context window when models[].max_context_tokens
// is unset, matching the UI HUD fallback. Without it, auto-compaction could never trigger for
// configs that omit the window.
const compactionDefaultContextWindow = 128000

// summaryPrefix precedes the generated summary in the synthetic message sent to the model. It is
// deliberately short and in English (the summary body follows in the conversation's language).
const summaryPrefix = "Summary of the earlier conversation (older turns were compacted to save context):\n\n"

// resolveContextWindow returns the model's context window and per-completion output cap.
// Falls back to a default window when max_context_tokens is unset.
func (a *Agent) resolveContextWindow() (maxContext, maxOutput int) {
	modelID := a.state.EffectiveModelID(a.cfg)
	if rm, err := a.cfg.ResolveLLM(modelID); err == nil && rm != nil {
		maxContext = rm.MaxContextTokens
		maxOutput = rm.MaxTokens
	}
	if maxContext <= 0 {
		maxContext = compactionDefaultContextWindow
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

// compactionProvider returns the provider used for the summarization pass: a dedicated model when
// compaction.model is configured, otherwise the passed-in main provider. Any resolution error
// falls back to the main provider rather than failing the turn.
func (a *Agent) compactionProvider(fallback llm.Provider) llm.Provider {
	ref := strings.TrimSpace(a.cfg.Compaction.Model)
	if ref == "" {
		return fallback
	}
	rm, err := a.cfg.ResolveLLM(ref)
	if err != nil || rm == nil {
		return fallback
	}
	if cap := a.cfg.Compaction.MaxTokens; cap > 0 && (rm.MaxTokens <= 0 || rm.MaxTokens > cap) {
		rm.MaxTokens = cap
	}
	mk := a.providerFactory
	if mk == nil {
		mk = llm.NewProvider
	}
	p, err := mk(a.llmProviderInput(rm))
	if err != nil || p == nil {
		return fallback
	}
	return p
}

// maybeCompact checks whether the conversation is close enough to the context window to summarize
// older turns, and if so performs the compaction: it rewrites the persisted history so [0..boundary)
// are marked Compacted (excluded from the model payload but kept for UI/replay) and a single
// CompactionSummary message is inserted. Returns true when a compaction happened. Failures are
// non-fatal — the caller continues with the full history.
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

	threshold := usable * a.cfg.Compaction.ThresholdPercent / 100
	if threshold <= 0 || current < threshold {
		return false, nil
	}

	boundary := compactionBoundary(history, a.cfg.Compaction.KeepLastTurns)
	if boundary <= 0 {
		return false, nil
	}

	sessionID := a.state.GetID()
	_ = a.server.SendSessionUpdate(sessionID, acp.CompactionUpdate{
		SessionUpdate: acp.UpdateTypeCompaction,
		Phase:         acp.CompactionPhaseStart,
		TokensBefore:  current,
	})

	summary, err := a.summarize(ctx, provider, history[:boundary])
	if err != nil {
		return false, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		// Never discard history without a usable summary.
		return false, nil
	}

	newHistory := buildCompactedHistory(history, boundary, summary)
	a.state.ReplaceMessagesAndPersist(newHistory)

	after := session.EstimateTokens(conversationText(newHistory)) +
		session.EstimateTokens(compactionSummaryText(newHistory))
	_ = a.server.SendSessionUpdate(sessionID, acp.CompactionUpdate{
		SessionUpdate:   acp.UpdateTypeCompaction,
		Phase:           acp.CompactionPhaseDone,
		RemovedMessages: boundary,
		TokensBefore:    current,
		TokensAfter:     after,
	})
	if a.log != nil {
		a.log.Info("context compacted", "removed_messages", boundary, "tokens_before", current, "tokens_after", after)
	}
	return true, nil
}

// summarize runs a single non-streaming completion that condenses old into a plain-prose summary.
func (a *Agent) summarize(ctx context.Context, provider llm.Provider, old []llm.Message) (string, error) {
	p := a.compactionProvider(provider)
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: compactionSystemPrompt},
		{Role: llm.RoleUser, Content: conversationText(old)},
	}
	resp, err := p.Complete(ctx, msgs, nil)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.Content, nil
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
