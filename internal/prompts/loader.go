// Package prompts manages system prompt templates for each agent mode.
//
// Built-in (embedded) prompts are assembled from reusable Markdown section
// fragments under sections/ (see sections.go): each (mode, variant) is an
// ordered list of fragments that the loader concatenates into one template
// source, then parses with Go text/template. This keeps a single source of
// truth for every shared block (the agent body, the conditional footer, the
// read/search and background guidance that used to drift out of per-family
// forks) and reduces tuning an already-classified family to small fragments.
//
// Custom prompts loaded from a directory configured under YAML key prompts
// (config.Prompts in internal/config/prompts.go) keep the legacy
// one-file-per-mode shape and bypass section assembly entirely.
//
// Template variables available in the fragments / custom files:
//
//	{{.CWD}}      - session working directory
//	{{.Tools}}    - readable list of tools available in the current mode (markdown)
//	{{.Skills}}   - active skills markdown (slash catalog and bodies), built by the agent
//	{{.Rules}}    - project docs (root AGENTS.md, DESIGN.md) and active project rules markdown (may be empty)
//	{{.Memory}}   - session agent memory notes (may be empty)
//	{{.TodoList}} - current session todo checklist rendered as markdown (empty until plan tools populate state)
//	{{.PlanContext}}    - design plan text injected when the user runs a saved plan (agent mode, may be empty)
//	{{.DiscardedPlans}} - plan-mode guidance when the user discarded design plan slugs (may be empty)
//	{{.Instructions}}   - concatenated instructions.files, minus files {{.Rules}} already carries (may be empty)
//	{{.Subagents}}      - catalog of subagents the session may spawn (may be empty)
//	{{.SubagentRole}}   - role block when this session is itself a subagent run (may be empty)
//	{{.UTCNow}}   - current date and time in UTC (RFC3339), set each time the system prompt renders
//
// Use {{if .Skills}}...{{end}} (and similarly for .Tools, .Memory, .TodoList) when sections should be omitted when empty.
//
// The ReAct runner renders the system prompt once per turn and then freezes it: the provider caches
// a request by its prefix, and the system message sits in front of the whole conversation, so a byte
// that moves there throws away the cached copy of every message behind it. The built-in templates
// therefore render neither {{.UTCNow}} nor {{.TodoList}}; the runner sends the clock, the checklist
// and the rules a tool call activated after the history, in a <turn_context> block
// (internal/agent/turn_context.go). Both fields stay populated for a template under prompts.dir that
// wants them anyway, at the cost of that cache on every request.
package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

const (
	fileAgent = "agent.md"
	filePlan  = "plan.md"
	fileDocs  = "docs.md"
	fileAsk   = "ask.md"
	fileDebug = "debug.md"
)

// TemplateData holds values injected into prompt templates.
type TemplateData struct {
	// CWD is the session working directory.
	CWD string

	// Skills is preformatted markdown for slash skills (may be empty).
	Skills string

	// Rules is preformatted markdown for the project docs (the root AGENTS.md and DESIGN.md)
	// and the active project rules (may be empty).
	Rules string

	// Tools is a human-readable markdown list of tools for the current mode (may be empty).
	Tools string

	// Memory is session-scoped notes injected into the prompt (may be empty).
	Memory string

	// TodoList is the current session checklist as markdown lines (may be empty).
	// The built-in templates do not render it: it is rewritten by every
	// foxxycode_todo_* call and travels in the turn context block instead.
	TodoList string

	// PlanContext is design plan text injected when the user runs a saved plan (may be empty).
	PlanContext string

	// DiscardedPlans is plan-mode guidance when the user discarded design plan slugs (may be empty).
	DiscardedPlans string

	// Instructions is the concatenated content of the instructions.files, leaving out any file
	// Rules already carries (the root AGENTS.md is the default entry), may be empty.
	Instructions string

	// Subagents is the catalog block a parent that may spawn subagents reads (may be empty).
	Subagents string

	// SubagentRole is the role block of a child agent run (may be empty).
	SubagentRole string

	// UTCNow is the wall-clock instant in RFC3339 (UTC) at render time for model
	// grounding. The built-in templates do not render it: a clock in the system
	// prompt is a cache miss on every request, and the turn context block carries
	// it after the history.
	UTCNow string
}

