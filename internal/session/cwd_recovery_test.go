package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/gitws"
)

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	return cmd.Run()
}

func TestRecoverManagedWorktreeCWD(t *testing.T) {
	repo := t.TempDir()
	if !gitws.GitAvailable() {
		t.Skip("git is unavailable")
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "foxxycode@test"}, {"config", "user.name", "foxxycode"}, {"commit", "--allow-empty", "-m", "init"}} {
		if err := runGit(repo, args...); err != nil {
			t.Fatal(err)
		}
	}

	removed := filepath.Join(repo, ".foxxycode", "worktrees", "feature", "nested")
	got, ok := RecoverManagedWorktreeCWD(removed)
	if !ok {
		t.Fatalf("RecoverManagedWorktreeCWD(%q) did not recover the repository", removed)
	}
	if got != repo {
		t.Fatalf("recovered cwd = %q, want %q", got, repo)
	}

	random := filepath.Join(t.TempDir(), "missing")
	if got, ok := RecoverManagedWorktreeCWD(random); ok || got != "" {
		t.Fatalf("random missing cwd recovered as %q, %v", got, ok)
	}

	lookalike := filepath.Join(repo, "other", ".foxxycode", "worktrees", "feature")
	if err := os.MkdirAll(filepath.Dir(lookalike), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := RecoverManagedWorktreeCWD(lookalike); ok || got != "" {
		t.Fatalf("lookalike cwd recovered as %q, %v", got, ok)
	}
}
