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
	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type miniAppsCommandsState struct {
	root, home, cwd, sessionsRoot string
	ts                            *httptest.Server
	srv                           *Server
	store                         *session.FileStore
	fake                          cmdtest.Fake
	sessionID, jobID, appID       string
	draft                         map[string]any
	testJobID                     string
}

func (s *miniAppsCommandsState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-cmd-*")
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
	s.sessionID = "encoder-session"
	s.store = &session.FileStore{Root: s.sessionsRoot}
	if _, err := s.store.EnsureLayout(s.sessionID); err != nil {
		return err
	}
	fake, err := cmdtest.Build(filepath.Join(root, "bin"), "fakeenc")
	if err != nil {
		return err
	}
	s.fake = fake
	// The fake binary reads its log path from the environment inherited by the
	// server's child processes; the suite runs sequentially in one process.
	if err := os.Setenv(cmdtest.EnvLog, fake.Log); err != nil {
		return err
	}
	if err := os.Setenv(cmdtest.EnvStdout, "The clip is converted."); err != nil {
		return err
	}
	return s.boot()
}

func (s *miniAppsCommandsState) close() {
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

func (s *miniAppsCommandsState) boot() error {
	cfg := &config.Config{
		Paths:     config.Paths{Home: s.home, CWD: s.cwd},
		Agent:     config.Agent{Model: "openai/gpt-4o"},
		Models:    []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 128}},
		Providers: []config.ProviderConfig{{Name: "openai", Type: "openai", APIKey: "test"}},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.cwd, s.store)
	s.srv = New(cfg, mgr, slog.Default(), s.cwd)
	s.ts = httptest.NewServer(s.srv.Handler())
	return nil
}

// declareProfile injects the fake-encoder profile into the live config, the
// same way an operator's commands: section would.
func (s *miniAppsCommandsState) declareProfile() error {
	cfg := s.srv.activeCfg()
	cfg.Commands = []cmdprofile.ProfileSpec{{
		Name: "fakeenc_convert", Binary: s.fake.Binary, Permission: "allow",
		Template: []string{"-i", "{input_path}", "-mode", "{mode}", "{output_path}"},
		Params: []cmdprofile.ParamSpec{
			{Name: "input_path", Type: cmdprofile.ParamFile},
			{Name: "mode", Type: cmdprofile.ParamEnum, Enum: []string{"fast", "best"}},
			{Name: "output_path", Type: cmdprofile.ParamFile},
		},
	}}
	return nil
}

func (s *miniAppsCommandsState) completedCommandSession() error {
	argsJSON := `{"command":"fakeenc -i source-clip.mp4 -mode fast out-clip.mp3"}`
	state := &session.State{
		ID: s.sessionID, CWD: s.cwd, Mode: session.ModeAgent,
		SessionDir: s.store.SessionPath(s.sessionID),
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Convert source-clip.mp4 to out-clip.mp3 with the fast mode"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "enc-call", Name: "run_command", InputJSON: argsJSON}}},
			{Role: llm.RoleTool, ToolCallID: "enc-call", Content: "converted"},
			{Role: llm.RoleAssistant, Content: "The clip is converted."},
		},
	}
	if err := s.store.Save(state); err != nil {
		return err
	}
	sessionDir := s.store.SessionPath(s.sessionID)
	if err := session.MarkToolCallStarted(sessionDir, "enc-call", "run_command", "builtin", "in_progress"); err != nil {
		return err
	}
	if err := session.WriteToolCallArgs(sessionDir, "enc-call", argsJSON); err != nil {
		return err
	}
	if err := session.WriteToolCallResult(sessionDir, "enc-call", "converted"); err != nil {
		return err
	}
	return session.MarkToolCallFinished(sessionDir, "enc-call", "run_command", "builtin", "completed")
}

func (s *miniAppsCommandsState) startDistillation() error {
	response, err := s.post("/foxxycode/sessions/"+s.sessionID+"/miniapps/distill", map[string]any{"title": "Fake Encoder"})
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

func (s *miniAppsCommandsState) confirmScenario() error {
	job, err := s.waitJobPath("/foxxycode/miniapp-distillations/", s.jobID, "waiting_for_scenario")
	if err != nil {
		return err
	}
	candidates, _ := job["candidates"].([]any)
	if len(candidates) == 0 {
		return fmt.Errorf("no scenario candidates")
	}
	candidate, _ := candidates[0].(map[string]any)
	id, _ := candidate["id"].(string)
	response, err := s.post("/foxxycode/miniapp-distillations/"+s.jobID+"/scenario", map[string]any{"scenario": map[string]any{"candidate_id": id}})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("scenario status %d: %s", response.StatusCode, response.Body)
	}
	job, err = s.waitJobPath("/foxxycode/miniapp-distillations/", s.jobID, "succeeded")
	if err != nil {
		return err
	}
	s.appID, _ = job["app_id"].(string)
	if s.appID == "" {
		return fmt.Errorf("generated app id missing")
	}
	return nil
}

