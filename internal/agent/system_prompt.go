package agent

import (
	"strings"
	"time"

	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	"github.com/EvilFreelancer/coddy-agent/internal/llm"
	"github.com/EvilFreelancer/coddy-agent/internal/prompts"
	"github.com/EvilFreelancer/coddy-agent/internal/skills"
	"github.com/EvilFreelancer/coddy-agent/internal/tools"
	"github.com/EvilFreelancer/coddy-agent/internal/tools/todo"
)

// buildSystemPrompt constructs the system prompt for the current mode and skills.
// It is rebuilt each agent turn so the checklist section stays aligned with todo tool mutations.
func (a *Agent) buildSystemPrompt(mode string, activeSkills []*skills.Skill, toolDefs []llm.ToolDefinition) string {
	promptsDir := a.cfg.Prompts.ResolvedDir(a.state.GetCWD())
	promptTodoMD := checklistMarkdownFromPlan(a.state.GetPlan())
	mem := formatMergedMemory(strings.TrimSpace(a.state.GetAgentMemory()), strings.TrimSpace(a.state.GetMemoryCopilotBlock()))
	return prompts.RenderWithFallback(mode, promptsDir, a.cfg.Prompts.AgentFile(), a.cfg.Prompts.PlanFile(), prompts.TemplateData{
		CWD:      a.state.GetCWD(),
		Skills:   skills.BuildSystemPromptSection(activeSkills),
		Tools:    tools.FormatDefinitionsForPrompt(toolDefs),
		Memory:   mem,
		TodoList: promptTodoMD,
		UTCNow:   time.Now().UTC().Format(time.RFC3339),
	})
}

// checklistMarkdownFromPlan renders the session plan for embedding in prompts (trimmed checklist text).
func checklistMarkdownFromPlan(entries []acp.PlanEntry) string {
	return strings.TrimSpace(todo.FormatPlanMarkdown(entries))
}

func formatMergedMemory(sessionNotes, recall string) string {
	var parts []string
	if recall != "" {
		parts = append(parts, recall)
	}
	if sessionNotes != "" {
		parts = append(parts, "Session notes:\n"+sessionNotes)
	}
	return strings.Join(parts, "\n\n")
}
