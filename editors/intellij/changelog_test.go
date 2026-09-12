// Guards for CHANGELOG.md, the source of the "Change Notes" tab the IDE shows in
// Settings | Plugins.
//
// The Gradle build renders this file into plugin.xml, but Gradle needs JDK 17 plus
// Go and Node and therefore only runs in the intellij-plugin.yaml workflow. These
// tests run with plain `go test ./...` on every PR and catch what would otherwise
// reach a release unnoticed: a section the renderer cannot parse, versions drifting
// out of order, an empty entry, an entry written in English when the whole point of
// the file is that a user reads it in Russian, or merge-conflict markers left in the
// file (0.2.46 shipped with a literal `=======` paragraph in its Change Notes tab).
//
// The newest section is written as `## Unreleased — YYYY-MM-DD`: the version a merge
// will produce cannot be known while the PR is open, so the build stamps it with the
// version actually being built. See .claude/rules/release-changelog.md.
package intellij

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const (
	changelogPath   = "CHANGELOG.md"
	gradleBuildPath = "build.gradle.kts"
)

// changelogHeading and changelogUnreleased mirror the two regexes in build.gradle.kts.
// They must agree, or the build silently renders fewer sections than the file holds.
var (
	changelogHeading    = regexp.MustCompile(`^##\s+(\d+\.\d+\.\d+)\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$`)
	changelogUnreleased = regexp.MustCompile(`^##\s+Unreleased\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$`)
)

type changelogEntry struct {
	version    string // empty for the not-yet-tagged section; the build fills it in
	date       string
	body       string
	line       int
	unreleased bool
}

// label names an entry in test failures, where an unreleased section has no version.
func (e changelogEntry) label() string {
	if e.unreleased {
		return "Unreleased"
	}
	return e.version
}

func parseChangelog(t *testing.T) []changelogEntry {
	t.Helper()
	data, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("read %s: %v", changelogPath, err)
	}
	var entries []changelogEntry
	var current *changelogEntry
	var body strings.Builder
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimRight(line, "\r")
		var next *changelogEntry
		if m := changelogHeading.FindStringSubmatch(trimmed); m != nil {
			next = &changelogEntry{version: m[1], date: m[2], line: i + 1}
		} else if m := changelogUnreleased.FindStringSubmatch(trimmed); m != nil {
			next = &changelogEntry{date: m[1], line: i + 1, unreleased: true}
		}
		if next != nil {
			if current != nil {
				current.body = body.String()
				entries = append(entries, *current)
			}
			body.Reset()
			current = next
			continue
		}
		if current != nil {
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	if current != nil {
		current.body = body.String()
		entries = append(entries, *current)
	}
	if len(entries) == 0 {
		t.Fatalf("%s has no `## X.Y.Z — YYYY-MM-DD` sections; the plugin would ship without change notes", changelogPath)
	}
	return entries
}

// TestChangelogSectionsAreOrdered pins that versions descend strictly. A duplicate or
// an out-of-order entry means the renderer would show the wrong release on top. The
// unreleased section carries no version yet, so it is skipped here and pinned to the
// top of the file by TestChangelogUnreleasedSectionIsSingleAndFirst instead.
func TestChangelogSectionsAreOrdered(t *testing.T) {
	var released []changelogEntry
	for _, e := range parseChangelog(t) {
		if !e.unreleased {
			released = append(released, e)
		}
	}
	for i := 1; i < len(released); i++ {
		prev, cur := released[i-1], released[i]
		if compareSemVer(t, cur.version, prev.version) >= 0 {
			t.Errorf("%s:%d: version %s is not below %s; sections must run newest first",
				changelogPath, cur.line, cur.version, prev.version)
		}
	}
}

// TestChangelogUnreleasedSectionIsSingleAndFirst keeps the deferred-version convention
// usable. The build stamps the unreleased section with the version it is building, so a
// second one would ship two sections claiming the same release, and one sitting below a
// numbered section would be stamped with a version older than the notes above it.
func TestChangelogUnreleasedSectionIsSingleAndFirst(t *testing.T) {
	entries := parseChangelog(t)
	var unreleased []changelogEntry
	for _, e := range entries {
		if e.unreleased {
			unreleased = append(unreleased, e)
		}
	}
	if len(unreleased) == 0 {
		return
	}
	if first := unreleased[0]; first.line != entries[0].line {
		t.Errorf("%s:%d: the `## Unreleased` section must be the newest one, above %s",
			changelogPath, first.line, entries[0].label())
	}
	for _, extra := range unreleased[1:] {
		t.Errorf("%s:%d: a second `## Unreleased` section; merge it into the one at line %d, or stamp "+
			"the released one with the tag it actually shipped under",
			changelogPath, extra.line, unreleased[0].line)
	}
}

