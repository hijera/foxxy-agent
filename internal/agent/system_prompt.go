package agent

import (
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/prompts"
	"github.com/hijera/foxxycode-agent/internal/rules"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/skills"
	"github.com/hijera/foxxycode-agent/internal/tools"
	"github.com/hijera/foxxycode-agent/internal/tools/todo"
)

func joinNonEmptyPromptBlocks(parts ...string) string {
	var b []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			b = append(b, p)
		}
	}
	return strings.Join(b, "\n\n")
}

// buildSkillsPromptMarkdown merges the slash catalog and bodies of context-matched non-command skills.
// Slash-command bodies (invoked via /name) are NOT included here — they are injected directly into
// the user message by buildMessages so the LLM sees them close to the user's request.
func buildSkillsPromptMarkdown(allLoaded []*skills.Skill, active []*skills.Skill) string {
	activeDedup := skills.DedupeSkillsByCanonicalName(active)
	skillSums := skills.ListSkills(allLoaded)
	catalogNameSet := make(map[string]struct{}, len(skillSums))
	for _, s := range skillSums {
		catalogNameSet[s.Name] = struct{}{}
	}

	// Active section: skill bodies for context-matched skills that are NOT slash commands.
	// Slash commands are listed in the catalog only; their bodies are injected into the user
	// message on explicit invocation so the LLM sees them as close to the request as possible.
	var activeForSection []*skills.Skill
	for _, sk := range activeDedup {
		n := skills.CanonicalCommandName(sk)
		if n != "" {
			if _, inCat := catalogNameSet[n]; inCat {
				continue
			}
		}
		activeForSection = append(activeForSection, sk)
	}

	catalog := skills.BuildSlashCatalogMarkdown(skillSums)
	section := skills.BuildSystemPromptSection(activeForSection)
	return joinNonEmptyPromptBlocks(catalog, section)
}

// systemPromptBuild is a rendered system message plus what the turn still needs
// from it once it is frozen: the component blocks the context estimate
// subtracts, the tool definitions it described, and the sticky rule set it
// already carries, so a rule activated later can be told apart from one the
// model has already been given.
type systemPromptBuild struct {
	Mode     string
	Content  string
	SkillsMD string
	ToolsMD  string
	RulesMD  string
	ToolDefs []llm.ToolDefinition
	// RenderedRules is what the {{.Rules}} block of this message carries, and
	// what the turn context block diffs a later activation against.
	RenderedRules []*rules.Rule
	// Clock is the wall clock this build was stamped with: the {{.UTCNow}} a
	// template may render and the reading the turn context block carries. It is
	// taken once per turn and reused by every step, so a step the lane re-issues
	// sends byte for byte the request that failed (react.go, lane replays)
	// rather than one that ticked a second forward.
	Clock time.Time
	// RendersRules is false for a template under prompts.dir with no
	// {{.Rules}} in it. That operator asked for no rules block at all, so a
	// rule a tool call activates is not smuggled in after the history either.
	RendersRules bool
	// Volatile marks a template under prompts.dir that prints {{.UTCNow}} or
	// {{.TodoList}}. Such a message cannot be frozen for the turn - its own
	// conditionals have to keep matching the state - so the loop re-renders it
	// before every call, as it did for every template before, and sends no turn
	// context block: the template is already carrying what the block would say.
	Volatile bool
}

// buildSystemPrompt constructs the system prompt for the current mode and skills.
func (a *Agent) buildSystemPrompt(mode string, activeSkills []*skills.Skill, toolDefs []llm.ToolDefinition, userText string, contextFiles []string) string {
	return a.buildSystemPromptParts(mode, activeSkills, toolDefs, userText, contextFiles).Content
}

