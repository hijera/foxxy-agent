package session

import (
	"path/filepath"
	"runtime"
	"testing"
)

// PathInWorkspace is the confinement predicate a restricted turn's reads ride
// on: the table pins the prefix-correct exclusion of a workspace-nested agent
// home - siblings with a similar name stay allowed, Windows inputs fold case,
// and a second volume fails closed.
func TestPathInWorkspaceTable(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "agent-home")
	note := filepath.Join(root, "note.txt")

	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"workspace note", note, true},
		{"relative path", "note.txt", true},
		{"relative below", filepath.Join("sub", "dir", "file.go"), true},
		{"outside the workspace", filepath.Join(filepath.Dir(root), "elsewhere.txt"), false},
		{"relative escape", "../escape.txt", false},
		{"nested home config", filepath.Join(home, "config.yaml"), false},
		{"nested home session bundle", filepath.Join(home, "sessions", "sess_1", "messages.json"), false},
		{"the home itself", home, false},
		// A name that merely shares the home's prefix is an ordinary
		// workspace directory: the exclusion must not swallow it.
		{"prefix-similar sibling", filepath.Join(root, "agent-home2", "file.txt"), true},
		{"dot-prefixed sibling", filepath.Join(root, ".agent-home-backup", "file.txt"), true},
		{"empty path", "", false},
		{"tilde path", "~/notes.md", false},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases,
			struct {
				name string
				p    string
				want bool
			}{"case-insensitive home", filepath.Join(root, "AGENT-HOME", "config.yaml"), false},
			struct {
				name string
				p    string
				want bool
			}{"case-insensitive workspace", filepath.Join(root, "NOTE.TXT"), true},
		)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathInWorkspace(tc.p, root, home); got != tc.want {
				t.Fatalf("PathInWorkspace(%q, cwd, home) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}

	t.Run("empty cwd denies", func(t *testing.T) {
		if PathInWorkspace(note, "", "") {
			t.Fatal("an empty cwd must deny every path")
		}
	})
	t.Run("no home keeps nothing out", func(t *testing.T) {
		if !PathInWorkspace(note, root, "") {
			t.Fatal("an empty home must keep nothing out of the workspace")
		}
	})
	if runtime.GOOS == "windows" {
		t.Run("second volume fails closed", func(t *testing.T) {
			if PathInWorkspace(`D:\other\secret.txt`, root, home) {
				t.Fatal("a path on another volume must be denied, not panicking Rel")
			}
		})
	}
}
