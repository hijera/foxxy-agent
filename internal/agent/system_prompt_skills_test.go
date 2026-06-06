package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/skills"
)

// TestBuildSkillsPromptMarkdown_invokedCatalogSkillBodyInjected verifies that when a user
// explicitly invokes a slash command (/find-skills) whose skill has no globs and
// alwaysApply:false, the skill body is injected in the ephemeral section.
//
// Regression: the old filteredInvoke check compared against activeGlobCanon which always
// included no-glob skills, so the body was never emitted in either section or ephemeral.
func TestBuildSkillsPromptMarkdown_invokedCatalogSkillBodyInjected(t *testing.T) {
	const body = "UNIQUE_FIND_SKILLS_BODY_TOKEN"
	sk := &skills.Skill{
		Name:        "SKILL",
		FilePath:    filepath.Join("skills", "find-skills", "SKILL.md"),
		Description: "find skills",
		AlwaysApply: false,
		Content:     body,
	}

	allLoaded := []*skills.Skill{sk}
	// FilterForContext with no-globs and no contextFiles includes the skill (always-apply fallback).
	active := skills.FilterForContext(allLoaded, nil)

	result := buildSkillsPromptMarkdown(allLoaded, active, "/find-skills search pdf")

	if !strings.Contains(result, body) {
		t.Fatalf("expected skill body %q to be injected when /find-skills is invoked; got:\n%s", body, result)
	}
}

// TestBuildSkillsPromptMarkdown_alwaysApplySkillBodyNotDuplicated ensures a skill with
// alwaysApply:true whose body appears in the active section is NOT duplicated in ephemeral
// when the user also types /skillname.
func TestBuildSkillsPromptMarkdown_alwaysApplySkillBodyNotDuplicated(t *testing.T) {
	const body = "ALWAYS_SKILL_BODY"
	sk := &skills.Skill{
		Name:        "SKILL",
		FilePath:    filepath.Join("skills", "my-tool", "SKILL.md"),
		Description: "my tool",
		AlwaysApply: true,
		Content:     body,
	}

	allLoaded := []*skills.Skill{sk}
	active := skills.FilterForContext(allLoaded, nil)

	result := buildSkillsPromptMarkdown(allLoaded, active, "/my-tool do something")

	count := strings.Count(result, body)
	if count > 1 {
		t.Fatalf("skill body should not be duplicated; found %d occurrences in:\n%s", count, result)
	}
}
