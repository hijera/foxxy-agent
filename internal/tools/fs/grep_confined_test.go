package fs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func TestGrepConfinedChecksPathsBeforeReadingAndCounting(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "00-home")
	writeSearchFixture(t, filepath.Join(home, "config.yaml"), "needle credential\n")
	writeSearchFixture(t, filepath.Join(root, "note.txt"), "needle public note\n")
	env := &tooling.Env{
		CWD: root, Confined: true,
		ReadPathAllowed: func(p string) bool {
			if p == home {
				return false
			}
			if isWithinDir(p, home) {
				t.Fatalf("search entered a denied directory: %s", p)
			}
			return true
		},
	}
	runner := grepRunner{
		lookPath: func(string) (string, error) {
			t.Fatal("a confined search must not delegate unchecked paths to ripgrep")
			return "", nil
		},
	}
	args, err := json.Marshal(map[string]any{"pattern": "needle", "max_results": 1})
	if err != nil {
		t.Fatal(err)
	}
	out, err := executeGrepWithRunner(context.Background(), string(args), env, runner)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "credential") || !strings.Contains(out, "public note") {
		t.Fatalf("confined search lost the public result or read a private file: %q", out)
	}
}

func TestGrepConfinedRefusesDeniedSearchRoots(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "secret.txt")
	writeSearchFixture(t, file, "credential\n")
	for _, path := range []string{"", root, file} {
		t.Run(path, func(t *testing.T) {
			args, err := json.Marshal(map[string]string{"pattern": "credential", "path": path})
			if err != nil {
				t.Fatal(err)
			}
			out, err := executeGrepWithRunner(context.Background(), string(args), &tooling.Env{
				CWD: root, Confined: true, ReadPathAllowed: func(string) bool { return false },
			}, nativeOnlyGrepRunner())
			if err == nil || out != "" {
				t.Fatalf("denied search root returned output=%q err=%v", out, err)
			}
		})
	}
}