func (s *miniAppsCommandsState) draftCarriesCommandStep() error {
	response, err := http.Get(s.ts.URL + "/foxxycode/miniapps/" + s.appID + "/draft")
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("draft status %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&s.draft); err != nil {
		return err
	}
	workflow, _ := s.draft["workflow"].([]any)
	foundStep := false
	for _, value := range workflow {
		step, _ := value.(map[string]any)
		if step["tool"] == "cmd_fakeenc_convert" {
			foundStep = true
		}
		if step["tool"] == "run_command" {
			return fmt.Errorf("draft still carries a raw run_command step")
		}
	}
	if !foundStep {
		return fmt.Errorf("draft workflow has no command-profile step: %v", workflow)
	}
	requirements, _ := s.draft["requirements"].(map[string]any)
	commands, _ := requirements["commands"].([]any)
	if len(commands) != 1 {
		return fmt.Errorf("requirements.commands = %v", commands)
	}
	profile, _ := commands[0].(map[string]any)
	if profile["name"] != "fakeenc_convert" {
		return fmt.Errorf("embedded profile = %v", profile)
	}
	return nil
}

func (s *miniAppsCommandsState) testDraft() error {
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
	s.testJobID, _ = job["id"].(string)
	return nil
}

func (s *miniAppsCommandsState) testRunSucceededWithoutShell() error {
	if _, err := s.waitJobPath("/foxxycode/miniapp-runs/", s.testJobID, "succeeded"); err != nil {
		return err
	}
	calls, err := s.fake.Calls()
	if err != nil || len(calls) == 0 {
		return fmt.Errorf("fake encoder calls = %v, err %v", calls, err)
	}
	last := calls[len(calls)-1]
	if strings.Join(last.Args, " ") != "-i source-clip.mp4 -mode fast out-clip.mp3" {
		return fmt.Errorf("recorded argv = %v", last.Args)
	}
	return nil
}

func (s *miniAppsCommandsState) release(version string) error {
	revision, _ := s.draft["revision"].(string)
	response, err := s.post("/foxxycode/miniapps/"+s.appID+"/release", map[string]any{
		"version": version, "approved": true, "expected_revision": revision,
	})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("release status %d: %s", response.StatusCode, response.Body)
	}
	return nil
}

