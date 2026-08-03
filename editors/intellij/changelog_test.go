// Guards for CHANGELOG.md, the source of the "Change Notes" tab the IDE shows in
// Settings | Plugins.
//
// The Gradle build renders this file into plugin.xml, but Gradle needs JDK 17 plus
// Go and Node and therefore only runs in the intellij-plugin.yaml workflow. These
// tests run with plain `go test ./...` on every PR and catch what would otherwise
// reach a release unnoticed: a section the renderer cannot parse, versions drifting
// out of order, an empty entry, or an entry written in English when the whole point
// of the file is that a user reads it in Russian.
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

// changelogHeading mirrors the regex in build.gradle.kts. The two must agree, or the
// build silently renders fewer sections than the file holds.
var changelogHeading = regexp.MustCompile(`^##\s+(\d+\.\d+\.\d+)\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$`)

type changelogEntry struct {
	version string
	date    string
	body    string
	line    int
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
		if m := changelogHeading.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
			if current != nil {
				current.body = body.String()
				entries = append(entries, *current)
			}
			body.Reset()
			current = &changelogEntry{version: m[1], date: m[2], line: i + 1}
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
// an out-of-order entry means the renderer would show the wrong release on top.
func TestChangelogSectionsAreOrdered(t *testing.T) {
	entries := parseChangelog(t)
	for i := 1; i < len(entries); i++ {
		prev, cur := entries[i-1], entries[i]
		if compareSemVer(t, cur.version, prev.version) >= 0 {
			t.Errorf("%s:%d: version %s is not below %s; sections must run newest first",
				changelogPath, cur.line, cur.version, prev.version)
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
				changelogPath, e.line, e.version)
			continue
		}
		if !hasCyrillic(text) {
			t.Errorf("%s:%d: entry %s has no Russian text; the Change Notes tab is written in Russian",
				changelogPath, e.line, e.version)
		}
		for _, line := range strings.Split(text, "\n") {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, "commit") {
				t.Errorf("%s:%d: entry %s reads like a commit list; describe the change, not the commits",
					changelogPath, e.line, e.version)
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
	const want = `^##\s+(\d+\.\d+\.\d+)\s*[—-]\s*(.+)$`
	if !strings.Contains(string(data), want) {
		t.Errorf("%s no longer contains the expected heading regex %q; update changelogHeading here to match",
			gradleBuildPath, want)
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
