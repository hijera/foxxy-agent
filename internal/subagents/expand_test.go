package subagents

import (
	"os"
	"path/filepath"
	"testing"
)

// subagents.dirs keeps a leading ~ as written (the config loader no longer expands
// it), so the loader is what resolves it, whichever separator follows it.
func TestExpandDirResolvesALeadingTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no user home directory")
	}
	sep := string(filepath.Separator)
	for in, want := range map[string]string{
		"~":                  home,
		"~/team-agents":      filepath.Join(home, "team-agents"),
		"~" + sep + "agents": filepath.Join(home, "agents"),
	} {
		if got := expandDir(in, t.TempDir(), "/agent-home"); got != filepath.Clean(want) {
			t.Errorf("expandDir(%q) = %q, want %q", in, got, want)
		}
	}
}
