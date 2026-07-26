package svn_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	toolsvn "github.com/hijera/foxxycode-agent/internal/tools/svn"
)

const repoRoot = "https://svn.example.test/repo"

type harness struct {
	fake  svntest.Fake
	cfg   *config.Config
	wc    string
	tools map[string]*tooling.Tool
}

// newHarness builds the fake client, registers a working copy, and returns the
// registered svn tools keyed by name.
func newHarness(t *testing.T) *harness {
	t.Helper()
	fake, err := svntest.Build(t.TempDir())
	if err != nil {
		t.Fatalf("build fake svn: %v", err)
	}
	fake.Setenv(t.Setenv)

	wc := filepath.Join(t.TempDir(), "branch-folder")
	if err := os.MkdirAll(wc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fake.WriteState(svntest.NewState(repoRoot, wc)); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.VCS.SVN.Binary = fake.Binary
	cfg.VCS.SVN.TimeoutSeconds = 30

	h := &harness{fake: fake, cfg: cfg, wc: wc, tools: map[string]*tooling.Tool{}}
	toolsvn.RegisterBuiltins(func(tool *tooling.Tool) {
		h.tools[tool.Definition.Name] = tool
	}, cfg)
	return h
}

func (h *harness) run(t *testing.T, name string, args map[string]interface{}) string {
	t.Helper()
	tool, ok := h.tools[name]
	if !ok {
		t.Fatalf("tool %q is not registered", name)
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Execute(context.Background(), string(raw), &tooling.Env{CWD: h.wc})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

func TestRegistrationCoversTheWorkingSet(t *testing.T) {
	h := newHarness(t)
	for _, name := range toolsvn.ToolNames() {
		if _, ok := h.tools[name]; !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestMutatingToolsRequirePermission(t *testing.T) {
	h := newHarness(t)
	readOnly := map[string]bool{}
	for _, n := range toolsvn.ReadOnlyToolNames() {
		readOnly[n] = true
	}
	for name, tool := range h.tools {
		if readOnly[name] && tool.RequiresPermission {
			t.Errorf("%s is read-only but requires permission", name)
		}
		if !readOnly[name] && !tool.RequiresPermission {
			t.Errorf("%s mutates state but does not require permission", name)
		}
	}
}

func TestToolsAreNotRegisteredWhenDisabled(t *testing.T) {
	fake, err := svntest.Build(t.TempDir())
	if err != nil {
		t.Fatalf("build fake svn: %v", err)
	}
	disabled := false
	cfg := &config.Config{}
	cfg.VCS.SVN.Binary = fake.Binary
	cfg.VCS.SVN.Enabled = &disabled

	registered := 0
	toolsvn.RegisterBuiltins(func(*tooling.Tool) { registered++ }, cfg)
	if registered != 0 {
		t.Fatalf("registered %d tools with vcs.svn.enabled=false", registered)
	}
	if toolsvn.Enabled(cfg) {
		t.Errorf("Enabled reports true with vcs.svn.enabled=false")
	}
}

func TestToolsAreNotRegisteredWithoutClient(t *testing.T) {
	cfg := &config.Config{}
	cfg.VCS.SVN.Binary = filepath.Join(t.TempDir(), "definitely-missing-svn")

	registered := 0
	toolsvn.RegisterBuiltins(func(*tooling.Tool) { registered++ }, cfg)
	if registered != 0 {
		t.Fatalf("registered %d tools without an svn client", registered)
	}
}

func TestInfoReportsBranchAndRevision(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "svn_info", map[string]interface{}{})
	for _, want := range []string{"branch: trunk", "revision: 12", repoRoot + "/trunk"} {
		if !strings.Contains(out, want) {
			t.Errorf("svn_info output %q missing %q", out, want)
		}
	}
}

func TestStatusDiffAndLogRelayClientOutput(t *testing.T) {
	h := newHarness(t)
	if out := h.run(t, "svn_status", map[string]interface{}{}); !strings.Contains(out, "M       src/main.go") {
		t.Errorf("svn_status = %q", out)
	}
	if out := h.run(t, "svn_diff", map[string]interface{}{}); !strings.Contains(out, "+// changed") {
		t.Errorf("svn_diff = %q", out)
	}
	if out := h.run(t, "svn_log", map[string]interface{}{"limit": 5}); !strings.Contains(out, "r12") {
		t.Errorf("svn_log = %q", out)
	}
}

func TestCommitPassesExplicitPaths(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "svn_commit", map[string]interface{}{
		"message": "fix: nested repo untouched",
		"paths":   []string{"src/main.go"},
	})
	if !strings.Contains(out, "Committed revision 13") {
		t.Errorf("svn_commit = %q", out)
	}
	call, ok := h.fake.FindCall("commit")
	if !ok {
		t.Fatalf("commit was not invoked")
	}
	joined := strings.Join(call.Args, " ")
	if !strings.Contains(joined, "--message fix: nested repo untouched") {
		t.Errorf("commit args = %v", call.Args)
	}
	if !strings.HasSuffix(joined, "-- src/main.go") {
		t.Errorf("commit must pass explicit paths after --; got %v", call.Args)
	}
}

func TestCommitWithoutPathsIsRejected(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "svn_commit", map[string]interface{}{"message": "no paths"})
	if !strings.Contains(out, "error:") {
		t.Fatalf("expected an error result, got %q", out)
	}
	if _, ok := h.fake.FindCall("commit"); ok {
		t.Errorf("commit must not reach the client without paths")
	}
}

func TestSwitchResolvesBranchNameAgainstRepositoryRoot(t *testing.T) {
	h := newHarness(t)
	h.run(t, "svn_switch", map[string]interface{}{"branch": "branches/feature-x"})

	call, ok := h.fake.FindCall("switch")
	if !ok {
		t.Fatalf("switch was not invoked")
	}
	if !strings.Contains(strings.Join(call.Args, " "), repoRoot+"/branches/feature-x") {
		t.Fatalf("switch args = %v", call.Args)
	}
	out := h.run(t, "svn_info", map[string]interface{}{})
	if !strings.Contains(out, "branch: branches/feature-x") {
		t.Errorf("working copy did not move to the branch: %q", out)
	}
}

func TestSwitchToUnknownBranchReportsTheClientError(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "svn_switch", map[string]interface{}{"branch": "branches/nope"})
	if !strings.Contains(out, "error:") {
		t.Fatalf("expected a client error, got %q", out)
	}
}

func TestUpdateAdvancesTheRevision(t *testing.T) {
	h := newHarness(t)
	if out := h.run(t, "svn_update", map[string]interface{}{}); !strings.Contains(out, "At revision 13") {
		t.Errorf("svn_update = %q", out)
	}
}

func TestMergeReportsTheResult(t *testing.T) {
	h := newHarness(t)
	out := h.run(t, "svn_merge", map[string]interface{}{"source": "branches/release-1"})
	if !strings.Contains(out, "Merging") {
		t.Errorf("svn_merge = %q", out)
	}
	call, ok := h.fake.FindCall("merge")
	if !ok {
		t.Fatalf("merge was not invoked")
	}
	if !strings.Contains(strings.Join(call.Args, " "), repoRoot+"/branches/release-1") {
		t.Errorf("merge args = %v", call.Args)
	}
}

func TestMergeConflictSurfacesAsAnError(t *testing.T) {
	h := newHarness(t)
	state := svntest.NewState(repoRoot, h.wc)
	state.Fail = map[string]string{
		"merge": "svn: E155015: One or more conflicts were produced while merging",
	}
	if err := h.fake.WriteState(state); err != nil {
		t.Fatal(err)
	}
	out := h.run(t, "svn_merge", map[string]interface{}{"source": "branches/release-1"})
	if !strings.Contains(out, "E155015") {
		t.Fatalf("merge conflict not surfaced: %q", out)
	}
}

func TestCheckoutCreatesABranchFolder(t *testing.T) {
	h := newHarness(t)
	dest := filepath.Join(t.TempDir(), "feature-folder")
	out := h.run(t, "svn_checkout", map[string]interface{}{
		"branch":      "branches/feature-x",
		"destination": dest,
	})
	if !strings.Contains(out, "Checked out revision") {
		t.Fatalf("svn_checkout = %q", out)
	}
	if _, err := os.Stat(filepath.Join(dest, ".svn")); err != nil {
		t.Fatalf("checkout folder not created: %v", err)
	}
}

func TestToolsRefuseOutsideAWorkingCopy(t *testing.T) {
	h := newHarness(t)
	plain := t.TempDir()
	for _, name := range []string{"svn_info", "svn_status", "svn_commit"} {
		tool := h.tools[name]
		out, err := tool.Execute(context.Background(), `{"message":"m","paths":["a"]}`,
			&tooling.Env{CWD: plain})
		if err != nil {
			t.Fatalf("%s returned a hard error: %v", name, err)
		}
		if !strings.Contains(out, "not an svn working copy") {
			t.Errorf("%s outside a working copy = %q", name, out)
		}
	}
}

// A git repository living inside an SVN branch folder must not disturb the svn
// tools: they still resolve the enclosing working copy.
func TestToolsWorkWithANestedGitRepository(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Join(h.wc, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := h.run(t, "svn_info", map[string]interface{}{})
	if !strings.Contains(out, "branch: trunk") {
		t.Fatalf("svn_info with a nested git repo = %q", out)
	}
}
