package agent

import (
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// llmVisibleMessages returns exactly the history slice buildMessages sends to the model, so a
// context estimate never counts turns the active compaction engine already dropped. The coddy
// engine replays only the window after the last summary row; the opencode engine keeps the full
// slice and filters messages flagged Compacted.
func (a *Agent) llmVisibleMessages() []llm.Message {
	history := a.state.GetMessages()
	if a.cfg.Compaction.EngineIsCoddy() {
		return session.MessagesForLLM(history)
	}
	out := make([]llm.Message, 0, len(history))
	for _, m := range history {
		if isLLMHistoryMessage(m) {
			out = append(out, m)
		}
	}
	return out
}

// setContextBreakdown stores and publishes the current model-window estimate.
// Provider token usage is intentionally separate: ACP usage_update is current
// context state, while the token_usage update tracks completed LLM calls.
func (a *Agent) setContextBreakdown(b *session.ContextBreakdown, persist bool) {
	if a == nil || b == nil {
		return
	}
	rs, ok := a.state.(rulesState)
	if !ok {
		return
	}
	cp := *b
	cp.Sum()
	rs.SetLastContextBreakdown(&cp)

	if persist {
		if sd := strings.TrimSpace(a.state.GetPersistedSessionDir()); sd != "" {
			if err := session.WriteSessionContextBreakdown(sd, &cp); err != nil {
				a.log.Warn("persist context usage", "error", err)
			}
		}
	}

	if a.server == nil || a.cfg == nil {
		return
	}
	ent := a.cfg.FindModelEntry(a.state.EffectiveModelID(a.cfg))
	if ent == nil || ent.MaxContextTokens <= 0 {
		return
	}
	_ = a.server.SendSessionUpdate(a.state.GetID(), acp.UsageUpdate{
		SessionUpdate: acp.UpdateTypeUsage,
		Used:          cp.EstimatedTotal,
		Size:          ent.MaxContextTokens,
	})
}

// refreshConversationContextUsage keeps the static categories from the most recent rendered
// system prompt and recalculates the transcript categories after compaction or after a newly
// persisted message. Conversation and Summary are both recomputed because compaction moves text
// from the former into the latter.
func (a *Agent) refreshConversationContextUsage(persist bool) {
	if a == nil {
		return
	}
	rs, ok := a.state.(rulesState)
	if !ok {
		return
	}
	b := rs.GetLastContextBreakdown()
	if b == nil {
		b = &session.ContextBreakdown{}
	}
	visible := a.llmVisibleMessages()
	b.Conversation = session.EstimateTokens(conversationText(visible))
	b.Summary = session.EstimateTokens(compactionSummaryText(visible))
	a.setContextBreakdown(b, persist)
}
