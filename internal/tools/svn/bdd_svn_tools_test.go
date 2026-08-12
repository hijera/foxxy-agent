package svn_test

// Godog harness for features/svn_tools.feature: drives the registered svn_*
// tools exactly as the agent would, against the fake Subversion client.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	toolsvn "github.com/hijera/foxxycode-agent/internal/tools/svn"
)

type svnToolsState struct {
	fake   svntest.Fake
	cfg    *config.Config
	root   string
	wc     string
	state  svntest.State
	tools  map[string]*tooling.Tool
	result string
}

func (s *svnToolsState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-svn-tools-*")
	if err != nil {
		return err
	}
	s.root = root
	s.wc = ""
	s.result = ""
	s.tools = map[string]*tooling.Tool{}
	s.cfg = &config.Config{}
	return nil
}

func (s *svnToolsState) close() {
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

// buildFakeClient compiles the fake svn client once per suite; the state and log
// files live in the per-scenario temp dir.
func (s *svnToolsState) buildFakeClient() error {
	binDir := filepath.Join(s.root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	fake, err := svntest.Build(binDir)
	if err != nil {
		return err
	}
	s.fake = fake
	if err := os.Setenv(svntest.EnvState, fake.State); err != nil {
		return err
	}
	if err := os.Setenv(svntest.EnvLog, fake.Log); err != nil {
		return err
	}
	s.cfg.VCS.SVN.Binary = fake.Binary
	s.cfg.VCS.SVN.TimeoutSeconds = 30
	return nil
}

// registerTools rebuilds the tool set from the current config, mirroring what
// agent.NewAgent does at the start of every prompt turn.
func (s *svnToolsState) registerTools() {
	s.tools = map[string]*tooling.Tool{}
	toolsvn.RegisterBuiltins(func(tool *tooling.Tool) {
		s.tools[tool.Definition.Name] = tool
	}, s.cfg)
}

func (s *svnToolsState) workingCopyOnBranch(branch string) error {
	s.wc = filepath.Join(s.root, "wc")
	if err := os.MkdirAll(s.wc, 0o755); err != nil {
		return err
	}
	s.state = svntest.NewState("https://svn.example.test/repo", s.wc)
	s.state.WorkingCopies[s.wc].Branch = branch
	if err := s.fake.WriteState(s.state); err != nil {
		return err
	}
	s.registerTools()
	return nil
}

func (s *svnToolsState) conflictOnMerge() error {
	s.state.Fail = map[string]string{
		"merge": "svn: E155015: One or more conflicts were produced while merging",
	}
	return s.fake.WriteState(s.state)
}

func (s *svnToolsState) alsoAGitRepository() error {
	return os.MkdirAll(filepath.Join(s.wc, ".git"), 0o755)
}

func (s *svnToolsState) disableSVN() error {
	disabled := false
	s.cfg.VCS.SVN.Enabled = &disabled
	s.registerTools()
	return nil
}

func (s *svnToolsState) enableSVN() error {
	enabled := true
	s.cfg.VCS.SVN.Enabled = &enabled
	s.registerTools()
	return nil
}

func (s *svnToolsState) call(name string, args map[string]interface{}) error {
	tool, ok := s.tools[name]
	if !ok {
		return fmt.Errorf("tool %q is not registered", name)
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return err
	}
	out, err := tool.Execute(context.Background(), string(raw), &tooling.Env{CWD: s.wc})
	if err != nil {
		return err
	}
	s.result = out
	return nil
}

func (s *svnToolsState) callSimple(name string) error {
	return s.call(name, map[string]interface{}{})
}

func (s *svnToolsState) commit(path, message string) error {
	return s.call("svn_commit", map[string]interface{}{
		"message": message,
		"paths":   []string{path},
	})
}

func (s *svnToolsState) switchToBranch(branch string) error {
	return s.call("svn_switch", map[string]interface{}{"branch": branch})
}

func (s *svnToolsState) mergeFromBranch(branch string) error {
	return s.call("svn_merge", map[string]interface{}{"source": branch})
}

func (s *svnToolsState) checkoutInto(branch, folder string) error {
	return s.call("svn_checkout", map[string]interface{}{
		"branch":      branch,
		"destination": filepath.Join(s.root, folder),
	})
}

func (s *svnToolsState) resultReportsBranch(branch string) error {
	return s.resultContains("branch: " + branch)
}

func (s *svnToolsState) resultReportsRevision(rev int) error {
	return s.resultContains(fmt.Sprintf("revision: %d", rev))
}

func (s *svnToolsState) resultContains(want string) error {
	if !strings.Contains(s.result, want) {
		return fmt.Errorf("tool result %q does not contain %q", s.result, want)
	}
	return nil
}

// clientCalledWith matches a recorded invocation by its argument list, ignoring
// the global flags svnws always prepends.
func (s *svnToolsState) clientCalledWith(want string) error {
	calls, err := s.fake.Calls()
	if err != nil {
		return err
	}
	for _, c := range calls {
		if strings.Contains(strings.Join(c.Args, " "), want) {
			return nil
		}
	}
	return fmt.Errorf("no svn invocation matched %q; recorded: %v", want, calls)
}

func (s *svnToolsState) followingInfoReportsBranch(branch string) error {
	if err := s.callSimple("svn_info"); err != nil {
		return err
	}
	return s.resultReportsBranch(branch)
}

func (s *svnToolsState) folderIsWorkingCopy(name string) error {
	if _, err := os.Stat(filepath.Join(s.root, name, ".svn")); err != nil {
		return fmt.Errorf("%s is not a working copy: %w", name, err)
	}
	return nil
}

func (s *svnToolsState) toolsRunWithoutPermission(list string) error {
	for _, name := range bddSplitList(list) {
		tool, ok := s.tools[name]
		if !ok {
			return fmt.Errorf("tool %q is not registered", name)
		}
		if tool.RequiresPermission {
			return fmt.Errorf("%s should not require permission", name)
		}
	}
	return nil
}

func (s *svnToolsState) toolsRequirePermission(list string) error {
	for _, name := range bddSplitList(list) {
		tool, ok := s.tools[name]
		if !ok {
			return fmt.Errorf("tool %q is not registered", name)
		}
		if !tool.RequiresPermission {
			return fmt.Errorf("%s must require permission", name)
		}
	}
	return nil
}

func (s *svnToolsState) noToolOffered() error {
	if len(s.tools) != 0 {
		names := make([]string, 0, len(s.tools))
		for n := range s.tools {
			names = append(names, n)
		}
		return fmt.Errorf("svn tools still offered: %v", names)
	}
	return nil
}

func (s *svnToolsState) toolsOffered(list string) error {
	for _, name := range bddSplitList(list) {
		if _, ok := s.tools[name]; !ok {
			return fmt.Errorf("tool %q is not offered to the model", name)
		}
	}
	return nil
}

func bddSplitList(list string) []string {
	parts := strings.Split(list, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func initializeSVNToolsScenario(sc *godog.ScenarioContext) {
	s := &svnToolsState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a fake svn client$`, s.buildFakeClient)
	sc.Step(`^an svn working copy on branch "([^"]+)"$`, s.workingCopyOnBranch)
	sc.Step(`^the svn client reports a conflict on merge$`, s.conflictOnMerge)
	sc.Step(`^the working copy also holds a git repository$`, s.alsoAGitRepository)
	sc.Step(`^subversion support is disabled in the settings$`, s.disableSVN)
	sc.Step(`^subversion support is enabled again in the settings$`, s.enableSVN)

	sc.Step(`^the model calls (svn_[a-z]+)$`, s.callSimple)
	sc.Step(`^the model commits "([^"]+)" with the message "([^"]+)"$`, s.commit)
	sc.Step(`^the model switches to branch "([^"]+)"$`, s.switchToBranch)
	sc.Step(`^the model merges from branch "([^"]+)"$`, s.mergeFromBranch)
	sc.Step(`^the model checks out branch "([^"]+)" into "([^"]+)"$`, s.checkoutInto)

	sc.Step(`^the tool result reports branch "([^"]+)"$`, s.resultReportsBranch)
	sc.Step(`^the tool result reports revision (\d+)$`, s.resultReportsRevision)
	sc.Step(`^the tool result contains "([^"]+)"$`, s.resultContains)
	sc.Step(`^the svn client was called with "([^"]+)"$`, s.clientCalledWith)
	sc.Step(`^a following svn_info reports branch "([^"]+)"$`, s.followingInfoReportsBranch)
	sc.Step(`^the folder "([^"]+)" is an svn working copy$`, s.folderIsWorkingCopy)
	sc.Step(`^the tools "([^"]+)" run without permission$`, s.toolsRunWithoutPermission)
	sc.Step(`^the tools "([^"]+)" require permission$`, s.toolsRequirePermission)
	sc.Step(`^no svn tool is offered to the model$`, s.noToolOffered)
	sc.Step(`^the tools "([^"]+)" are offered to the model$`, s.toolsOffered)
}

func TestSVNToolsFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "svn-tools",
		ScenarioInitializer: initializeSVNToolsScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../../features/svn_tools.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("svn tools feature suite failed")
	}
}
