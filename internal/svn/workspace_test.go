package svn_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svn"
	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
)

func TestBranchOperationReturnsSessionEffectsWithoutASessionManager(t *testing.T) {
	fake, err := svntest.Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	wc := filepath.Join(t.TempDir(), "working-copy")
	if err := os.MkdirAll(wc, 0o755); err != nil {
		t.Fatal(err)
	}
	state := svntest.NewState("https://svn.example.test/repo", wc)
	state.Branches = append(state.Branches, "branches/feature-a", "branches/feature-b")
	if err := fake.WriteState(state); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Paths.Home = t.TempDir()
	cfg.VCS.SVN.Binary = fake.Binary
	result, err := svn.SwitchBranch(context.Background(), cfg, wc, "branches/feature-a", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path == "" || result.Reload {
		t.Fatalf("branch folder effects: %+v", result)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatal(err)
	}
	result, err = svn.SwitchBranch(context.Background(), cfg, wc, "branches/feature-b", false)
	if err != nil || !result.Reload || result.Path != "" {
		t.Fatalf("in-place effects: %+v, %v", result, err)
	}
	result, err = svn.SwitchBranch(context.Background(), cfg, wc, "branches/feature-b", false)
	if err != nil || result.Reload || result.Path != "" {
		t.Fatalf("no-op effects: %+v, %v", result, err)
	}
}

func TestDisabledWorkspaceReportsConflictBeforeRunningClient(t *testing.T) {
	_, err := svn.SwitchBranch(context.Background(), nil, t.TempDir(), "trunk", false)
	var operationErr *svn.WorkspaceError
	if !errors.As(err, &operationErr) || !operationErr.Conflict {
		t.Fatalf("disabled SVN: %v", err)
	}
}
