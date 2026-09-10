//go:build http

package httpserver

// Godog harness for features/session_changes.feature: drives the live
// /foxxycode/sessions/{id}/changes surface behind the changed-files card, from
// a real turn that edits the workspace through to rolling the session back.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type changedFileRow struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary"`
	Patch     string `json:"patch"`
}

type sessionChangesState struct {
	root      string
	home      string
	workspace string
	sessRoot  string
	ts        *httptest.Server
	srv       *Server
	mgr       *session.Manager
	sessionID string

	// pendingWrite is what the stub agent does during the next turn.
	pendingWrite struct {
		path    string
		content string
	}
	files []changedFileRow
	one   changedFileRow
}

// gherkinText turns the literal backslash-n of a feature file into newlines so
// scenarios can spell out small file bodies inline.
func gherkinText(s string) string {
	return strings.ReplaceAll(s, `\n`, "\n")
}

func (s *sessionChangesState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-changes-*")
	if err != nil {
		return err
	}
	s.root = root
	s.home = filepath.Join(root, "home")
	s.workspace = filepath.Join(root, "workspace")
	s.sessRoot = filepath.Join(root, "sessions")
	s.sessionID = ""
	s.files = nil
	s.one = changedFileRow{}
	s.pendingWrite.path = ""
	return nil
}

func (s *sessionChangesState) close() {
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

func (s *sessionChangesState) startServer() error {
	for _, d := range []string{s.home, s.workspace, s.sessRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// The stub agent performs whatever edit the scenario queued, standing in
	// for a real tool call so the turn diff is captured the usual way.
	runner := func(_ context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		if s.pendingWrite.path != "" {
			target := filepath.Join(s.workspace, s.pendingWrite.path)
			if err := os.WriteFile(target, []byte(s.pendingWrite.content), 0o644); err != nil {
				return "", err
			}
			s.pendingWrite.path = ""
		}
		// Record the exchange the way the real ReAct loop does: turn diffs are
		// filed under the user-turn count, so a stub that never grows the
		// transcript would overwrite turn 0 on every prompt.
		text := ""
		if len(prompt) > 0 {
			text = prompt[0].Text
		}
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: text})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "done"})
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: s.home, CWD: s.workspace},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	store := &session.FileStore{Root: s.sessRoot}
	s.mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.workspace, store)
	s.srv = New(cfg, s.mgr, slog.Default(), s.workspace)
	s.ts = httptest.NewServer(s.srv.Handler())

	newRes, err := s.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.workspace})
	if err != nil {
		return err
	}
	s.sessionID = newRes.SessionID
	return nil
}

func (s *sessionChangesState) workspaceContains(name, content string) error {
	return os.WriteFile(filepath.Join(s.workspace, name), []byte(gherkinText(content)), 0o644)
}