// TestChangelogHasNoConflictMarkers catches the failure this file exists to prevent but
// missed once: 0.2.46 shipped with `<<<<<<< HEAD` / `=======` / `>>>>>>> origin/main`
// committed into the changelog, and the renderer turned the `=======` into a paragraph
// of the newest section in the IDE's Change Notes tab. Nothing else flagged it, because
// a conflicted file still parses, still descends, and still reads as Russian.
func TestChangelogHasNoConflictMarkers(t *testing.T) {
	data, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("read %s: %v", changelogPath, err)
	}
	markers := []string{"<<<<<<<", "=======", ">>>>>>>"}
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimRight(line, "\r")
		for _, marker := range markers {
			if strings.HasPrefix(trimmed, marker) {
				t.Errorf("%s:%d: unresolved merge-conflict marker %q; it would render into the "+
					"Change Notes tab users read", changelogPath, i+1, trimmed)
			}
		}
	}
}

// TestChangelogEntriesAreWrittenForHumans is the rule this file exists for: every entry
// carries real prose in Russian, not a placeholder and not a commit list left in English.
func TestChangelogEntriesAreWrittenForHumans(t *testing.T) {
	for _, e := range parseChangelog(t) {
		text := strings.TrimSpace(e.body)
		if len(text) < 40 {
			t.Errorf("%s:%d: entry %s is empty or a stub; describe what changed for the user",
				changelogPath, e.line, e.label())
			continue
		}
		if !hasCyrillic(text) {
			t.Errorf("%s:%d: entry %s has no Russian text; the Change Notes tab is written in Russian",
				changelogPath, e.line, e.label())
		}
		for _, line := range strings.Split(text, "\n") {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, "commit") {
				t.Errorf("%s:%d: entry %s reads like a commit list; describe the change, not the commits",
					changelogPath, e.line, e.label())
				break
			}
		}
	}
}

// TestChangelogHeadingRegexMatchesGradle keeps this file's parser and the renderer in
// build.gradle.kts from drifting apart: a heading one accepts and the other does not
// would drop a whole release from the IDE tab with no error anywhere.
func TestChangelogHeadingRegexMatchesGradle(t *testing.T) {
	data, err := os.ReadFile(gradleBuildPath)
	if err != nil {
		t.Fatalf("read %s: %v", gradleBuildPath, err)
	}
	for _, want := range []string{
		`^##\s+(\d+\.\d+\.\d+)\s*[—-]\s*(.+)$`,
		`^##\s+Unreleased\s*[—-]\s*(.+)$`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("%s no longer contains the expected heading regex %q; update changelogHeading "+
				"and changelogUnreleased here to match", gradleBuildPath, want)
		}
	}
	// The unreleased section only carries the right number if the build substitutes the
	// version it is building; without this the notes would ship headed "Unreleased".
	if !strings.Contains(string(data), "stampUnreleased") {
		t.Errorf("%s does not stamp the unreleased section with the version being built; "+
			"the Change Notes tab would show a heading with no version", gradleBuildPath)
	}
	if !strings.Contains(string(data), "CHANGELOG.md") {
		t.Errorf("%s does not read CHANGELOG.md; change notes would be dropped from the plugin", gradleBuildPath)
	}
	if !strings.Contains(string(data), "changeNotes.set(") {
		t.Errorf("%s does not set changeNotes; the IDE Change Notes tab would stay empty", gradleBuildPath)
	}
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func compareSemVer(t *testing.T, a, b string) int {
	t.Helper()
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		na, err := strconv.Atoi(pa[i])
		if err != nil {
			t.Fatalf("bad version %q: %v", a, err)
		}
		nb, err := strconv.Atoi(pb[i])
		if err != nil {
			t.Fatalf("bad version %q: %v", b, err)
		}
		if na != nb {
			return na - nb
		}
	}
	return 0
}
