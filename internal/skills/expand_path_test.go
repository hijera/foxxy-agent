package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/skills"
)

// skills.dirs keeps a leading ~ as written (the config loader no longer expands it),
// so the skill loader is what resolves it, whichever separator follows it.
func TestExpandConfiguredPathResolvesALeadingTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no user home directory")
	}
	sep := string(filepath.Separator)
	for in, want := range map[string]string{
		"~":                  home,
		"~/team-skills":      filepath.Join(home, "team-skills"),
		"~" + sep + "skills": filepath.Join(home, "skills"),
	} {
		if got := skills.ExpandConfiguredPath(in, "/work", "/agent-home"); filepath.Clean(got) != filepath.Clean(want) {
			t.Errorf("ExpandConfiguredPath(%q) = %q, want %q", in, got, want)
		}
	}
}