// Embedded default prompts are assembled from reusable section fragments in
// sections/ (see sections.go). On-disk custom prompts keep the legacy
// one-file-per-mode shape described by loadSource.

// Render renders the prompt template for the given mode with the provided data.
// promptsDir must be empty to use built-in templates; otherwise it is a directory that
// contains the files named agentFile, planFile, and docsFile plus askFile.
// mode must be "agent", "plan", "docs", or "ask". Unknown modes use the agent template file.
func Render(mode, promptsDir, agentFile, planFile, docsFile, askFile string, data TemplateData) (string, error) {
	return RenderForFamily(mode, "", promptsDir, agentFile, planFile, docsFile, askFile, data)
}

// RenderForFamily is Render with a provider family. When family is non-empty it selects the
// per-family template variant (for example agent.anthropic.md), falling back to the base
// per-mode template when the variant does not exist. family "" behaves exactly like Render.
func RenderForFamily(mode, family, promptsDir, agentFile, planFile, docsFile, askFile string, data TemplateData) (string, error) {
	return RenderForVariants(mode, familyVariants(family), promptsDir, agentFile, planFile, docsFile, askFile, data)
}

// RenderForVariants is Render with an ordered list of variant keys, most-specific first
// (for example model-reference slug, API-model slug, then provider family). Embedded prompts
// resolve the manifest and each fragment across that list; custom prompt directories select
// the first complete <mode>.<key>.md file. A nil or empty list behaves exactly like Render.
func RenderForVariants(mode string, variants []string, promptsDir, agentFile, planFile, docsFile, askFile string, data TemplateData) (string, error) {
	src, err := loadSource(mode, variants, promptsDir, agentFile, planFile, docsFile, askFile)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(mode).Parse(src)
	if err != nil {
		return "", fmt.Errorf("parse prompt template %q: %w", mode, err)
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render prompt template %q: %w", mode, err)
	}

	return strings.TrimSpace(b.String()), nil
}

// RenderWithFallback renders the prompt and returns a safe default on error.
func RenderWithFallback(mode, promptsDir, agentFile, planFile, docsFile, askFile string, data TemplateData) string {
	return RenderWithFallbackForVariants(mode, nil, promptsDir, agentFile, planFile, docsFile, askFile, data)
}

// RenderWithFallbackForFamily renders the per-family prompt and returns a safe default on error.
func RenderWithFallbackForFamily(mode, family, promptsDir, agentFile, planFile, docsFile, askFile string, data TemplateData) string {
	return RenderWithFallbackForVariants(mode, familyVariants(family), promptsDir, agentFile, planFile, docsFile, askFile, data)
}

// RenderWithFallbackForVariants renders the most-specific available variant and returns a
// safe default on error.
func RenderWithFallbackForVariants(mode string, variants []string, promptsDir, agentFile, planFile, docsFile, askFile string, data TemplateData) string {
	s, err := RenderForVariants(mode, variants, promptsDir, agentFile, planFile, docsFile, askFile, data)
	if err != nil {
		return fallbackPrompt(mode, data.CWD)
	}
	return s
}

// familyVariants wraps a single family key into a variant list (empty family -> nil).
func familyVariants(family string) []string {
	if strings.TrimSpace(family) == "" {
		return nil
	}
	return []string{family}
}

// DefaultSource returns the built-in template source for a mode, assembled from
// the shared section fragments in sections/. Useful for displaying to the user
// so they can customize it.
func DefaultSource(mode string) string {
	return assembleEmbeddedSource(mode, nil)
}

