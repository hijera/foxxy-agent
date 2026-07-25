//go:build http

package httpserver

// Godog harness for features/ide_last_session.feature: drives the live
// GET/PUT /foxxycode/project/last-session surface an editor plugin uses to
// reopen the session the user last had open in the project.

import (
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
	"github.com/hijera/foxxycode-agent/internal/project"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type lastSessionFeatureState struct {
	root      string
	home      string
	sessRoot  string
	projectWD string
	ts        *httptest.Server
	mgr       *session.Manager
	srv       *Server
	store     *session.FileStore
	sessionID string
	offered   string
}

func (s *lastSessionFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-lastsess-*")
	if err != nil {
		return err
	}
	s.root = root
	s.home = filepath.Join(root, "home")
	s.sessRoot = filepath.Join(root, "sessions")
	s.sessionID = ""
	s.offered = ""
	return nil
}

func (s *lastSessionFeatureState) close() {
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

// startServer boots a server whose current project is the named folder, the
// way a plugin does with `foxxycode http --cwd <project>`.
func (s *lastSessionFeatureState) startServer(name string) error {
	s.projectWD = filepath.Join(s.root, name)
	for _, d := range []string{filepath.Join(s.home, "memory"), s.sessRoot, s.projectWD} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return s.bootServer()
}

func (s *lastSessionFeatureState) bootServer() error {
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: s.home, CWD: s.projectWD},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	s.store = &session.FileStore{Root: s.sessRoot}
	s.mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.projectWD, s.store)
	s.srv = New(cfg, s.mgr, slog.Default(), s.projectWD)
	ps, err := project.Open(s.home)
	if err != nil {
		return err
	}
	if err := ps.SetCurrent(s.projectWD); err != nil {
		return err
	}
	s.srv.AttachProjectStore(ps)
	s.ts = httptest.NewServer(s.srv.Handler())
	return nil
}

// restart tears the server down and boots a fresh one against the same home,
// which is what happens when the IDE relaunches the backend on a new port.
func (s *lastSessionFeatureState) restart() error {
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.srv != nil {
		s.srv.Drain()
		s.srv = nil
	}
	return s.bootServer()
}

func (s *lastSessionFeatureState) sessionInProject(name string) error {
	if filepath.Base(s.projectWD) != name {
		return fmt.Errorf("project %q is not the running project %q", name, s.projectWD)
	}
	return s.createSession(s.projectWD)
}

func (s *lastSessionFeatureState) sessionOutsideProject(name string) error {
	dir := filepath.Join(s.root, "other", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return s.createSession(dir)
}

func (s *lastSessionFeatureState) createSession(cwd string) error {
	res, err := s.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		return err
	}
	st := s.mgr.SessionByID(res.SessionID)
	if st == nil {
		return fmt.Errorf("session %q not registered", res.SessionID)
	}
	if err := s.store.Save(st); err != nil {
		return err
	}
	s.sessionID = res.SessionID
	return nil
}

func (s *lastSessionFeatureState) recordSession() error {
	return s.putLastSession(s.sessionID)
}

func (s *lastSessionFeatureState) recordEmptySession() error {
	return s.putLastSession("")
}

func (s *lastSessionFeatureState) putLastSession(id string) error {
	body, err := json.Marshal(map[string]string{"session_id": id})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut,
		s.ts.URL+"/foxxycode/project/last-session", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("PUT last-session returned %d", res.StatusCode)
	}
	return nil
}

func (s *lastSessionFeatureState) deleteSession() error {
	if s.sessionID == "" {
		return fmt.Errorf("no session created")
	}
	return os.RemoveAll(s.store.SessionPath(s.sessionID))
}

func (s *lastSessionFeatureState) askWhichSession() error {
	res, err := http.Get(s.ts.URL + "/foxxycode/project/last-session")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET last-session returned %d", res.StatusCode)
	}
	var body struct {
		Object    string `json:"object"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	if body.Object != "foxxycode.project_last_session" {
		return fmt.Errorf("object = %q", body.Object)
	}
	s.offered = body.SessionID
	return nil
}

func (s *lastSessionFeatureState) noSessionOffered() error {
	if s.offered != "" {
		return fmt.Errorf("session %q offered, want none", s.offered)
	}
	return nil
}

func (s *lastSessionFeatureState) recordedSessionOffered() error {
	if s.offered != s.sessionID {
		return fmt.Errorf("offered %q, want %q", s.offered, s.sessionID)
	}
	return nil
}

func initializeLastSessionScenario(sc *godog.ScenarioContext) {
	s := &lastSessionFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server for project "([^"]+)"$`, s.startServer)
	sc.Step(`^a session in project "([^"]+)"$`, s.sessionInProject)
	sc.Step(`^a session in folder "([^"]+)" outside the project$`, s.sessionOutsideProject)
	sc.Step(`^the plugin recorded that session as last opened$`, s.recordSession)

	sc.Step(`^the plugin restarts on a new port$`, s.restart)
	sc.Step(`^the plugin records an empty session$`, s.recordEmptySession)
	sc.Step(`^that session is deleted$`, s.deleteSession)
	sc.Step(`^I ask which session to reopen$`, s.askWhichSession)

	sc.Step(`^no session is offered for reopening$`, s.noSessionOffered)
	sc.Step(`^the recorded session is offered for reopening$`, s.recordedSessionOffered)
}

func TestIDELastSessionFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "ide-last-session",
		ScenarioInitializer: initializeLastSessionScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/ide_last_session.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("ide last session feature suite failed")
	}
}
