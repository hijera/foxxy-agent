package agent

import (
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/rules"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/skills"
)

// rulesState is implemented by session.State for rules prompt wiring.
type rulesState interface {
	GetCWD() string
	GetRulesCatalog() []*rules.Rule
	GetActiveAutoRules() []*rules.Rule
	SetActiveAutoRules([]*rules.Rule)
	GetMessages() []llm.Message
	GetLastContextBreakdown() *session.ContextBreakdown
	SetLastContextBreakdown(*session.ContextBreakdown)
}

func buildRulesPromptMarkdown(st rulesState, contextFiles []string, userText string) string {
	catalog := st.GetRulesCatalog()
	newAuto := rules.MatchAuto(catalog, contextFiles)
	sticky := rules.UnionStable(st.GetActiveAutoRules(), newAuto)
	st.SetActiveAutoRules(sticky)
	mentioned := rules.SelectMentioned(catalog, userText)
	return rules.RenderPrompt(st.GetCWD(), sticky, mentioned)
}

// computeContextBreakdown estimates category sizes for the context UI.
// fullSystem is the rendered system message; tools/skills/rules are subtracted for SystemPrompt.
func computeContextBreakdown(
	fullSystem string,
	skillsMD, toolsMD, rulesMD string,
	messages []llm.Message,
	toolDefs []llm.ToolDefinition,
) *session.ContextBreakdown {
	toolsTok := session.EstimateTokens(toolsMD)
	rulesTok := session.EstimateTokens(rulesMD)
	skillsTok := session.EstimateTokens(skillsMD)
	mcpTok := estimateMCPTokens(toolDefs)
	convTok := session.EstimateTokens(conversationText(messages))
	summaryTok := session.EstimateTokens(compactionSummaryText(messages))
	fullTok := session.EstimateTokens(fullSystem)
	sysTok := fullTok - toolsTok - rulesTok - skillsTok
	if sysTok < 0 {
		sysTok = 0
	}
	b := &session.ContextBreakdown{
		SystemPrompt:    sysTok,
		ToolDefinitions: toolsTok,
		Rules:           rulesTok,
		Skills:          skillsTok,
		MCP:             mcpTok,
		Subagents:       0,
		Conversation:    convTok,
		Summary:         summaryTok,
	}
	b.Sum()
	return b
}

// conversationText concatenates the messages that are actually sent to the model. Compacted
// messages (superseded by a summary) and the summary itself are excluded — the summary is
// accounted for separately via compactionSummaryText / the Summary breakdown category.
func conversationText(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Compacted || m.CompactionSummary {
			continue
		}
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		b.WriteString(string(m.Role))
		b.WriteString(":\n")
		b.WriteString(m.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

// compactionSummaryText concatenates the content of compaction summary messages.
func compactionSummaryText(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if !m.CompactionSummary || strings.TrimSpace(m.Content) == "" {
			continue
		}
		b.WriteString(m.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

func estimateMCPTokens(defs []llm.ToolDefinition) int {
	var b strings.Builder
	for _, d := range defs {
		if strings.Contains(d.Name, "__") {
			b.WriteString(d.Name)
			b.WriteString(d.Description)
		}
	}
	if b.Len() == 0 {
		return 0
	}
	return session.EstimateTokens(b.String())
}

// FilterSkillsForContext wraps skills filter (unchanged semantics for skills only).
func FilterSkillsForContext(all []*skills.Skill, contextFiles []string) []*skills.Skill {
	return skills.FilterForContext(all, contextFiles)
}