// familyFileName inserts ".<family>" before the extension of base.
// familyFileName("agent.md", "anthropic") == "agent.anthropic.md".
// An empty family returns base unchanged.
func familyFileName(base, family string) string {
	f := strings.TrimSpace(family)
	if f == "" {
		return base
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return stem + "." + f + ext
}

func fileNameForMode(mode string) string {
	switch mode {
	case "plan":
		return filePlan
	case "docs":
		return fileDocs
	case "ask":
		return fileAsk
	case "debug":
		return fileDebug
	default:
		return fileAgent
	}
}

// loadSource returns the template source: files from promptsDir when set, built-in otherwise.
// variants is an ordered list of keys tried most-specific first (agent.<key>.md); the base
// file is always the final fallback (embedded for built-ins, on-disk for promptsDir).
func loadSource(mode string, variants []string, promptsDir, agentFile, planFile, docsFile, askFile string) (string, error) {
	dir := strings.TrimSpace(promptsDir)
	if dir == "" {
		return assembleEmbeddedSource(mode, variants), nil
	}

	base := strings.TrimSpace(agentFile)
	switch mode {
	case "plan":
		base = strings.TrimSpace(planFile)
	case "docs":
		base = strings.TrimSpace(docsFile)
	case "ask":
		base = strings.TrimSpace(askFile)
	case "debug":
		base = fileDebug
	}
	if base == "" {
		base = fileNameForMode(mode)
	}

	// Prefer variant files on disk (most-specific first), then fall back to the base file.
	candidates := make([]string, 0, len(variants)+1)
	seen := make(map[string]struct{}, len(variants)+1)
	add := func(fn string) {
		if _, dup := seen[fn]; dup {
			return
		}
		seen[fn] = struct{}{}
		candidates = append(candidates, fn)
	}
	for _, v := range variants {
		if fam := familyFileName(base, v); fam != base {
			add(fam)
		}
	}
	add(base)

	var lastErr error
	for _, fn := range candidates {
		path := filepath.Join(dir, fn)
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data), nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("read prompt file %q: %w", filepath.Join(dir, base), lastErr)
}

// RendersRules reports whether the template for mode puts the {{.Rules}} block
// into the prompt at all. The cross-block dedupe depends on it: the project
// docs preamble is dropped from {{.Instructions}} only because the rules block
// carries it, and an operator's own template under prompts.dir is free to
// render one block and not the other. A template that cannot be read falls
// back to the built-in one, which does render the block. The field is looked
// for by name, so {{if .Rules}} and {{ .Rules }} count too: answering yes when
// in doubt keeps the file in one block rather than in none.
func RendersRules(mode string, variants []string, promptsDir, agentFile, planFile, docsFile, askFile string) bool {
	src, err := loadSource(mode, variants, promptsDir, agentFile, planFile, docsFile, askFile)
	if err != nil {
		return true
	}
	return strings.Contains(src, ".Rules")
}

// RendersVolatile reports whether the template for mode prints something that
// changes between the steps of one turn: the wall clock, or the todo checklist
// a foxxycode_todo_* call rewrites. The built-in templates print neither, and the
// ReAct runner can then render the system message once and freeze it. A
// template under prompts.dir that prints either one keeps the old behaviour -
// re-rendered before every LLM call - because its own conditionals around those
// fields have to keep matching the state. It pays for that with the provider's
// prompt cache, which is why the built-in templates stopped doing it.
//
// A source that cannot be read counts as volatile: the render fallback
// (fallbackPrompt) carries a clock of its own.
func RendersVolatile(mode string, variants []string, promptsDir, agentFile, planFile, docsFile, askFile string) bool {
	src, err := loadSource(mode, variants, promptsDir, agentFile, planFile, docsFile, askFile)
	if err != nil {
		return true
	}
	return strings.Contains(src, ".UTCNow") || strings.Contains(src, ".TodoList")
}

func fallbackPrompt(mode, cwd string) string {
	return fmt.Sprintf(
		"You are an AI coding assistant in %s mode.\nWorking directory: %s\n\n## Current UTC time\n\n%s\n",
		mode, cwd, time.Now().UTC().Format(time.RFC3339))
}