// buildSystemPromptParts renders the system message for the turn. It is built
// once per turn and then frozen: a provider caches a request by its prefix, and
// the system message sits in front of the whole conversation, so rewriting it
// between the steps of a turn throws away the cached copy of everything behind
// it. What moves while the turn runs - the wall clock, the todo checklist, the
// rules a tool call activated - travels in the turn context block appended
// after the history instead (turn_context.go).
func (a *Agent) buildSystemPromptParts(mode string, activeSkills []*skills.Skill, toolDefs []llm.ToolDefinition, userText string, contextFiles []string) *systemPromptBuild {
	promptsDir := a.cfg.Prompts.ResolvedDir(a.state.GetCWD())
	clock := a.now().UTC()
	promptTodoMD := checklistMarkdownFromPlan(a.state.GetPlan())
	mem := formatMergedMemory(strings.TrimSpace(a.state.GetAgentMemory()), strings.TrimSpace(a.state.GetMemoryCopilotBlock()))
	planCtx := ""
	if mode == "agent" {
		// Read, never taken. The turn it belongs to renders this prompt more
		// than once - a rebuild after a compaction, the continuation after a
		// permission prompt - and releasePlanContext hands it back when that
		// turn is really over (react.go).
		planCtx = a.state.PendingPlanContext()
	}
	discardedPlans := ""
	if mode == "plan" {
		discardedPlans = discardedPlansPromptBlock(a.state.DiscardedPlanSlugs())
	}
	skillsMD := buildSkillsPromptMarkdown(a.state.GetSkills(), activeSkills)
	toolsMD := tools.FormatDefinitionsForPrompt(toolDefs)
	// The per-provider template variants are settled first: whether the chosen
	// template renders {{.Rules}} decides what the instructions block leaves out.
	var promptVariants []string
	if a.cfg.Prompts.PerProviderEnabled() {
		promptVariants = a.promptVariants()
	}
	rulesMD := ""
	// Project docs the rules block already carries: instructions.files names
	// AGENTS.md too, and one system prompt does not need it twice. A template
	// under prompts.dir may render {{.Instructions}} and not {{.Rules}}, and
	// then nothing carries them - so the skip list is taken only from a
	// template that actually prints the block.
	var embeddedDocs []string
	var renderedRules []*rules.Rule
	rendersRules := prompts.RendersRules(mode, promptVariants, promptsDir, a.cfg.Prompts.AgentFile(), a.cfg.Prompts.PlanFile(), a.cfg.Prompts.DocsFile(), a.cfg.Prompts.AskFile())
	if rs, ok := a.state.(rulesState); ok {
		rulesMD, embeddedDocs, renderedRules = buildRulesPromptMarkdown(rs, a.cfg.Paths.Home, contextFiles, userText, a.agentsOnDemand())
		if !rendersRules {
			embeddedDocs = nil
		}
	}
	instructionsMD := session.LoadInstructions(a.state.GetCWD(), a.cfg.Paths.Home, a.cfg.Instructions.Files, embeddedDocs)
	intellijContextMD := session.LoadIntelliJProjectContext(a.state.GetCWD())
	vscodeContextMD := session.LoadVSCodeProjectContext(a.state.GetCWD())
	full := prompts.RenderWithFallbackForVariants(mode, promptVariants, promptsDir, a.cfg.Prompts.AgentFile(), a.cfg.Prompts.PlanFile(), a.cfg.Prompts.DocsFile(), a.cfg.Prompts.AskFile(), prompts.TemplateData{
		CWD:            a.state.GetCWD(),
		Skills:         skillsMD,
		Rules:          rulesMD,
		Tools:          toolsMD,
		Memory:         mem,
		TodoList:       promptTodoMD,
		PlanContext:    planCtx,
		DiscardedPlans: discardedPlans,
		Instructions:   instructionsMD,
		Subagents:      a.subagentCatalogBlock(),
		SubagentRole:   a.subagentRoleBlock(),
		// The built-in templates no longer render this: a wall clock in the
		// system message breaks the provider's prefix cache on every request,
		// and the turn context block carries the clock instead. It stays
		// available to an operator's own prompts.dir template, at that cost.
		UTCNow: clock.Format(time.RFC3339),
	})
	// The identity sentence has to fall inside the window a gateway inspects (see
	// internal/prompts/identity.go), and this fork opens its prompts with a language
	// directive long enough to push a template's own opening out of that window. So
	// the identity is prepended in front of the directive rather than applied to the
	// finished prompt, and the built-in templates stay generic - otherwise the line
	// would appear twice.
	full = prompts.WithIdentity(languageDirective(a.cfg.UI.Locale) + "\n\n" + full)
	// Appended outside the configurable template so custom prompts cannot drop IDE metadata or platform facts.
	full = joinNonEmptyPromptBlocks(full, intellijContextMD, vscodeContextMD, a.environment.PromptContext())
	// Context handed over by SessionStart and UserPromptSubmit hooks; appended
	// like the environment block so a custom template carries it too.
	full = joinNonEmptyPromptBlocks(full, a.hookContextBlock())
	// What the surface running this turn asked the model to know about
	// answering through it. Last of the appended blocks, so a messenger's
	// answer format is the nearest instruction to the conversation itself.
	full = joinNonEmptyPromptBlocks(full, a.surfaceBlock())
	build := &systemPromptBuild{
		Mode:          mode,
		Content:       full,
		SkillsMD:      skillsMD,
		ToolsMD:       toolsMD,
		RulesMD:       rulesMD,
		ToolDefs:      toolDefs,
		RenderedRules: renderedRules,
		Clock:         clock,
		RendersRules:  rendersRules,
		// The variant list is the one this render resolved, so the verdict
		// follows the footer the provider family actually got.
		Volatile: prompts.RendersVolatile(mode, promptVariants, promptsDir, a.cfg.Prompts.AgentFile(), a.cfg.Prompts.PlanFile(), a.cfg.Prompts.DocsFile(), a.cfg.Prompts.AskFile()),
	}
	// Counted with the block its requests will carry, as the loop counts every
	// step: this is the estimate usage_update reports before the first model call
	// and the one the coddy trigger reads before the loop.
	a.refreshContextBreakdown(build, a.buildTurnContext(build))
	return build
}

