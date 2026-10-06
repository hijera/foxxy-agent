//go:build http

package httpserver

// Godog harness for features/svn_workspace.feature: drives the live HTTP
// surface (workspace context and branch switching) against a fake Subversion
// client, including workspaces that are both git repositories and svn working
// copies.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/gitws"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
)

const bddSVNRepoRoot = "https://svn.example.test/repo"

type svnWSState struct {
	root      string
	sessRoot  string
	home      string
	ts        *httptest.Server
	mgr       *session.Manager
	srv       *Server
	cfg       *config.Config
	fake      svntest.Fake
	svnState  svntest.State
	folders   map[string]string
	sessionID string
	startCWD  string
	status    int
	body      map[string]interface{}
}

func (s *svnWSState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-svnws-*")
	if err != nil {
		return err
	}
	s.root = root
	s.sessRoot = filepath.Join(root, "sessions")
	s.home = filepath.Join(root, "home")
	s.folders = map[string]string{}
	s.sessionID = ""
	s.startCWD = ""
	s.status = 0
	s.body = nil
	return nil
}

func (s *svnWSState) close() {
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.srv != nil {
		s.srv.Drain()
		s.srv = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

// startServer boots the HTTP server with a freshly built fake svn client wired
// through vcs.svn.binary.
func (s *svnWSState) startServer() error {
	if err := os.MkdirAll(filepath.Join(s.home, "memory"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.sessRoot, 0o755); err != nil {
		return err
	}
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

	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: s.home, CWD: s.root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	cfg.VCS.SVN.Binary = fake.Binary
	cfg.VCS.SVN.TimeoutSeconds = 30
	cfg.VCS.SVN.BranchLookup = boolPtr(true)
	s.cfg = cfg

	store := &session.FileStore{Root: s.sessRoot}
	s.mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, store)
	s.srv = New(cfg, s.mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(s.srv.Handler())
	return nil
}

func boolPtr(v bool) *bool { return &v }

// replaceConfig mirrors what PUT /foxxycode/config does after a settings save.
func (s *svnWSState) replaceConfig(mutate func(*config.Config)) {
	next := *s.cfg
	mutate(&next)
	s.cfg = &next
	s.srv.ReplaceConfig(&next)
	s.mgr.ReplaceConfig(&next)
}

func (s *svnWSState) plainFolder(name string) error {
	dir := filepath.Join(s.root, "ws", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.folders[name] = dir
	return nil
}

func (s *svnWSState) svnWorkingCopy(name, branch string) error {
	if err := s.plainFolder(name); err != nil {
		return err
	}
	dir := s.folders[name]
	s.svnState = svntest.NewState(bddSVNRepoRoot, dir)
	s.svnState.WorkingCopies[dir].Branch = branch
	return s.fake.WriteState(s.svnState)
}

func (s *svnWSState) alsoGitRepo(name, branch string) error {
	dir, ok := s.folders[name]
	if !ok {
		return fmt.Errorf("unknown folder %q", name)
	}
	return s.initGitRepo(dir, branch)
}

func (s *svnWSState) initGitRepo(dir, branch string) error {
	if !gitws.GitAvailable() {
		return fmt.Errorf("git binary not available for BDD svn/git scenarios")
	}
	if err := bddGit(dir, "init", "-b", branch); err != nil {
		return err
	}
	return bddGit(dir, "-c", "user.email=foxxycode@test", "-c", "user.name=foxxycode",
		"commit", "--allow-empty", "-m", "init")
}

func (s *svnWSState) gitBranch(branch string) error {
	dir, ok := s.folders[s.gitFolder()]
	if !ok {
		return fmt.Errorf("no git folder registered")
	}
	return bddGit(dir, "branch", branch)
}

// gitFolder returns the folder that was turned into a git repository.
func (s *svnWSState) gitFolder() string {
	for name, dir := range s.folders {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return name
		}
	}
	return ""
}

// unversionedGitSubfolder creates a git clone in a subfolder that svn does not
// track, the usual shape of a git repository living inside an svn branch folder.
func (s *svnWSState) unversionedGitSubfolder(sub, wc, branch string) error {
	parent, ok := s.folders[wc]
	if !ok {
		return fmt.Errorf("unknown folder %q", wc)
	}
	dir := filepath.Join(parent, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.folders[sub] = dir
	s.svnState.Unversioned = append(s.svnState.Unversioned, dir)
	if err := s.fake.WriteState(s.svnState); err != nil {
		return err
	}
	return s.initGitRepo(dir, branch)
}

func (s *svnWSState) disableSVN() error {
	s.replaceConfig(func(c *config.Config) {
		disabled := false
		c.VCS.SVN.Enabled = &disabled
	})
	return nil
}

func (s *svnWSState) svnClientMissing() error {
	s.replaceConfig(func(c *config.Config) {
		c.VCS.SVN.Binary = filepath.Join(s.root, "bin", "definitely-missing-svn")
	})
	return nil
}

func (s *svnWSState) sessionRootedAt(name string) error {
	dir, ok := s.folders[name]
	if !ok {
		return fmt.Errorf("unknown folder %q", name)
	}
	res, err := s.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: dir})
	if err != nil {
		return err
	}
	s.sessionID = res.SessionID
	s.startCWD = dir
	return nil
}

func (s *svnWSState) sessionHasUserMessage() error {
	st := s.mgr.SessionByID(s.sessionID)
	if st == nil {
		return fmt.Errorf("session %q not registered", s.sessionID)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	return nil
}

func (s *svnWSState) do(req *http.Request) error {
	if s.sessionID != "" {
		req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	s.status = res.StatusCode
	s.body = nil
	var parsed map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err == nil {
		s.body = parsed
	}
	return nil
}

func (s *svnWSState) requestContext() error {
	req, err := http.NewRequest(http.MethodGet, s.ts.URL+"/foxxycode/workspace/context", nil)
	if err != nil {
		return err
	}
	return s.do(req)
}

func (s *svnWSState) postWorkspace(payload map[string]interface{}) error {
	if s.sessionID == "" {
		return fmt.Errorf("no session created")
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost,
		s.ts.URL+"/foxxycode/sessions/"+s.sessionID+"/workspace", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return s.do(req)
}

func (s *svnWSState) switchSVNBranch(branch string) error {
	return s.postWorkspace(map[string]interface{}{"branch": branch, "vcs": "svn"})
}

func (s *svnWSState) switchSVNBranchSeparateFolder(branch string) error {
	return s.postWorkspace(map[string]interface{}{
		"branch": branch, "vcs": "svn", "worktree": true,
	})
}

func (s *svnWSState) switchGitBranch(branch string) error {
	return s.postWorkspace(map[string]interface{}{"branch": branch})
}

// freshContext re-fetches the workspace context so assertions always run
// against live state rather than a stale switch response.
func (s *svnWSState) freshContext() (map[string]interface{}, error) {
	if err := s.requestContext(); err != nil {
		return nil, err
	}
	if s.status != http.StatusOK {
		return nil, fmt.Errorf("workspace context returned %d: %v", s.status, s.body)
	}
	if s.body == nil {
		return nil, fmt.Errorf("workspace context returned no JSON body")
	}
	return s.body, nil
}

func (s *svnWSState) svnSection() (map[string]interface{}, error) {
	ctxBody, err := s.freshContext()
	if err != nil {
		return nil, err
	}
	svn, _ := ctxBody["svn"].(map[string]interface{})
	if svn == nil {
		return nil, fmt.Errorf("context carries no svn section: %v", ctxBody)
	}
	return svn, nil
}

func (s *svnWSState) contextOnSVNBranch(branch string) error {
	ctxBody, err := s.freshContext()
	if err != nil {
		return err
	}
	if isRepo, _ := ctxBody["is_svn_repo"].(bool); !isRepo {
		return fmt.Errorf("context does not report an svn working copy: %v", ctxBody)
	}
	svn, _ := ctxBody["svn"].(map[string]interface{})
	if got, _ := svn["branch"].(string); got != branch {
		return fmt.Errorf("svn branch = %v, want %q", svn["branch"], branch)
	}
	return nil
}

func (s *svnWSState) contextSVNRevision(rev int) error {
	svn, err := s.svnSection()
	if err != nil {
		return err
	}
	got, _ := svn["revision"].(float64)
	if int(got) != rev {
		return fmt.Errorf("svn revision = %v, want %d", svn["revision"], rev)
	}
	return nil
}

func (s *svnWSState) contextSVNURL(url string) error {
	svn, err := s.svnSection()
	if err != nil {
		return err
	}
	if got, _ := svn["url"].(string); got != url {
		return fmt.Errorf("svn url = %v, want %q", svn["url"], url)
	}
	return nil
}

func (s *svnWSState) contextNotSVN() error {
	ctxBody, err := s.freshContext()
	if err != nil {
		return err
	}
	if isRepo, _ := ctxBody["is_svn_repo"].(bool); isRepo {
		return fmt.Errorf("context reports an svn working copy: %v", ctxBody)
	}
	return nil
}

func (s *svnWSState) contextSVNAvailability(negated string) error {
	svn, err := s.svnSection()
	if err != nil {
		return err
	}
	available, _ := svn["available"].(bool)
	want := negated == ""
	if available != want {
		return fmt.Errorf("svn available = %v, want %v", available, want)
	}
	return nil
}

func (s *svnWSState) contextListsSVNBranches(list string) error {
	svn, err := s.svnSection()
	if err != nil {
		return err
	}
	raw, _ := svn["branches"].([]interface{})
	have := map[string]bool{}
	for _, b := range raw {
		if v, ok := b.(string); ok {
			have[v] = true
		}
	}
	for _, want := range bddSplitList(list) {
		if !have[want] {
			return fmt.Errorf("svn branch %q missing in %v", want, raw)
		}
	}
	return nil
}

func (s *svnWSState) contextWCRootAboveSession() error {
	ctxBody, err := s.freshContext()
	if err != nil {
		return err
	}
	svn, _ := ctxBody["svn"].(map[string]interface{})
	nested, _ := svn["nested"].(bool)
	if !nested {
		return fmt.Errorf("expected a nested working copy: %v", svn)
	}
	path, _ := ctxBody["path"].(string)
	root, _ := svn["wc_root"].(string)
	if bddNormPath(path) == bddNormPath(root) {
		return fmt.Errorf("wc root %q equals the session folder", root)
	}
	return nil
}

func (s *svnWSState) contextOnGitBranch(branch string) error {
	ctxBody, err := s.freshContext()
	if err != nil {
		return err
	}
	if isRepo, _ := ctxBody["is_git_repo"].(bool); !isRepo {
		return fmt.Errorf("context does not report a git repo: %v", ctxBody)
	}
	if got, _ := ctxBody["branch"].(string); got != branch {
		return fmt.Errorf("git branch = %v, want %q", ctxBody["branch"], branch)
	}
	return nil
}

func (s *svnWSState) sessionFolderUnchanged() error {
	ctxBody, err := s.freshContext()
	if err != nil {
		return err
	}
	got, _ := ctxBody["path"].(string)
	if bddNormPath(got) != bddNormPath(s.startCWD) {
		return fmt.Errorf("session folder = %q, want %q", got, s.startCWD)
	}
	return nil
}

func (s *svnWSState) sessionMovedToBranchFolder() error {
	ctxBody, err := s.freshContext()
	if err != nil {
		return err
	}
	got, _ := ctxBody["path"].(string)
	if bddNormPath(got) == bddNormPath(s.startCWD) {
		return fmt.Errorf("session folder did not move: %q", got)
	}
	if _, err := os.Stat(filepath.Join(got, ".svn")); err != nil {
		return fmt.Errorf("branch folder %q is not a working copy: %w", got, err)
	}
	if !strings.Contains(bddNormPath(got), bddNormPath(s.home)) {
		return fmt.Errorf("branch folder %q is not under the home worktrees root", got)
	}
	return nil
}

func (s *svnWSState) requestFailsWithStatus(code int) error {
	if s.status != code {
		return fmt.Errorf("status = %d, want %d (body: %v)", s.status, code, s.body)
	}
	return nil
}

func initializeSVNWorkspaceScenario(sc *godog.ScenarioContext) {
	s := &svnWSState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server with a fake svn client$`, s.startServer)
	sc.Step(`^an svn working copy "([^"]+)" on branch "([^"]+)"$`, s.svnWorkingCopy)
	sc.Step(`^a plain folder "([^"]+)"$`, s.plainFolder)
	sc.Step(`^folder "([^"]+)" is also a git repository on branch "([^"]+)"$`, s.alsoGitRepo)
	sc.Step(`^the git repository has branch "([^"]+)"$`, s.gitBranch)
	sc.Step(`^an unversioned subfolder "([^"]+)" inside "([^"]+)" that is a git repository on branch "([^"]+)"$`,
		s.unversionedGitSubfolder)
	sc.Step(`^a session rooted at (?:the subfolder|folder) "([^"]+)"$`, s.sessionRootedAt)
	sc.Step(`^the session already has a user message$`, s.sessionHasUserMessage)
	sc.Step(`^subversion support is disabled in the settings$`, s.disableSVN)
	sc.Step(`^the svn client is not installed$`, s.svnClientMissing)

	sc.Step(`^I request the workspace context$`, s.requestContext)
	sc.Step(`^I switch the session to svn branch "([^"]+)" in a separate folder$`, s.switchSVNBranchSeparateFolder)
	sc.Step(`^I switch the session to svn branch "([^"]+)"$`, s.switchSVNBranch)
	sc.Step(`^I switch the session to branch "([^"]+)"$`, s.switchGitBranch)

	sc.Step(`^the context reports an svn working copy on branch "([^"]+)"$`, s.contextOnSVNBranch)
	sc.Step(`^the context reports svn revision (\d+)$`, s.contextSVNRevision)
	sc.Step(`^the context reports the svn url "([^"]+)"$`, s.contextSVNURL)
	sc.Step(`^the context reports it is not an svn working copy$`, s.contextNotSVN)
	sc.Step(`^the context reports the svn client is (un)?available$`, s.contextSVNAvailability)
	sc.Step(`^the context lists svn branches "([^"]+)"$`, s.contextListsSVNBranches)
	sc.Step(`^the context reports the svn working copy root is above the session folder$`, s.contextWCRootAboveSession)
	sc.Step(`^the context reports a git repository on branch "([^"]+)"$`, s.contextOnGitBranch)
	sc.Step(`^the session folder is unchanged$`, s.sessionFolderUnchanged)
	sc.Step(`^the session moved to a new branch folder$`, s.sessionMovedToBranchFolder)
	sc.Step(`^the workspace request fails with status (\d+)$`, s.requestFailsWithStatus)
}

func TestSVNWorkspaceFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "svn-workspace",
		ScenarioInitializer: initializeSVNWorkspaceScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/svn_workspace.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("svn workspace feature suite failed")
	}
}