// runTurn sends a prompt and waits for the workspace diff of that turn to land:
// the capture runs on a background goroutine, so the card would otherwise race it.
func (s *sessionChangesState) runTurn(name, content string) error {
	s.pendingWrite.path = name
	s.pendingWrite.content = gherkinText(content)

	before, err := session.ListStoredTurnDiffs(filepath.Join(s.sessRoot, s.sessionID))
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, s.ts.URL+"/v1/responses",
		strings.NewReader(`{"model":"agent","input":"edit it","stream":false}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /v1/responses returned %d", res.StatusCode)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		now, err := session.ListStoredTurnDiffs(filepath.Join(s.sessRoot, s.sessionID))
		if err != nil {
			return err
		}
		if len(now) > len(before) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("turn diff for %q was never stored", name)
}

func (s *sessionChangesState) askWhatChanged() error {
	return s.askWhatChangedInScope("")
}

// askWhatChangedInScope reads the change set the review window would show for
// one scope; an empty scope exercises the default the card uses.
func (s *sessionChangesState) askWhatChangedInScope(scope string) error {
	url := s.ts.URL + "/foxxycode/sessions/" + s.sessionID + "/changes"
	if scope != "" {
		url += "?scope=" + scope
	}
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET changes returned %d", res.StatusCode)
	}
	var body struct {
		Object string           `json:"object"`
		Files  []changedFileRow `json:"files"`
		Totals struct {
			Files     int `json:"files"`
			Additions int `json:"additions"`
			Deletions int `json:"deletions"`
		} `json:"totals"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	if body.Object != "foxxycode.session_changes" {
		return fmt.Errorf("object = %q", body.Object)
	}
	if body.Totals.Files != len(body.Files) {
		return fmt.Errorf("totals.files = %d but %d files listed", body.Totals.Files, len(body.Files))
	}
	s.files = body.Files
	return nil
}

func (s *sessionChangesState) openDiff(name string) error {
	res, err := http.Get(s.ts.URL + "/foxxycode/sessions/" + s.sessionID + "/changes/file?path=" + name)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET changes/file returned %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(&s.one)
}

func (s *sessionChangesState) rollBack() error {
	req, err := http.NewRequest(http.MethodPost,
		s.ts.URL+"/foxxycode/sessions/"+s.sessionID+"/changes/revert", nil)
	if err != nil {
		return err
	}
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST revert returned %d", res.StatusCode)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

func (s *sessionChangesState) noFilesChanged() error {
	if err := s.askWhatChanged(); err != nil {
		return err
	}
	if len(s.files) != 0 {
		return fmt.Errorf("want no changed files, got %d: %+v", len(s.files), s.files)
	}
	return nil
}

func (s *sessionChangesState) countAndStats(count, additions, deletions int) error {
	if len(s.files) != count {
		return fmt.Errorf("want %d changed file(s), got %d: %+v", count, len(s.files), s.files)
	}
	gotAdd, gotDel := 0, 0
	for _, f := range s.files {
		gotAdd += f.Additions
		gotDel += f.Deletions
	}
	if gotAdd != additions || gotDel != deletions {
		return fmt.Errorf("want +%d -%d, got +%d -%d", additions, deletions, gotAdd, gotDel)
	}
	return nil
}

func (s *sessionChangesState) reportedAs(name, status string) error {
	for _, f := range s.files {
		if f.Path == name {
			if f.Status != status {
				return fmt.Errorf("%s status = %q, want %q", name, f.Status, status)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not in the change set: %+v", name, s.files)
}

func (s *sessionChangesState) diffReplaces(removed, added string) error {
	if !strings.Contains(s.one.Patch, "\n-"+removed+"\n") {
		return fmt.Errorf("patch does not remove %q:\n%s", removed, s.one.Patch)
	}
	if !strings.Contains(s.one.Patch, "\n+"+added+"\n") {
		return fmt.Errorf("patch does not add %q:\n%s", added, s.one.Patch)
	}
	return nil
}

func (s *sessionChangesState) fileContains(name, content string) error {
	got, err := os.ReadFile(filepath.Join(s.workspace, name))
	if err != nil {
		return err
	}
	if string(got) != gherkinText(content) {
		return fmt.Errorf("%s = %q, want %q", name, got, gherkinText(content))
	}
	return nil
}

func (s *sessionChangesState) fileGone(name string) error {
	if _, err := os.Stat(filepath.Join(s.workspace, name)); !os.IsNotExist(err) {
		return fmt.Errorf("%s still exists (%v)", name, err)
	}
	return nil
}

func initializeSessionChangesScenario(sc *godog.ScenarioContext) {
	s := &sessionChangesState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server with a workspace$`, s.startServer)
	sc.Step(`^the workspace contains "([^"]+)" with "([^"]*)"$`, s.workspaceContains)
	sc.Step(`^the agent runs a turn that writes "([^"]+)" as "([^"]*)"$`, s.runTurn)
	sc.Step(`^I ask what the session changed$`, s.askWhatChanged)
	sc.Step(`^I ask what the last turn changed$`, func() error {
		return s.askWhatChangedInScope("turn")
	})
	sc.Step(`^I open the diff for "([^"]+)"$`, s.openDiff)
	sc.Step(`^I roll the session changes back$`, s.rollBack)

	sc.Step(`^no files are reported as changed$`, s.noFilesChanged)
	sc.Step(`^(\d+) files? (?:is|are) reported as changed with (\d+) additions? and (\d+) deletions?$`, s.countAndStats)
	sc.Step(`^"([^"]+)" is reported as "([^"]+)"$`, s.reportedAs)
	sc.Step(`^the diff removes "([^"]+)" and adds "([^"]+)"$`, s.diffReplaces)
	sc.Step(`^"([^"]+)" contains "([^"]*)"$`, s.fileContains)
	sc.Step(`^"([^"]+)" no longer exists$`, s.fileGone)
}

func TestSessionChangesFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "session-changes",
		ScenarioInitializer: initializeSessionChangesScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/session_changes.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("session changes feature suite failed")
	}
}
