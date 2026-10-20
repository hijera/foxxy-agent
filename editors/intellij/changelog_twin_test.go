// Guards for CHANGELOG.en.md, the English twin of CHANGELOG.md.
//
// The IDE reads only the Russian file. The twin exists for the project website, which shows
// the recent change notes in both languages, so it must follow the Russian file section by
// section: the same headings from the top down and the same entries in each section. It may
// stop earlier than the Russian file, because older releases were never translated; it may
// never run ahead of it or drift in between. See .claude/rules/release-changelog.md.
package intellij

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const changelogEnPath = "CHANGELOG.en.md"

var (
	// changelogEntryTitle matches the bold line that opens each entry of a section.
	changelogEntryTitle = regexp.MustCompile(`^\*\*.+\*\*\s*$`)
	changelogStub       = regexp.MustCompile(`(?i)\bTBD\b`)
	// Code and quotations may carry Russian on purpose (a UI label, a path), so the
	// language check ignores them.
	changelogFencedCode = regexp.MustCompile("(?s)```.*?```")
	changelogInlineCode = regexp.MustCompile("`[^`\n]*`")
	changelogQuoted     = regexp.MustCompile(`«[^»]*»|“[^”]*”|"[^"\n]*"`)
)

// TestEnglishChangelogFollowsTheRussianHeadings pins the twin to the Russian file: section i
// of the twin carries the same version (or Unreleased) and date as section i of CHANGELOG.md.
// A new Russian section on top without its translation, or an Unreleased renamed in one file
// only, shifts every pair below it and fails here.
func TestEnglishChangelogFollowsTheRussianHeadings(t *testing.T) {
	ru := parseChangelogFile(t, changelogPath)
	en := parseChangelogFile(t, changelogEnPath)
	if len(en) > len(ru) {
		t.Errorf("%s has %d sections but %s only %d; the twin cannot describe releases the IDE never showed",
			changelogEnPath, len(en), changelogPath, len(ru))
	}
	for i := 0; i < len(en) && i < len(ru); i++ {
		if en[i].label() != ru[i].label() || en[i].date != ru[i].date {
			t.Fatalf("%s:%d: section %d is %q (%s) but %s:%d has %q (%s); add, rename or date the "+
				"section in both files together",
				changelogEnPath, en[i].line, i+1, en[i].label(), en[i].date,
				changelogPath, ru[i].line, ru[i].label(), ru[i].date)
		}
	}
}

// TestEnglishChangelogHasTheSameEntries catches an entry added to a Russian section that already
// has a translation: the headings still pair up, only the number of entries tells them apart.
func TestEnglishChangelogHasTheSameEntries(t *testing.T) {
	ru := parseChangelogFile(t, changelogPath)
	en := parseChangelogFile(t, changelogEnPath)
	for i := 0; i < len(en) && i < len(ru); i++ {
		if got, want := countEntryTitles(en[i].body), countEntryTitles(ru[i].body); got != want {
			t.Errorf("%s:%d: section %s has %d `**Title.**` entries, %s:%d has %d; translate every entry",
				changelogEnPath, en[i].line, en[i].label(), got, changelogPath, ru[i].line, want)
		}
	}
}

// TestEnglishChangelogIsWrittenInEnglish keeps the twin a translation: prose, not a stub, and no
// Russian left outside code and quotations.
func TestEnglishChangelogIsWrittenInEnglish(t *testing.T) {
	for _, e := range parseChangelogFile(t, changelogEnPath) {
		text := strings.TrimSpace(e.body)
		if len(text) < 40 || changelogStub.MatchString(text) {
			t.Errorf("%s:%d: entry %s is empty or a stub; translate the Russian section",
				changelogEnPath, e.line, e.label())
			continue
		}
		prose := changelogFencedCode.ReplaceAllString(text, "")
		prose = changelogInlineCode.ReplaceAllString(prose, "")
		prose = changelogQuoted.ReplaceAllString(prose, "")
		if hasCyrillic(prose) {
			t.Errorf("%s:%d: entry %s still has Russian text outside code and quotes",
				changelogEnPath, e.line, e.label())
		}
	}
}

// TestEnglishChangelogHasNoConflictMarkers mirrors TestChangelogHasNoConflictMarkers: the website
// renders the twin, so a leftover marker would reach readers the same way.
func TestEnglishChangelogHasNoConflictMarkers(t *testing.T) {
	data, err := os.ReadFile(changelogEnPath)
	if err != nil {
		t.Fatalf("read %s: %v", changelogEnPath, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimRight(line, "\r")
		for _, marker := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
			if strings.HasPrefix(trimmed, marker) {
				t.Errorf("%s:%d: unresolved merge-conflict marker %q", changelogEnPath, i+1, trimmed)
			}
		}
	}
}

// TestEnglishChangelogIsNotShippedAsChangeNotes keeps the IDE on the Russian file: the plugin's
// Change Notes tab is product copy in Russian, and the twin is for the website only.
func TestEnglishChangelogIsNotShippedAsChangeNotes(t *testing.T) {
	data, err := os.ReadFile(gradleBuildPath)
	if err != nil {
		t.Fatalf("read %s: %v", gradleBuildPath, err)
	}
	if strings.Contains(string(data), changelogEnPath) {
		t.Errorf("%s reads %s; the Change Notes tab must keep rendering the Russian CHANGELOG.md",
			gradleBuildPath, changelogEnPath)
	}
}

func countEntryTitles(body string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if changelogEntryTitle.MatchString(strings.TrimRight(line, "\r")) {
			n++
		}
	}
	return n
}
