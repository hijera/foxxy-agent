package forkguard

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	rulePath   = ".claude/rules/upstream-divergences.md"
	mirrorPath = ".cursor/rules/upstream-divergences.mdc"
	syncPath   = "UPSTREAM_SYNC.md"

	syncStart = "<!-- divergence-registry:start -->"
	syncEnd   = "<!-- divergence-registry:end -->"
)

var (
	// markerRx finds a marker in a Go comment: `// fork(<id>)`.
	markerRx = regexp.MustCompile(`//.*?fork\(([a-z0-9][a-z0-9-]*)\)`)
	// idCellRx reads the ID of a registry row: its first cell, in backticks.
	idCellRx = regexp.MustCompile("^\\|\\s*`([a-z0-9][a-z0-9-]*)`\\s*\\|")
	// testNameRx finds the guard tests a row names.
	testNameRx = regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")
)

// entry is one row of the registry.
type entry struct {
	id, kind string
	tests    []string
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// body strips a rule file's frontmatter.
func body(text string) string {
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	parts := strings.SplitN(text, "---\n", 3)
	if len(parts) < 3 {
		return text
	}
	return parts[2]
}

func parseRegistry(t *testing.T, text string) []entry {
	t.Helper()
	var out []entry
	for _, line := range strings.Split(text, "\n") {
		m := idCellRx.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 8 {
			t.Fatalf("registry row %q has %d cells, want 7 columns", m[1], len(cells)-2)
		}
		e := entry{id: m[1], kind: strings.TrimSpace(cells[2])}
		for _, tm := range testNameRx.FindAllStringSubmatch(cells[6], -1) {
			e.tests = append(e.tests, tm[1])
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		t.Fatalf("no registry rows found in %s", rulePath)
	}
	return out
}

// goFiles walks the Go sources of the module, skipping hidden directories
// (the main checkout keeps other worktrees under .claude), dependencies and
// build output.
func goFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "build" || name == "dist" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			rel, _ := filepath.Rel(root, path)
			out[filepath.ToSlash(rel)] = readText(t, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The registry, the markers in the code and the guard tests hold each other:
// a port that takes the upstream side of a marked site, or loses a test that
// pins a decision, fails here.
func TestDivergenceRegistryHoldsItsSites(t *testing.T) {
	root := repoRoot(t)
	entries := parseRegistry(t, body(readText(t, filepath.Join(root, rulePath))))
	files := goFiles(t, root)

	known := map[string]entry{}
	for _, e := range entries {
		if _, dup := known[e.id]; dup {
			t.Errorf("registry ID %q appears twice", e.id)
		}
		known[e.id] = e
	}

	markedIn := map[string][]string{}
	for path, text := range files {
		for _, m := range markerRx.FindAllStringSubmatch(text, -1) {
			id := m[1]
			if _, ok := known[id]; !ok {
				t.Errorf("%s: marker fork(%s) names no registry entry", path, id)
				continue
			}
			if !strings.HasSuffix(path, "_test.go") {
				markedIn[id] = append(markedIn[id], path)
			}
		}
	}

	funcs := map[string]bool{}
	funcRx := regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)
	for path, text := range files {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}
		for _, m := range funcRx.FindAllStringSubmatch(text, -1) {
			funcs[m[1]] = true
		}
	}

	for _, e := range entries {
		switch e.kind {
		case "code":
			if len(markedIn[e.id]) == 0 {
				t.Errorf("%s: no // fork(%s) marker is left in the code; a port took the upstream side, or the entry must go", e.id, e.id)
			}
			if len(e.tests) == 0 {
				t.Errorf("%s: a code entry names no guard test", e.id)
			}
			for _, name := range e.tests {
				if !funcs[name] {
					t.Errorf("%s: guard test %s does not exist", e.id, name)
				}
			}
		case "deferred":
		default:
			t.Errorf("%s: unknown kind %q (code or deferred)", e.id, e.kind)
		}
	}
}

// Cursor and Codex read the .mdc copy of the rule, Claude Code the .md one:
// the two carry the same text under their own frontmatter.
func TestDivergenceRuleMirrorMatches(t *testing.T) {
	root := repoRoot(t)
	rule := body(readText(t, filepath.Join(root, rulePath)))
	mirror := body(readText(t, filepath.Join(root, mirrorPath)))
	if rule != mirror {
		t.Fatalf("%s and %s have drifted apart; carry the edit into both", rulePath, mirrorPath)
	}
}

// UPSTREAM_SYNC.md keeps the same table in Russian, for the operator who reads
// the port history; its IDs are the rule's.
func TestUpstreamSyncListsTheSameDivergences(t *testing.T) {
	root := repoRoot(t)
	entries := parseRegistry(t, body(readText(t, filepath.Join(root, rulePath))))
	sync := readText(t, filepath.Join(root, syncPath))
	start, end := strings.Index(sync, syncStart), strings.Index(sync, syncEnd)
	if start < 0 || end < start {
		t.Fatalf("%s has no %s ... %s block", syncPath, syncStart, syncEnd)
	}
	var syncIDs, ruleIDs []string
	for _, line := range strings.Split(sync[start:end], "\n") {
		if m := idCellRx.FindStringSubmatch(line); m != nil {
			syncIDs = append(syncIDs, m[1])
		}
	}
	for _, e := range entries {
		ruleIDs = append(ruleIDs, e.id)
	}
	sort.Strings(syncIDs)
	sort.Strings(ruleIDs)
	if strings.Join(syncIDs, ",") != strings.Join(ruleIDs, ",") {
		t.Fatalf("%s lists %v, the rule lists %v", syncPath, syncIDs, ruleIDs)
	}
}
