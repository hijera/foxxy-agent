package agent

import (
	"fmt"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// Reaching the model's output cap is the one ending that still looks exactly like
// a finished answer. The stream closes cleanly - finish_reason: length followed by
// [DONE] - so the transport reports success, the turn returns StopReasonMaxTokens,
// and nothing renders that reason: the answer simply stops mid-sentence. Unlike a
// stall, there is nothing to wait out and nothing to continue, because the model
// did not fail - it ran out of the budget the configuration gave it. The only
// useful response is to say so, and to name the setting that would raise it.

// effectiveMaxTokens reports the output cap configured for the model this turn
// runs on, or 0 when no cap is sent and the provider's own default applies.
func (a *Agent) effectiveMaxTokens() int {
	rm, err := a.cfg.ResolveLLM(a.state.EffectiveModelID(a.cfg))
	if err != nil || rm == nil {
		return 0
	}
	return rm.MaxTokens
}

// maxTokensNotice phrases an output-cap truncation for the transcript. Reasoning
// models get their own sentence: their thinking is billed against the same budget
// but never appears on screen, so the visible answer can be a fraction of what the
// cap nominally allowed. Measured on kimi-k2.6 at max_tokens 8192, roughly half
// the budget went to reasoning the user never saw.
func maxTokensNotice(configuredCap, outputTokens int) string {
	const lead = "The answer above is cut off: the model hit its output limit and stopped mid-answer."
	if configuredCap > 0 {
		return fmt.Sprintf(
			"%s It produced %d of the %d tokens allowed by models[].max_tokens for this model."+
				" Raise that value to let it finish; on a reasoning model the hidden thinking spends"+
				" part of the same budget, so the visible answer runs out well before the number suggests.",
			lead, outputTokens, configuredCap)
	}
	return lead + " No models[].max_tokens is configured, so the provider's own default cap applied." +
		" Set max_tokens for this model to raise it."
}

// persistTruncationNotice streams a notice and stores it in the transcript, so the
// reason a turn ended short survives a reload the same way the answer does.
func (a *Agent) persistTruncationNotice(text string) {
	_ = a.server.SendSessionUpdate(a.state.GetID(), acp.MessageChunkUpdate{
		SessionUpdate: acp.UpdateTypeAgentMessageChunk,
		Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: "\n\n" + text},
	})
	a.state.AddMessage(llm.Message{
		Role:      llm.RoleAssistant,
		Content:   text,
		Model:     a.state.EffectiveModelID(a.cfg),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
