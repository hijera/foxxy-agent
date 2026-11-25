package gitws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

func writeConfig(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// excluded mirrors how go-git's status reads wt.Excludes: the last pattern
// that matches decides.
func excluded(patterns []gitignore.Pattern, rel string) bool {
	result := gitignore.NoMatch
	for _, p := range patterns {
		if r := p.Match(strings.Split(rel, "/"), false); r != gitignore.NoMatch {
			result = r
		}
	}
	return result == gitignore.Exclude
}

// git applies core.excludesFile from the highest-priority configuration that
// names one, not the union of every level: a file ignored only by a
// lower-priority excludesFile stays visible to the git binary, and the
// built-in backend must agree with it.
func TestGlobalExcludesFromKeepsOnlyTheHighestPriorityFile(t *testing.T) {
	dir := t.TempDir()
	sysIgnore := writeConfig(t, dir, "sys.ignore", "sys-only\n")
	userIgnore := writeConfig(t, dir, "user.ignore", "user-only\n")
	sysCfg := writeConfig(t, dir, "sysconfig", "[core]\n\texcludesFile = "+filepath.ToSlash(sysIgnore)+"\n")
	userCfg := writeConfig(t, dir, "userconfig", "[core]\n\texcludesFile = "+filepath.ToSlash(userIgnore)+"\n")

	patterns := globalExcludesFrom([]string{sysCfg, userCfg})
	if excluded(patterns, "sys-only") {
		t.Error("system-level excludesFile must not apply when the user level names one")
	}
	if !excluded(patterns, "user-only") {
		t.Error("user-level excludesFile must apply")
	}

	// A level that names no file leaves the lower one in effect, like git.
	patterns = globalExcludesFrom([]string{sysCfg, writeConfig(t, dir, "nokey", "[core]\n")})
	if !excluded(patterns, "sys-only") {
		t.Error("the highest-priority excludesFile that names a file must apply")
	}
}

// git expands only `~` and `~/...` as the home directory; `~user/...` names
// another user's home and must not be folded into $HOME.
func TestExcludesFileOfExpandsOnlyTheHomeTilde(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got := excludesFileOf(writeConfig(t, dir, "home-tilde", "[core]\n\texcludesFile = ~/global.ignore\n"))
	want := filepath.Join(home, "global.ignore")
	if got != want {
		t.Fatalf("~/ expansion: got %q, want %q", got, want)
	}

	got = excludesFileOf(writeConfig(t, dir, "user-tilde", "[core]\n\texcludesFile = ~user/global.ignore\n"))
	if got != "~user"+string(filepath.Separator)+"global.ignore" &&
		got != "~user/global.ignore" {
		t.Fatalf("~user must stay unexpanded, got %q", got)
	}
}
