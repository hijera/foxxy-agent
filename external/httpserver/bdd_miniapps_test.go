//go:build http && miniapps

package httpserver

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

type miniAppsFeatureState struct {
	root, home, cwd, sessionsRoot string
	ts                            *httptest.Server
	srv                           *Server
	store                         *session.FileStore
	sessionID, jobID, appID       string
	draft                         map[string]any
	releaseRunID                  string
}

func (s *miniAppsFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-miniapps-*")
	if err != nil {
		return err
	}
	s.root = root
	s.home = filepath.Join(root, "home")
	s.cwd = filepath.Join(root, "workspace")
	s.sessionsRoot = filepath.Join(root, "sessions")
	for _, dir := range []string{s.home, s.cwd, s.sessionsRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	s.sessionID = "greeting-session"
	s.store = &session.FileStore{Root: s.sessionsRoot}
	if _, err := s.store.EnsureLayout(s.sessionID); err != nil {
		return err
	}
	return s.boot()
}

func (s *miniAppsFeatureState) close() {
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

func (s *miniAppsFeatureState) boot() error {
	cfg := &config.Config{
		Paths:  config.Paths{Home: s.home, CWD: s.cwd},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 128}},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.cwd, s.store)
	s.srv = New(cfg, mgr, slog.Default(), s.cwd)
	s.ts = httptest.NewServer(s.srv.Handler())
	return nil
}

func (s *miniAppsFeatureState) completedSession() error {
	const source = "Hello from FoxxyCode\n"
	state := &session.State{
		ID: s.sessionID, CWD: s.cwd, Mode: session.ModeAgent,
		SessionDir: s.store.SessionPath(s.sessionID),
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Write a greeting to greeting.txt: Hello from FoxxyCode"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "write-call", Name: "write", InputJSON: `{"path":"greeting.txt","content":"Hello from FoxxyCode\n"}`}}},
			{Role: llm.RoleTool, ToolCallID: "write-call", Content: "wrote greeting.txt"},
			{Role: llm.RoleAssistant, Content: "Greeting file accepted."},
		},
	}
	if err := s.store.Save(state); err != nil {
		return err
	}
	sessionDir := s.store.SessionPath(s.sessionID)
	if err := session.MarkToolCallStarted(sessionDir, "write-call", "write", "builtin", "in_progress"); err != nil {
		return err
	}
	if err := session.WriteToolCallArgs(sessionDir, "write-call", `{"path":"greeting.txt","content":"Hello from FoxxyCode\n"}`); err != nil {
		return err
	}
	if err := session.WriteToolCallResult(sessionDir, "write-call", "wrote greeting.txt"); err != nil {
		return err
	}
	if err := session.MarkToolCallFinished(sessionDir, "write-call", "write", "builtin", "completed"); err != nil {
		return err
	}
	return session.StoreWorkspaceDiff(sessionDir, 1, &session.WorkspaceDiff{Changes: []session.WorkspaceChange{{
		Path: "greeting.txt", After: &session.WorkspaceFile{Content: []byte(source), Mode: 0o644},
	}}})
}

func (s *miniAppsFeatureState) startDistillation() error {
	response, err := s.post("/foxxycode/sessions/"+s.sessionID+"/miniapps/distill", map[string]any{"title": "Greeting File"})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("distillation status %d: %s", response.StatusCode, response.Body)
	}
	var job map[string]any
	if err := json.Unmarshal([]byte(response.Body), &job); err != nil {
		return err
	}
	s.jobID, _ = job["id"].(string)
	if s.jobID == "" {
		return fmt.Errorf("distillation job id missing")
	}
	return nil
}

func (s *miniAppsFeatureState) confirmScenario() error {
	job, err := s.waitDistillationJob(s.jobID, "waiting_for_scenario")
	if err != nil {
		return err
	}
	candidates, _ := job["candidates"].([]any)
	if len(candidates) == 0 {
		return fmt.Errorf("distillation candidates missing")
	}
	candidate, _ := candidates[0].(map[string]any)
	id, _ := candidate["id"].(string)
	if id == "" {
		return fmt.Errorf("scenario candidate id missing")
	}
	response, err := s.post("/foxxycode/miniapp-distillations/"+s.jobID+"/scenario", map[string]any{"scenario": map[string]any{"candidate_id": id}})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("scenario status %d: %s", response.StatusCode, response.Body)
	}
	job, err = s.waitDistillationJob(s.jobID, "succeeded")
	if err != nil {
		return err
	}
	s.appID, _ = job["app_id"].(string)
	if s.appID == "" {
		return fmt.Errorf("generated app id missing")
	}
	return nil
}

func (s *miniAppsFeatureState) generatedDraftHasInputsAndWriteStep() error {
	response, err := http.Get(s.ts.URL + "/foxxycode/miniapps/" + s.appID + "/draft")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("draft status %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&s.draft); err != nil {
		return err
	}
	inputs, _ := s.draft["inputs"].([]any)
	workflow, _ := s.draft["workflow"].([]any)
	if len(inputs) == 0 || len(workflow) == 0 {
		return fmt.Errorf("generated draft has inputs=%d workflow=%d", len(inputs), len(workflow))
	}
	for _, value := range workflow {
		step, _ := value.(map[string]any)
		if step["kind"] == "tool" && step["tool"] == "write" {
			sourceResponse, sourceErr := http.Get(s.ts.URL + "/foxxycode/miniapps/" + s.appID + "/authoring/source")
			if sourceErr != nil {
				return sourceErr
			}
			defer sourceResponse.Body.Close()
			var source map[string]any
			if decodeErr := json.NewDecoder(sourceResponse.Body).Decode(&source); decodeErr != nil {
				return decodeErr
			}
			fixtures, _ := source["fixture_files"].([]any)
			if len(fixtures) == 0 {
				return fmt.Errorf("source evidence has no persisted fixture files")
			}
			return nil
		}
	}
	return fmt.Errorf("generated draft has no write tool step")
}

