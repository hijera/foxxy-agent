package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/skills"
)

// TestAugmentUserMessageWithInvokedSkills_bodyInjected verifies that when a user
// explicitly invokes a slash command (/find-skills), its body is prepended to the
// user message sent to the LLM. The chat display (stored Content) remains unchanged.
//
// Regression: previously the body was never emitted because filteredInvoke compared
// against activeGlobCanon which always included no-glob skills, preventing injection
// in both the system-prompt ephemeral section and (by extension) the user message.
func TestAugmentUserMessageWithInvokedSkills_bodyInjected(t *testing.T) {
	const body = "UNIQUE_FIND_SKILLS_BODY_TOKEN"
	sk := &skills.Skill{
		Name:        "SKILL",
		FilePath:    filepath.Join("skills", "find-skills", "SKILL.md"),
		Description: "find skills",
		AlwaysApply: false,
		Content:     body,
	}

	userText := "/find-skills search pdf"
	result := augmentUserMessageWithInvokedSkills(userText, []*skills.Skill{sk})

	if !strings.Contains(result, body) {
		t.Fatalf("expected skill body %q to be prepended to user message; got:\n%s", body, result)
	}
	if !strings.Contains(result, userText) {
		t.Fatalf("expected original user text %q to be preserved in result; got:\n%s", userText, result)
	}
	// Skill body must come BEFORE the original user text.
	if strings.Index(result, body) > strings.Index(result, userText) {
		t.Fatalf("skill body should appear before user text in augmented message")
	}
}

// TestAugmentUserMessageWithInvokedSkills_noSkillMatch returns userText unchanged when
// the invoked name does not match any loaded skill.
func TestAugmentUserMessageWithInvokedSkills_noSkillMatch(t *testing.T) {
	sk := &skills.Skill{
		Name:     "SKILL",
		FilePath: filepath.Join("skills", "other", "SKILL.md"),
		Content:  "other body",
	}
	userText := "/find-skills pdf"
	result := augmentUserMessageWithInvokedSkills(userText, []*skills.Skill{sk})
	if result != userText {
		t.Fatalf("expected unchanged userText when no skill matches; got:\n%s", result)
	}
}

// TestAugmentUserMessageWithInvokedSkills_noSlashCommand returns userText unchanged when
// the message contains no slash command.
func TestAugmentUserMessageWithInvokedSkills_noSlashCommand(t *testing.T) {
	sk := &skills.Skill{
		Name:     "SKILL",
		FilePath: filepath.Join("skills", "find-skills", "SKILL.md"),
		Content:  "body",
	}
	userText := "поищи что-нибудь"
	result := augmentUserMessageWithInvokedSkills(userText, []*skills.Skill{sk})
	if result != userText {
		t.Fatalf("expected unchanged userText when no slash command; got:\n%s", result)
	}
}

// TestBuildSkillsPromptMarkdown_catalogSkillBodyNotInSystemPrompt verifies that slash
// command skill bodies are NOT injected into the system prompt; only the catalog listing
// appears there. Bodies travel via user message augmentation instead.
func TestBuildSkillsPromptMarkdown_catalogSkillBodyNotInSystemPrompt(t *testing.T) {
	const body = "UNIQUE_FIND_SKILLS_BODY_TOKEN"
	sk := &skills.Skill{
		Name:        "SKILL",
		FilePath:    filepath.Join("skills", "find-skills", "SKILL.md"),
		Description: "find skills",
		AlwaysApply: false,
		Content:     body,
	}

	allLoaded := []*skills.Skill{sk}
	active := skills.FilterForContext(allLoaded, nil)

	result := buildSkillsPromptMarkdown(allLoaded, active)

	if strings.Contains(result, body) {
		t.Fatalf("slash command skill body should NOT be in system prompt; got:\n%s", result)
	}
	if !strings.Contains(result, "find-skills") {
		t.Fatalf("skill name should appear in the slash catalog; got:\n%s", result)
	}
}

// TestBuildSkillsPromptMarkdown_alwaysApplyNonCatalogBodyInSystemPrompt verifies that a
// skill with alwaysApply:true that is NOT in the catalog still has its body in the system prompt.
func TestBuildSkillsPromptMarkdown_alwaysApplyNonCatalogBodyInSystemPrompt(t *testing.T) {
	const body = "ALWAYS_APPLY_NON_CATALOG_BODY"
	// This skill uses a path that doesn't become a slash command name in the catalog.
	sk := &skills.Skill{
		Name:        "my-always-rule",
		FilePath:    filepath.Join("rules", "my-always-rule.md"),
		AlwaysApply: true,
		Content:     body,
	}

	allLoaded := []*skills.Skill{sk}
	active := skills.FilterForContext(allLoaded, nil)

	result := buildSkillsPromptMarkdown(allLoaded, active)

	if !strings.Contains(result, body) {
		t.Fatalf("always-apply non-catalog skill body should be in system prompt; got:\n%s", result)
	}
}
