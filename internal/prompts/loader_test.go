package prompts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/prompts"
)

// Deterministic clock value for built-in template tests (RFC3339 UTC).
const fixtureUTC = "2038-01-19T03:14:07Z"

const (
	defaultAgentTplFile = "agent.md"
	defaultPlanTplFile  = "plan.md"
	defaultDocsTplFile  = "docs.md"
)

func TestRenderAgentPrompt(t *testing.T) {
	result, err := prompts.Render("agent", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/home/user/project",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("Render agent: %v", err)
	}
	if !strings.Contains(result, "/home/user/project") {
		t.Error("agent prompt should contain CWD")
	}
	if !strings.Contains(result, "Mode: Agent") {
		t.Error("agent prompt should mention Mode: Agent")
	}
	if strings.Contains(result, "## Available tools") {
		t.Error("tools section should be omitted when .Tools is empty")
	}
	if strings.Contains(result, "### Current todo checklist") {
		t.Error("todo checklist section should be omitted when .TodoList is empty")
	}
	if !strings.Contains(result, "## Current UTC time") || !strings.Contains(result, fixtureUTC) {
		t.Error("agent prompt should end with Current UTC time section")
	}
}

func TestRenderPlanPrompt(t *testing.T) {
	result, err := prompts.Render("plan", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/tmp/workspace",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("Render plan: %v", err)
	}
	if !strings.Contains(result, "/tmp/workspace") {
		t.Error("plan prompt should contain CWD")
	}
	if !strings.Contains(result, "Mode: Plan") {
		t.Error("plan prompt should mention Mode: Plan")
	}
	if !strings.Contains(result, "plan_write") {
		t.Error("plan prompt should mention plan_write")
	}
	if !strings.Contains(result, "plan_read") {
		t.Error("plan prompt should mention plan_read")
	}
	if !strings.Contains(result, "websearch") {
		t.Error("plan prompt should mention websearch for external research")
	}
	if !strings.Contains(result, "run_command") {
		t.Error("plan prompt should mention run_command for shell inspection")
	}
	if !strings.Contains(result, "MCP") {
		t.Error("plan prompt should mention MCP servers")
	}
	if !strings.Contains(result, "## Current UTC time") || !strings.Contains(result, fixtureUTC) {
		t.Error("plan prompt should end with Current UTC time section")
	}
}

func TestRenderDocsPrompt(t *testing.T) {
	result, err := prompts.Render("docs", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/tmp/docs-workspace",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("Render docs: %v", err)
	}
	if !strings.Contains(result, "/tmp/docs-workspace") {
		t.Error("docs prompt should contain CWD")
	}
	if !strings.Contains(result, "Mode: Docs") {
		t.Error("docs prompt should mention Mode: Docs")
	}
	if !strings.Contains(result, "docs_write") {
		t.Error("docs prompt should mention docs_write")
	}
	if !strings.Contains(result, "docs_edit") {
		t.Error("docs prompt should mention docs_edit")
	}
	if strings.Contains(result, "Save design plans with **`plan_write`**") {
		t.Error("docs prompt should not instruct plan_write usage")
	}
}

func TestRenderWithSkillsToolsMemory(t *testing.T) {
	result, err := prompts.Render("agent", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/project",
		Skills: "## Active Skills\n\nstub",
		Tools:  "- `read`: read",
		Memory: "User prefers pytest.",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"stub", "`read`", "pytest", "Session memory"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in result", want)
		}
	}
}

func TestRenderEmptyOptionalSections(t *testing.T) {
	result, err := prompts.Render("agent", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/project",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(result, "## Active Skills") {
		t.Error("should not emit skills heading when Skills data is empty")
	}
	if strings.Contains(result, "## Available tools") {
		t.Error("should not emit tools section when Tools data is empty")
	}
	if strings.Contains(result, "Session memory") {
		t.Error("should not emit memory section when Memory is empty")
	}
	if strings.Contains(result, "### Current todo checklist") {
		t.Error("should not emit todo checklist heading when TodoList is empty")
	}
}