// refreshContextBreakdown re-estimates the context UI from a frozen system
// prompt. The loop calls it every step, because the estimate is what
// auto-compaction reads and tool results grow between calls while the system
// message no longer moves. turnCtx is the block that will trail the history, so
// what the request actually costs is counted.
func (a *Agent) refreshContextBreakdown(build *systemPromptBuild, turnCtx string) {
	if build == nil {
		return
	}
	if _, ok := a.state.(rulesState); !ok {
		return
	}
	// The Conversation estimate mirrors what buildMessages sends: only the window
	// the active compaction engine still replays.
	sys := joinNonEmptyPromptBlocks(build.Content, turnCtx)
	msgs := a.prunedForLLM(a.llmVisibleMessages())
	a.setContextBreakdown(computeContextBreakdown(sys, build.SkillsMD, build.ToolsMD, build.RulesMD, msgs, build.ToolDefs), false)
}

// languageDirective returns a system-prompt instruction telling the model which
// human language to think and respond in, based on the UI locale
// (config.UIConfig.Locale: "" auto-detect, "en", or "ru").
func languageDirective(locale string) string {
	const heading = "## Response language\n\n"
	const tail = " Only keep code, identifiers, commands, and file paths in their original form."
	switch strings.TrimSpace(locale) {
	case "ru":
		return heading + "Always think and respond in **Russian**, regardless of the language of the " +
			"code, file contents, tool output, or this system prompt." + tail
	case "en":
		return heading + "Always think and respond in **English**, regardless of the language of the " +
			"code, file contents, tool output, or this system prompt." + tail
	default: // "" auto-detect
		return heading + "Think and respond in the same language the user writes to you in." + tail
	}
}

// promptVariants returns the ordered model-reference, resolved API-model, then family prompt
// keys for the active model, most-specific first. Including the API-model slug lets a built-in
// model variant work across arbitrary provider names (for example local/gpt-oss-20b). Empty and
// duplicate keys are dropped.
func (a *Agent) promptVariants() []string {
	modelID := a.state.EffectiveModelID(a.cfg)
	modelSlug := prompts.ModelSlug(modelID)
	apiModelSlug := ""
	family := ""
	if rm, err := a.cfg.ResolveLLM(modelID); err == nil {
		apiModelSlug = prompts.ModelSlug(rm.Model)
		family = prompts.Family(rm.ProviderType, rm.Model)
	}
	var variants []string
	for _, v := range []string{modelSlug, apiModelSlug, family} {
		if v == "" {
			continue
		}
		dup := false
		for _, existing := range variants {
			if existing == v {
				dup = true
				break
			}
		}
		if !dup {
			variants = append(variants, v)
		}
	}
	return variants
}

func discardedPlansPromptBlock(slugs []string) string {
	if len(slugs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Discarded design plans\n\n")
	b.WriteString("The user discarded these plan files in the UI. Do **not** reuse their slugs or recycle their plan titles. ")
	b.WriteString("When you write a new design plan, pick a **fresh slug** and a **new name** until the user leaves plan mode.\n\n")
	for _, slug := range slugs {
		b.WriteString("- `")
		b.WriteString(slug)
		b.WriteString("`\n")
	}
	return strings.TrimSpace(b.String())
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

// loadSkillBody returns a loaded skill's full instruction body by its command name (with or
// without the leading slash), plus the list of available command names. It backs the model-driven
// load_skill tool (skills.auto_discovery).
func (a *Agent) loadSkillBody(name string) (string, []string, bool) {
	idx := skills.SkillBySlashName(a.state.GetSkills())
	available := make([]string, 0, len(idx))
	for n := range idx {
		available = append(available, n)
	}
	if sk, ok := idx[strings.TrimSpace(name)]; ok {
		return strings.TrimSpace(sk.Content), available, true
	}
	return "", available, false
}
