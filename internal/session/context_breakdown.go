package session

import "unicode/utf8"

// ContextBreakdown estimates token usage by prompt category.
type ContextBreakdown struct {
	SystemPrompt    int `json:"systemPrompt"`
	ToolDefinitions int `json:"toolDefinitions"`
	Rules           int `json:"rules"`
	Skills          int `json:"skills"`
	MCP             int `json:"mcp"`
	Subagents       int `json:"subagents"`
	Conversation    int `json:"conversation"`
	// Summary is the estimated size of compaction summary messages (older turns folded into a
	// concise summary by auto-compaction). Tracked separately from Conversation for the HUD.
	Summary        int `json:"summary"`
	EstimatedTotal int `json:"estimatedTotal"`
	// ProviderInputTokens anchors the last provider-reported prompt size to the
	// estimate taken immediately before that request. Later additions are
	// estimated relative to that point, including across session reloads.
	ProviderInputTokens    int    `json:"providerInputTokens,omitempty"`
	ProviderEstimateTokens int    `json:"providerEstimateTokens,omitempty"`
	ProviderModel          string `json:"providerModel,omitempty"`
}

// EstimateTokens approximates tokens for existing bounded compaction requests.
func EstimateTokens(s string) int {
	n := utf8.RuneCountInString(s)
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

// EstimateContextTokens is deliberately conservative for the automatic
// trigger. Tokenizers often spend about one token per non-ASCII rune, and
// code needs more room than the four-runes-per-token prose heuristic.
func EstimateContextTokens(s string) int {
	if s == "" {
		return 0
	}
	// ASCII prose is near four bytes per token; code is denser and non-ASCII
	// text (especially Cyrillic) is often around one token per rune.
	var ascii, other int
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+2)/3 + other
}

// Sum sets EstimatedTotal from parts.
func (b *ContextBreakdown) Sum() {
	if b == nil {
		return
	}
	b.EstimatedTotal = b.SystemPrompt + b.ToolDefinitions + b.Rules + b.Skills +
		b.MCP + b.Subagents + b.Conversation + b.Summary
}