func TestRenderTodoListWhenNonempty(t *testing.T) {
	todoMd := "- [ ] alpha\n- [x] beta"
	a, err := prompts.Render("agent", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p", TodoList: todoMd, UTCNow: fixtureUTC})
	if err != nil {
		t.Fatalf("Render agent: %v", err)
	}
	if !strings.Contains(a, "### Current todo checklist") || !strings.Contains(a, "- [ ] alpha") || !strings.Contains(a, "- [x] beta") {
		t.Errorf("expected injected todo markdown in agent prompt, got excerpt: %.200s", a)
	}

	p, err := prompts.Render("plan", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p", TodoList: todoMd, UTCNow: fixtureUTC})
	if err != nil {
		t.Fatalf("Render plan: %v", err)
	}
	if strings.Contains(p, "### Current todo checklist") {
		t.Error("plan prompt should not include session todo checklist section")
	}
}

func TestRenderUsesCustomTemplateFilenames(t *testing.T) {
	customAgent := "my-agent.tpl"
	body := "Hello {{.CWD}}\n"
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, customAgent), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := prompts.Render("agent", tmp, customAgent, "ignored-plan.tpl", "ignored-docs.tpl", prompts.TemplateData{CWD: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello /x" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderCustomPromptDir(t *testing.T) {
	customContent := "Custom for {{.CWD}}.\nSkills: {{.Skills}}\n"
	tmp := t.TempDir()
	path := filepath.Join(tmp, "agent.md")
	if err := os.WriteFile(path, []byte(customContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := prompts.Render("agent", tmp, defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/my/project",
		Skills: "S1",
	})
	if err != nil {
		t.Fatalf("Render custom dir: %v", err)
	}
	if !strings.Contains(result, "Custom for /my/project") || !strings.Contains(result, "S1") {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRenderCustomDirMissingAgentFile(t *testing.T) {
	tmp := t.TempDir()
	_, err := prompts.Render("agent", tmp, defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD: "/project",
	})
	if err == nil {
		t.Error("expected error when agent.md is missing in prompts dir")
	}
}

func TestRenderUnknownModeFallsBackToAgent(t *testing.T) {
	agent, _ := prompts.Render("agent", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p", UTCNow: fixtureUTC})
	unknown, err := prompts.Render("unknown_mode", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p", UTCNow: fixtureUTC})
	if err != nil {
		t.Fatalf("Render unknown mode: %v", err)
	}
	if agent != unknown {
		t.Error("unknown mode should use agent prompt file semantics")
	}
}

func TestDefaultSource(t *testing.T) {
	agentSrc := prompts.DefaultSource("agent")
	if agentSrc == "" {
		t.Error("agent source should not be empty")
	}
	if !strings.Contains(agentSrc, "{{.CWD}}") {
		t.Error("agent source should contain {{.CWD}} template variable")
	}
	if !strings.Contains(agentSrc, "{{.Skills}}") {
		t.Error("agent source should contain {{.Skills}}")
	}
	if !strings.Contains(agentSrc, "{{if .TodoList}}") {
		t.Error("agent source should conditionalize TodoList injection")
	}
	if !strings.Contains(agentSrc, "{{.UTCNow}}") {
		t.Error("agent source should expose UTCNow for clock grounding")
	}

	planSrc := prompts.DefaultSource("plan")
	if planSrc == "" {
		t.Error("plan source should not be empty")
	}
	if planSrc == agentSrc {
		t.Error("plan and agent sources should differ")
	}

	docsSrc := prompts.DefaultSource("docs")
	if docsSrc == "" {
		t.Error("docs source should not be empty")
	}
	if docsSrc == agentSrc || docsSrc == planSrc {
		t.Error("docs source should differ from agent and plan")
	}
}

func TestRenderForFamilyDirVariantSelected(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "agent.md"), []byte("BASE {{.CWD}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "agent.anthropic.md"), []byte("ANTHROPIC {{.CWD}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := prompts.RenderForFamily("agent", "anthropic", tmp, defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ANTHROPIC /p" {
		t.Fatalf("expected family variant, got %q", got)
	}
}

func TestRenderForFamilyDirFallsBackToBase(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "agent.md"), []byte("BASE {{.CWD}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No agent.gemini.md present: must fall back to agent.md.
	got, err := prompts.RenderForFamily("agent", "gemini", tmp, defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "BASE /p" {
		t.Fatalf("expected base fallback, got %q", got)
	}
}

func TestRenderForFamilyEmbeddedFallsBackToBase(t *testing.T) {
	// A family with no embedded variant must render the same as the base agent prompt.
	base, err := prompts.Render("agent", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p", UTCNow: fixtureUTC})
	if err != nil {
		t.Fatal(err)
	}
	fam, err := prompts.RenderForFamily("agent", "no-such-family", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p", UTCNow: fixtureUTC})
	if err != nil {
		t.Fatal(err)
	}
	if fam != base {
		t.Error("unknown family should fall back to the base embedded agent prompt")
	}
}

func TestRenderForVariantsPrefersMostSpecific(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "agent.md"), "BASE {{.CWD}}")
	mustWrite(t, filepath.Join(tmp, "agent.anthropic.md"), "FAMILY {{.CWD}}")
	mustWrite(t, filepath.Join(tmp, "agent.anthropic-claude-x.md"), "MODEL {{.CWD}}")

	// Model slug is first in the list, so it wins over the family variant.
	got, err := prompts.RenderForVariants("agent", []string{"anthropic-claude-x", "anthropic"}, tmp, defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "MODEL /p" {
		t.Fatalf("expected per-model variant, got %q", got)
	}
}

func TestRenderForVariantsFallsThroughToFamilyThenBase(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "agent.md"), "BASE {{.CWD}}")
	mustWrite(t, filepath.Join(tmp, "agent.anthropic.md"), "FAMILY {{.CWD}}")

	// No per-model file: falls through to the family variant.
	got, err := prompts.RenderForVariants("agent", []string{"anthropic-claude-x", "anthropic"}, tmp, defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "FAMILY /p" {
		t.Fatalf("expected family fallback, got %q", got)
	}

	// Neither model nor family file: falls through to base.
	got2, err := prompts.RenderForVariants("agent", []string{"gemini-2", "gemini"}, tmp, defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "BASE /p" {
		t.Fatalf("expected base fallback, got %q", got2)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedFamilyVariantsRender(t *testing.T) {
	families := []string{"anthropic", "openai", "gemini", "gpt-oss", "qwen", "gemma", "neuraldeep"}
	base, err := prompts.Render("agent", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p", UTCNow: fixtureUTC})
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range families {
		t.Run(fam, func(t *testing.T) {
			got, err := prompts.RenderForFamily("agent", fam, "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
				CWD:    "/home/user/project",
				UTCNow: fixtureUTC,
			})
			if err != nil {
				t.Fatalf("render family %q: %v", fam, err)
			}
			if got == base {
				t.Errorf("family %q should differ from the base agent prompt", fam)
			}
			if !strings.Contains(got, "Model-family notes") {
				t.Errorf("family %q prompt should contain a model-family notes section", fam)
			}
			// Shared template variables must survive in every family variant.
			if !strings.Contains(got, "/home/user/project") || !strings.Contains(got, fixtureUTC) {
				t.Errorf("family %q prompt dropped shared template sections", fam)
			}
		})
	}
}

func TestEmbeddedOpenAIAgentPromptOptimizedForOpenAIAPI(t *testing.T) {
	got, err := prompts.RenderForFamily("agent", "openai", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/home/user/project",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("render openai agent prompt: %v", err)
	}
	for _, want := range []string{
		"OpenAI API development prompt",
		"Responses API",
		"Chat Completions",
		"reasoning_effort",
		"phase",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("OpenAI agent prompt should contain %q", want)
		}
	}
}

func TestEmbeddedOpenAIPlanVariantRender(t *testing.T) {
	base, err := prompts.Render("plan", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/home/user/project",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("render base plan prompt: %v", err)
	}
	got, err := prompts.RenderForFamily("plan", "openai", "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
		CWD:    "/home/user/project",
		UTCNow: fixtureUTC,
	})
	if err != nil {
		t.Fatalf("render openai plan prompt: %v", err)
	}
	if got == base {
		t.Fatal("OpenAI plan variant should differ from the base plan prompt")
	}
	for _, want := range []string{
		"Mode: Plan",
		"OpenAI API planning prompt",
		"Responses API",
		"reasoning_effort",
		"plan_write",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("OpenAI plan prompt should contain %q", want)
		}
	}
}

func TestRenderWithFallbackNoPanic(t *testing.T) {
	result := prompts.RenderWithFallback("agent", "/nonexistent/prompt-dir", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/p"})
	if result == "" {
		t.Error("RenderWithFallback should return non-empty string even on error")
	}
}
