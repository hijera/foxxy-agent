package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

// hooks.files keeps a leading ~ as written (the config loader no longer expands it),
// so the loader is what resolves it, whichever separator follows it.
func TestExpandFileResolvesALeadingTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no user home directory")
	}
	sep := string(filepath.Separator)
	for in, want := range map[string]string{
		"~/hooks.json":                          filepath.Join(home, "hooks.json"),
		"~" + sep + "team" + sep + "hooks.json": filepath.Join(home, "team", "hooks.json"),
	} {
		if got := expandFile(in, t.TempDir(), "/agent-home"); got != filepath.Clean(want) {
			t.Errorf("expandFile(%q) = %q, want %q", in, got, want)
		}
	}
}
