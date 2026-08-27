package prompts_test

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/prompts"
)

// Gateways that sit in front of the model attribute a request to a client by
// matching the opening of its system prompt, so these tests pin two things:
// the marker exists, and it stays close enough to the start to be seen.
const gatewayWindow = 220

func headWindow(s string) string {
	if len(s) > gatewayWindow {
		return s[:gatewayWindow]
	}
	return s
}

func assertIdentified(t *testing.T, prompt, what string) {
	t.Helper()
	head := strings.ToLower(headWindow(prompt))
	if !strings.Contains(head, "you are foxxycode") {
		t.Errorf("%s does not identify FoxxyCode within the first %d characters:\n%s",
			what, gatewayWindow, headWindow(prompt))
	}
}

func TestWithIdentityPrependsMarkerToForeignPrompt(t *testing.T) {
	custom := "You are a helpful assistant. Follow the house style."
	got := prompts.WithIdentity(custom)

	assertIdentified(t, got, "custom prompt")
	if !strings.HasSuffix(got, custom) {
		t.Errorf("original prompt must be preserved verbatim after the identity line, got:\n%s", got)
	}
}

func TestWithIdentityDoesNotDuplicateExistingMarker(t *testing.T) {
	already := prompts.Identity + "\nWorking directory: /tmp"
	got := prompts.WithIdentity(already)

	if got != already {
		t.Errorf("a prompt that already identifies FoxxyCode must be returned untouched, got:\n%s", got)
	}
	if strings.Count(strings.ToLower(got), "you are foxxycode") != 1 {
		t.Errorf("identity line was duplicated:\n%s", got)
	}
}

func TestWithIdentityDedupIsCaseInsensitive(t *testing.T) {
	already := "YOU ARE FOXXYCODE, an AI coding agent.\nRest of the prompt."
	if got := prompts.WithIdentity(already); got != already {
		t.Errorf("dedup must ignore case, got:\n%s", got)
	}
}

// A marker buried past the window is invisible to a gateway, so a prompt that
// only names FoxxyCode deep inside still has to get the line prepended.
func TestWithIdentityPrependsWhenMarkerIsPastTheWindow(t *testing.T) {
	buried := strings.Repeat("x", gatewayWindow+50) + " you are foxxycode"
	got := prompts.WithIdentity(buried)

	assertIdentified(t, got, "prompt naming FoxxyCode past the window")
}

func TestWithIdentityHandlesEmptyPrompt(t *testing.T) {
	if got := strings.TrimSpace(prompts.WithIdentity("")); got != prompts.Identity {
		t.Errorf("empty prompt should collapse to the identity line, got %q", got)
	}
	if got := strings.TrimSpace(prompts.WithIdentity("   \n\t ")); got != prompts.Identity {
		t.Errorf("blank prompt should collapse to the identity line, got %q", got)
	}
}

func TestIdentityConstantCarriesTheMarker(t *testing.T) {
	assertIdentified(t, prompts.Identity, "Identity constant")
}

// This fork does not brand the built-in templates: its system prompt opens with
// a language directive long enough to push a template's own opening past the
// window a gateway reads, so branding them too would print the identity twice.
// The line therefore always comes from WithIdentity, which the agent applies to
// the assembled prompt (see internal/agent/system_prompt.go).
func TestBuiltInTemplatesAreIdentifiedByWithIdentity(t *testing.T) {
	for _, mode := range []string{"agent", "plan"} {
		rendered, err := prompts.Render(mode, "", defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{
			CWD:    "/home/user/project",
			UTCNow: fixtureUTC,
		})
		if err != nil {
			t.Fatalf("Render %s: %v", mode, err)
		}
		identified := prompts.WithIdentity(rendered)
		assertIdentified(t, identified, "built-in "+mode+" template")
		if strings.Count(strings.ToLower(identified), "you are foxxycode") != 1 {
			t.Errorf("built-in %s template must carry the identity exactly once:%s%s", mode, "\n", identified)
		}
	}
}

// RenderWithFallback returns a generic stub when a custom template is broken;
// that stub has no name of its own and depends on WithIdentity.
func TestFallbackPromptIsIdentifiedByWithIdentity(t *testing.T) {
	fallback := prompts.RenderWithFallback("agent", "/nonexistent-prompts-dir",
		defaultAgentTplFile, defaultPlanTplFile, defaultDocsTplFile, prompts.TemplateData{CWD: "/tmp", UTCNow: fixtureUTC})

	assertIdentified(t, prompts.WithIdentity(fallback), "render fallback")
}