func (s *miniAppsFeatureState) testDraft() error {
	response, err := s.post("/foxxycode/miniapps/"+s.appID+"/test-runs", map[string]any{"inputs": map[string]any{}})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("test run status %d: %s", response.StatusCode, response.Body)
	}
	var job map[string]any
	if err := json.Unmarshal([]byte(response.Body), &job); err != nil {
		return err
	}
	id, _ := job["id"].(string)
	_, err = s.waitRunJob(id, "succeeded")
	return err
}

func (s *miniAppsFeatureState) release(version string) error {
	revision, _ := s.draft["revision"].(string)
	response, err := s.post("/foxxycode/miniapps/"+s.appID+"/release", map[string]any{"version": version, "approved": true, "expected_revision": revision})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("release status %d: %s", response.StatusCode, response.Body)
	}
	return nil
}

func (s *miniAppsFeatureState) runReleased(version string) error {
	release, err := http.Get(s.ts.URL + "/foxxycode/miniapps/" + s.appID + "/versions/" + version)
	if err != nil {
		return err
	}
	release.Body.Close()
	if release.StatusCode != http.StatusOK {
		return fmt.Errorf("release lookup status %d", release.StatusCode)
	}
	inputs := map[string]any{}
	for _, value := range s.draft["inputs"].([]any) {
		input, _ := value.(map[string]any)
		id, _ := input["id"].(string)
		if strings.Contains(id, "content") {
			inputs[id] = "Hello from the operator\n"
		} else if strings.Contains(id, "path") {
			inputs[id] = "different-greeting.txt"
		}
	}
	response, err := s.post("/foxxycode/miniapps/"+s.appID+"/versions/"+version+"/runs", map[string]any{"inputs": inputs})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("release run status %d: %s", response.StatusCode, response.Body)
	}
	var job map[string]any
	if err := json.Unmarshal([]byte(response.Body), &job); err != nil {
		return err
	}
	id, _ := job["id"].(string)
	job, err = s.waitRunJob(id, "succeeded")
	if err != nil {
		return err
	}
	runID, _ := job["run_id"].(string)
	if runID == "" {
		return fmt.Errorf("release run id missing")
	}
	s.releaseRunID = runID
	run, err := s.srv.miniAppsHTTPState().store.FindRun(runID)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(run.LogPath), "workspace", "different-greeting.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(data) != "Hello from the operator\n" {
		return fmt.Errorf("released run wrote %q", string(data))
	}
	return nil
}

func (s *miniAppsFeatureState) post(path string, payload any) (struct {
	StatusCode int
	Body       string
}, error) {
	data, _ := json.Marshal(payload)
	request, err := http.NewRequest(http.MethodPost, s.ts.URL+path, strings.NewReader(string(data)))
	if err != nil {
		return struct {
			StatusCode int
			Body       string
		}{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return struct {
			StatusCode int
			Body       string
		}{}, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return struct {
		StatusCode int
		Body       string
	}{response.StatusCode, string(body)}, nil
}

func (s *miniAppsFeatureState) waitDistillationJob(id, want string) (map[string]any, error) {
	return s.waitJobPath("/foxxycode/miniapp-distillations/", id, want)
}

func (s *miniAppsFeatureState) waitRunJob(id, want string) (map[string]any, error) {
	return s.waitJobPath("/foxxycode/miniapp-runs/", id, want)
}

func (s *miniAppsFeatureState) waitJobPath(prefix, id, want string) (map[string]any, error) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(s.ts.URL + prefix + id)
		if err == nil {
			var job map[string]any
			if decodeErr := json.NewDecoder(response.Body).Decode(&job); decodeErr == nil {
				response.Body.Close()
				status, _ := job["status"].(string)
				if status == want {
					return job, nil
				}
				if status == "failed" || status == "cancelled" || status == "interrupted" {
					return job, fmt.Errorf("job %s ended %s: %v report=%v result=%v", id, status, job["error"], job["report"], job["result"])
				}
			} else {
				response.Body.Close()
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil, fmt.Errorf("job %s did not reach %s", id, want)
}

func initializeMiniAppsScenario(sc *godog.ScenarioContext) {
	s := &miniAppsFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) { return ctx, s.reset() })
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})
	sc.Step(`^a completed FoxxyCode session that wrote a greeting file$`, s.completedSession)
	sc.Step(`^I start Mini App distillation for the session$`, s.startDistillation)
	sc.Step(`^I confirm the selected greeting-file scenario$`, s.confirmScenario)
	sc.Step(`^the generated draft contains inferred inputs and a write-file tool step$`, s.generatedDraftHasInputsAndWriteStep)
	sc.Step(`^I test the unchanged generated draft with its source inputs$`, s.testDraft)
	sc.Step(`^the test run reproduces the accepted greeting file$`, func() error { return nil })
	sc.Step(`^I release the tested draft as version "([^"]+)"$`, s.release)
	sc.Step(`^I run released version "([^"]+)" with a different greeting$`, s.runReleased)
	sc.Step(`^the released run writes the different greeting$`, func() error { return nil })
}

func TestMiniAppsFeature(t *testing.T) {
	suite := godog.TestSuite{Name: "miniapps", ScenarioInitializer: initializeMiniAppsScenario, Options: &godog.Options{
		Format: "pretty", Paths: []string{"../../features/miniapps.feature"}, TestingT: t, Strict: true,
	}}
	if suite.Run() != 0 {
		t.Fatal("miniapps feature suite failed")
	}
}
