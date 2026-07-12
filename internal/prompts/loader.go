// Package prompts manages system prompt templates for each agent mode.
// Templates are markdown files embedded into the binary or loaded from a directory
// configured under YAML key prompts (config.Prompts in internal/config/prompts.go). They use Go text/template for variable substitution.
//
// Template variables available in .md files:
//
//	{{.CWD}}      - session working directory
//	{{.Tools}}    - readable list of tools available in the current mode (markdown)
//	{{.Skills}}   - active skills markdown (slash catalog and bodies), built by the agent
//	{{.Memory}}   - session agent memory notes (may be empty)
//	{{.TodoList}} - current session todo checklist rendered as markdown (empty until plan tools populate state)
//	{{.UTCNow}}   - current date and time in UTC (RFC3339), set each time the system prompt renders
//
// Use {{if .Skills}}...{{end}} (and similarly for .Tools, .Memory, .TodoList) when sections should be omitted when empty.
// The ReAct runner refreshes the rendered system prompt before each LLM call while handling one session prompt.
package prompts

import (
	"embed"
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
)

// TemplateData holds values injected into prompt templates.
type TemplateData struct {
	// CWD is the session working directory.
	CWD string

	// Skills is preformatted markdown for slash skills (may be empty).
	Skills string

	// Rules is preformatted markdown for active project rules (may be empty).
	Rules string

	// Tools is a human-readable markdown list of tools for the current mode (may be empty).
	Tools string

	// Memory is session-scoped notes injected into the prompt (may be empty).
	Memory string

	// TodoList is the current session checklist as markdown lines (may be empty).
	TodoList string

	// PlanContext is design plan text injected when the user runs a saved plan (may be empty).
	PlanContext string

	// DiscardedPlans is plan-mode guidance when the user discarded design plan slugs (may be empty).
	DiscardedPlans string

	// Instructions is the concatenated content of project instruction files (AGENTS.md etc.), may be empty.
	Instructions string

	// UTCNow is the wall-clock instant in RFC3339 (UTC) at render time for model grounding.
	UTCNow string
}

// Embedded default prompt template files. The glob covers the base per-mode
// templates (agent.md, plan.md, docs.md) and any per-family variants
// (for example agent.anthropic.md) selected by Family.
//
//go:embed *.md
var embeddedPrompts embed.FS

// Render renders the prompt template for the given mode with the provided data.
// promptsDir must be empty to use built-in templates; otherwise it is a directory that
// contains the files named agentFile, planFile, and docsFile (for example agent.md, plan.md, docs.md).
// mode must be "agent", "plan", or "docs". Unknown modes use the agent template file.
func Render(mode, promptsDir, agentFile, planFile, docsFile string, data TemplateData) (string, error) {
	return RenderForFamily(mode, "", promptsDir, agentFile, planFile, docsFile, data)
}

// RenderForFamily is Render with a provider family. When family is non-empty it selects the
// per-family template variant (for example agent.anthropic.md), falling back to the base
// per-mode template when the variant does not exist. family "" behaves exactly like Render.
func RenderForFamily(mode, family, promptsDir, agentFile, planFile, docsFile string, data TemplateData) (string, error) {
	return RenderForVariants(mode, familyVariants(family), promptsDir, agentFile, planFile, docsFile, data)
}

// RenderForVariants is Render with an ordered list of variant keys, most-specific first
// (for example a per-model slug then a provider family). The first key whose template file
// (agent.<key>.md) exists is used; otherwise the base per-mode template is rendered. A nil
// or empty list behaves exactly like Render.
func RenderForVariants(mode string, variants []string, promptsDir, agentFile, planFile, docsFile string, data TemplateData) (string, error) {
	src, err := loadSource(mode, variants, promptsDir, agentFile, planFile, docsFile)
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
func RenderWithFallback(mode, promptsDir, agentFile, planFile, docsFile string, data TemplateData) string {
	return RenderWithFallbackForVariants(mode, nil, promptsDir, agentFile, planFile, docsFile, data)
}

// RenderWithFallbackForFamily renders the per-family prompt and returns a safe default on error.
func RenderWithFallbackForFamily(mode, family, promptsDir, agentFile, planFile, docsFile string, data TemplateData) string {
	return RenderWithFallbackForVariants(mode, familyVariants(family), promptsDir, agentFile, planFile, docsFile, data)
}

// RenderWithFallbackForVariants renders the most-specific available variant and returns a
// safe default on error.
func RenderWithFallbackForVariants(mode string, variants []string, promptsDir, agentFile, planFile, docsFile string, data TemplateData) string {
	s, err := RenderForVariants(mode, variants, promptsDir, agentFile, planFile, docsFile, data)
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

// DefaultSource returns the built-in template source for a mode.
// Useful for displaying to the user so they can customize it.
func DefaultSource(mode string) string {
	return defaultSourceForVariants(mode, nil)
}

// defaultSourceForVariants returns the embedded template for a mode, preferring the first
// embedded variant that exists and falling back to the base per-mode file.
func defaultSourceForVariants(mode string, variants []string) string {
	base := fileNameForMode(mode)
	for _, v := range variants {
		if fam := familyFileName(base, v); fam != base {
			if b, err := embeddedPrompts.ReadFile(fam); err == nil {
				return string(b)
			}
		}
	}
	b, err := embeddedPrompts.ReadFile(base)
	if err != nil {
		return ""
	}
	return string(b)
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
	default:
		return fileAgent
	}
}

// loadSource returns the template source: files from promptsDir when set, built-in otherwise.
// variants is an ordered list of keys tried most-specific first (agent.<key>.md); the base
// file is always the final fallback (embedded for built-ins, on-disk for promptsDir).
func loadSource(mode string, variants []string, promptsDir, agentFile, planFile, docsFile string) (string, error) {
	dir := strings.TrimSpace(promptsDir)
	if dir == "" {
		return defaultSourceForVariants(mode, variants), nil
	}

	base := strings.TrimSpace(agentFile)
	switch mode {
	case "plan":
		base = strings.TrimSpace(planFile)
	case "docs":
		base = strings.TrimSpace(docsFile)
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

func fallbackPrompt(mode, cwd string) string {
	return fmt.Sprintf(
		"You are an AI coding assistant in %s mode.\nWorking directory: %s\n\n## Current UTC time\n\n%s\n",
		mode, cwd, time.Now().UTC().Format(time.RFC3339))
}