func (s *miniAppsCommandsState) runReleased(version string) error {
	inputs := map[string]any{}
	inputList, _ := s.draft["inputs"].([]any)
	for _, value := range inputList {
		input, _ := value.(map[string]any)
		id, _ := input["id"].(string)
		switch {
		case strings.Contains(id, "input_path"):
			inputs[id] = "another-take.mp4"
		case strings.Contains(id, "output_path"):
			inputs[id] = "another-take.mp3"
		case strings.Contains(id, "mode"):
			inputs[id] = "best"
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
	if _, err := s.waitJobPath("/foxxycode/miniapp-runs/", id, "succeeded"); err != nil {
		return err
	}
	return nil
}

func (s *miniAppsCommandsState) releasedRunUsedTheNewFile() error {
	calls, err := s.fake.Calls()
	if err != nil || len(calls) == 0 {
		return fmt.Errorf("fake encoder calls = %v, err %v", calls, err)
	}
	last := calls[len(calls)-1]
	joined := strings.Join(last.Args, " ")
	if !strings.Contains(joined, "another-take.mp4") || !strings.Contains(joined, "-mode best") {
		return fmt.Errorf("released run argv = %v", last.Args)
	}
	return nil
}

// dropConfigProfile removes the config declaration after distillation, so the
// draft's embedded profile is the only one left — the portability situation a
// receiving machine is in.
func (s *miniAppsCommandsState) dropConfigProfile() error {
	// The bare binary name must still resolve, as it would on a machine where
	// the tool is installed on PATH.
	if err := os.Setenv("PATH", filepath.Dir(s.fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		return err
	}
	s.srv.activeCfg().Commands = nil
	return nil
}

func (s *miniAppsCommandsState) testRunPausesForTrust() error {
	job, err := s.waitJobPath("/foxxycode/miniapp-runs/", s.testJobID, "waiting_for_confirmation")
	if err != nil {
		return err
	}
	confirmation, _ := job["confirmation"].(map[string]any)
	details, _ := confirmation["details"].(map[string]any)
	if details["kind"] != "command_profile" || details["name"] != "fakeenc_convert" {
		return fmt.Errorf("confirmation details = %v", confirmation)
	}
	if calls, _ := s.fake.Calls(); len(calls) != 0 {
		return fmt.Errorf("the binary ran before trust: %v", calls)
	}
	return nil
}

func (s *miniAppsCommandsState) approveTrustConfirmation() error {
	response, err := s.post("/foxxycode/miniapp-runs/"+s.testJobID+"/confirmation", map[string]any{"approved": true})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("confirmation status %d: %s", response.StatusCode, response.Body)
	}
	return nil
}

func (s *miniAppsCommandsState) trustApprovalRecorded() error {
	trustPath := filepath.Join(s.home, cmdprofile.TrustFileName)
	if _, err := os.Stat(trustPath); err != nil {
		return fmt.Errorf("trust file missing: %w", err)
	}
	return nil
}

func (s *miniAppsCommandsState) post(path string, payload any) (miniAppsResponse, error) {
	data, _ := json.Marshal(payload)
	request, err := http.NewRequest(http.MethodPost, s.ts.URL+path, strings.NewReader(string(data)))
	if err != nil {
		return miniAppsResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return miniAppsResponse{}, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return miniAppsResponse{}, err
	}
	return miniAppsResponse{response.StatusCode, string(body)}, nil
}

func (s *miniAppsCommandsState) waitJobPath(prefix, id, want string) (map[string]any, error) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(s.ts.URL + prefix + id)
		if err == nil {
			var job map[string]any
			decodeErr := json.NewDecoder(response.Body).Decode(&job)
			_ = response.Body.Close()
			if decodeErr == nil {
				status, _ := job["status"].(string)
				if status == want {
					return job, nil
				}
				if status == "failed" || status == "cancelled" || status == "interrupted" {
					return job, fmt.Errorf("job %s ended %s: %v result=%v", id, status, job["error"], job["result"])
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil, fmt.Errorf("job %s did not reach %s", id, want)
}

func initializeMiniAppsCommandsScenario(sc *godog.ScenarioContext) {
	s := &miniAppsCommandsState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) { return ctx, s.reset() })
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})
	sc.Step(`^a command profile for the fake encoder is declared in the config$`, s.declareProfile)
	sc.Step(`^a completed session that ran the fake encoder over a media file$`, s.completedCommandSession)
	sc.Step(`^I start Mini App distillation for the command session$`, s.startDistillation)
	sc.Step(`^I confirm the selected command scenario$`, s.confirmScenario)
	sc.Step(`^the draft carries a command step and embeds its profile$`, s.draftCarriesCommandStep)
	sc.Step(`^I test the command draft with its source inputs$`, s.testDraft)
	sc.Step(`^the command test run succeeds and the fake encoder ran without a shell$`, s.testRunSucceededWithoutShell)
	sc.Step(`^I release the command draft as version "([^"]+)"$`, s.release)
	sc.Step(`^I run released command version "([^"]+)" with a different media file$`, s.runReleased)
	sc.Step(`^the released command run executed the fake encoder with the new file$`, s.releasedRunUsedTheNewFile)
	sc.Step(`^the config no longer declares the fake encoder profile$`, s.dropConfigProfile)
	sc.Step(`^the test run pauses asking to trust the embedded profile$`, s.testRunPausesForTrust)
	sc.Step(`^I approve the trust confirmation$`, s.approveTrustConfirmation)
	sc.Step(`^the trust approval is recorded on this machine$`, s.trustApprovalRecorded)
}

func TestMiniAppsCommandsFeature(t *testing.T) {
	suite := godog.TestSuite{Name: "miniapp-commands", ScenarioInitializer: initializeMiniAppsCommandsScenario, Options: &godog.Options{
		Format: "pretty", Paths: []string{"../../features/miniapp_commands.feature"}, TestingT: t, Strict: true,
	}}
	if suite.Run() != 0 {
		t.Fatal("miniapp commands feature suite failed")
	}
}
