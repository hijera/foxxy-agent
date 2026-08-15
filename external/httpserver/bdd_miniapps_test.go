//go:build http && miniapps

package httpserver

import (
	"bytes"
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

	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type miniAppsFeatureState struct {
	root      string
	server    *Server
	http      *httptest.Server
	appID     string
	status    int
	response  []byte
	lastRun   miniapps.Run
	sessionID string
}

type miniAppsBDDProvider struct{}

func (p *miniAppsBDDProvider) Complete(_ context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	prompt := ""
	if len(messages) > 0 {
		prompt = messages[len(messages)-1].Content
	}
	if len(tools) > 0 {
		if len(messages) > 0 && messages[len(messages)-1].Role == llm.RoleTool {
			return &llm.Response{
				Content:    "Added the style input and decoration step.",
				StopReason: "end_turn",
			}, nil
		}
		inputJSON, _ := json.Marshal(map[string]any{
			"input": miniapps.Input{
				ID: "style", Type: "string", Title: "Style",
				UI: miniapps.InputUI{Control: "text", Order: 20},
			},
		})
		stepJSON, _ := json.Marshal(map[string]any{
			"step": miniapps.Step{
				ID: "decorate", Kind: "program", Title: "Apply style",
				Language: miniapps.VMVersion, Entry: "main",
				Functions: map[string][]miniapps.Instruction{"main": {
					{Op: "ref.get", Arg: "inputs.style"},
					{Op: "return"},
				}},
				Limits: miniapps.ProgramLimits{Instructions: 100, StackDepth: 16, CallDepth: 4},
			},
		})
		return &llm.Response{
			ToolCalls: []llm.ToolCall{
				{ID: "tool-input", Name: "miniapp_upsert_input", InputJSON: string(inputJSON)},
				{ID: "tool-step", Name: "miniapp_upsert_step", InputJSON: string(stepJSON)},
			},
			StopReason: "tool_use",
		}, nil
	}
	content := `{"passed":true,"reason":"The supplied name is present."}`
	if strings.Contains(prompt, "Create a concise, reusable result contract") {
		content = `{"expected_result":"A friendly greeting using the supplied name.","acceptance_criterion":"The result is friendly and contains the supplied name."}`
	}
	return &llm.Response{Content: content, StopReason: "end_turn"}, nil
}

func (p *miniAppsBDDProvider) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	response, err := p.Complete(ctx, messages, tools)
	if err == nil {
		onChunk(llm.StreamChunk{TextDelta: response.Content, StopReason: response.StopReason})
	}
	return response, err
}

func (s *miniAppsFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-miniapps-*")
	if err != nil {
		return err
	}
	s.root = root
	s.sessionID = "session-greeting"
	return nil
}

func (s *miniAppsFeatureState) close() {
	if s.http != nil {
		s.http.Close()
		s.http = nil
	}
	if s.server != nil {
		s.server.Drain()
		s.server = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

func (s *miniAppsFeatureState) completedSession() error {
	home := filepath.Join(s.root, "home")
	sessionsRoot := filepath.Join(home, "sessions")
	cwd := filepath.Join(s.root, "workspace")
	for _, dir := range []string{home, sessionsRoot, cwd} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	store := &session.FileStore{Root: sessionsRoot}
	sessionDir, err := store.EnsureLayout(s.sessionID)
	if err != nil {
		return err
	}
	state := &session.State{
		ID: s.sessionID, CWD: cwd, Mode: session.ModeAgent, SessionDir: sessionDir,
		TitlePinned: "Greeting formatter",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Format a friendly greeting for Foxxy."},
			{Role: llm.RoleAssistant, Content: "Hello, Foxxy!"},
		},
	}
	if err := store.Save(state); err != nil {
		return err
	}
	cfg := &config.Config{
		Paths: config.Paths{Home: home, CWD: cwd},
		Providers: []config.ProviderConfig{{
			Name: "fake", Type: "openai", APIBase: "https://fake.invalid/v1", APIKey: "test",
		}},
		Models:   []config.ModelEntry{{Model: "fake/reviewed-model"}},
		Agent:    config.Agent{Model: "fake/reviewed-model"},
		Sessions: config.Sessions{Dir: sessionsRoot},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	manager := session.NewManager(cfg, noopSender{}, runner, slog.Default(), cwd, store)
	s.server = New(cfg, manager, slog.Default(), cwd)
	provider := &miniAppsBDDProvider{}
	s.server.agentProviderFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}
	s.http = httptest.NewServer(s.server.Handler())
	return nil
}

func (s *miniAppsFeatureState) request(method, path string, body any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, s.http.URL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	s.status = response.StatusCode
	s.response, _ = io.ReadAll(response.Body)
	return nil
}

func (s *miniAppsFeatureState) distillSession() error {
	if err := s.request(http.MethodPost, "/foxxycode/sessions/"+s.sessionID+"/miniapps/distill", nil); err != nil {
		return err
	}
	if s.status != http.StatusAccepted {
		return fmt.Errorf("distill status %d: %s", s.status, s.response)
	}
	var job miniapps.DistillationJob
	if err := json.Unmarshal(s.response, &job); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := s.request(http.MethodGet, "/foxxycode/miniapp-distillations/"+job.ID, nil); err != nil {
			return err
		}
		if err := json.Unmarshal(s.response, &job); err != nil {
			return err
		}
		if job.Status == miniapps.DistillationCompleted {
			s.appID = job.AppID
			return nil
		}
		if job.Status == miniapps.DistillationFailed {
			return fmt.Errorf("distillation failed: %s", job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("distillation did not complete")
}

func (s *miniAppsFeatureState) draftCreated() error {
	if s.appID == "" {
		return fmt.Errorf("distillation returned no app id")
	}
	if err := s.request(http.MethodGet, "/foxxycode/miniapps/"+s.appID+"/draft", nil); err != nil {
		return err
	}
	if s.status != http.StatusOK {
		return fmt.Errorf("draft status %d: %s", s.status, s.response)
	}
	var app miniapps.MiniApp
	if err := json.Unmarshal(s.response, &app); err != nil {
		return err
	}
	if app.State != miniapps.StateDraft || len(app.Workflow) == 0 {
		return fmt.Errorf("unexpected draft: %#v", app)
	}
	return nil
}

func (s *miniAppsFeatureState) replaceDraft() error {
	app := deterministicGreetingApp(s.appID)
	if err := s.request(http.MethodPut, "/foxxycode/miniapps/"+s.appID+"/draft", app); err != nil {
		return err
	}
	if s.status != http.StatusOK {
		return fmt.Errorf("put draft status %d: %s", s.status, s.response)
	}
	return nil
}

func (s *miniAppsFeatureState) testWithName(name string) error {
	if err := s.request(http.MethodPost, "/foxxycode/miniapps/"+s.appID+"/test-runs",
		map[string]any{"inputs": map[string]any{"name": name}}); err != nil {
		return err
	}
	if err := json.Unmarshal(s.response, &s.lastRun); err != nil {
		return err
	}
	return nil
}

func (s *miniAppsFeatureState) generateExpectedResult(expectations string) error {
	if err := s.request(
		http.MethodPost,
		"/foxxycode/miniapps/"+s.appID+"/expected-result",
		map[string]any{"expectations": expectations},
	); err != nil {
		return err
	}
	if s.status != http.StatusOK {
		return fmt.Errorf("expected-result status %d: %s", s.status, s.response)
	}
	return nil
}

func (s *miniAppsFeatureState) draftHasExpectedResult() error {
	var generation miniapps.ExpectedResultGeneration
	if err := json.Unmarshal(s.response, &generation); err != nil {
		return err
	}
	app := generation.App
	if app.Success.ExpectedResult == "" || app.Success.AcceptanceCriterion == "" {
		return fmt.Errorf("expected-result contract is incomplete: %#v", app.Success)
	}
	for _, check := range app.Success.Checks {
		if check.Kind == "prompt" && check.ModelBinding != "" {
			return nil
		}
	}
	return fmt.Errorf("draft has no model-verified prompt check: %#v", app.Success.Checks)
}

func (s *miniAppsFeatureState) selectLogicalModel(model string) error {
	if err := s.request(
		http.MethodPost,
		"/foxxycode/miniapps/"+s.appID+"/model-binding",
		map[string]any{"model_ref": model},
	); err != nil {
		return err
	}
	if s.status != http.StatusOK {
		return fmt.Errorf("model-binding status %d: %s", s.status, s.response)
	}
	return nil
}

func (s *miniAppsFeatureState) draftUsesLogicalModel(model string) error {
	var app miniapps.MiniApp
	if err := json.Unmarshal(s.response, &app); err != nil {
		return err
	}
	for _, binding := range app.Requirements.ModelBindings {
		if binding.ID == "primary" && binding.LogicalModel == model {
			return nil
		}
	}
	return fmt.Errorf("primary logical model %q was not saved: %#v", model, app.Requirements.ModelBindings)
}

func (s *miniAppsFeatureState) askAuthoringAssistant() error {
	if err := s.request(
		http.MethodPost,
		"/foxxycode/miniapps/"+s.appID+"/authoring/chat",
		map[string]any{"message": "Add a style input and a decoration step."},
	); err != nil {
		return err
	}
	if s.status != http.StatusOK {
		return fmt.Errorf("authoring chat status %d: %s", s.status, s.response)
	}
	return nil
}

func (s *miniAppsFeatureState) assistantToolEditsSaved() error {
	var result miniapps.AuthoringResult
	if err := json.Unmarshal(s.response, &result); err != nil {
		return err
	}
	if result.Message == "" || len(result.Operations) != 2 {
		return fmt.Errorf("unexpected authoring result: %#v", result)
	}
	hasInput, hasStep := false, false
	for _, input := range result.App.Inputs {
		hasInput = hasInput || input.ID == "style"
	}
	for _, step := range result.App.Workflow {
		hasStep = hasStep || step.ID == "decorate"
	}
	if !hasInput || !hasStep {
		return fmt.Errorf("assistant edits are missing: inputs=%#v workflow=%#v", result.App.Inputs, result.App.Workflow)
	}
	stored, err := s.server.miniAppStore().GetDraft(s.appID)
	if err != nil {
		return err
	}
	if stored.Revision != result.App.Revision {
		return fmt.Errorf("assistant result was not saved")
	}
	return nil
}

func (s *miniAppsFeatureState) runHasText(text string) error {
	if s.status != http.StatusOK {
		return fmt.Errorf("run status %d: %s", s.status, s.response)
	}
	if s.lastRun.Status != miniapps.RunSucceeded || s.lastRun.Outputs["text"] != text {
		return fmt.Errorf("run = %#v, want %q", s.lastRun, text)
	}
	return nil
}

func (s *miniAppsFeatureState) releaseDraft() error {
	if err := s.request(http.MethodPost, "/foxxycode/miniapps/"+s.appID+"/release",
		map[string]any{"version": "1.0.0"}); err != nil {
		return err
	}
	if s.status != http.StatusCreated {
		return fmt.Errorf("release status %d: %s", s.status, s.response)
	}
	return nil
}

func (s *miniAppsFeatureState) runRelease(version, name string) error {
	if err := s.request(http.MethodPost,
		"/foxxycode/miniapps/"+s.appID+"/versions/"+version+"/runs",
		map[string]any{"inputs": map[string]any{"name": name}}); err != nil {
		return err
	}
	return json.Unmarshal(s.response, &s.lastRun)
}

func deterministicGreetingApp(id string) miniapps.MiniApp {
	return miniapps.MiniApp{
		SchemaVersion: miniapps.SchemaVersion, Kind: miniapps.KindMiniApp,
		ID: id, State: miniapps.StateDraft,
		Metadata: miniapps.Metadata{
			Name: "Greeting formatter", Description: "Formats a friendly greeting.",
			Goal: "Return a greeting for the supplied name.",
		},
		Inputs: []miniapps.Input{{
			ID: "name", Type: "string", Title: "Name", Required: true,
			UI: miniapps.InputUI{Control: "text", Order: 10},
		}},
		Workflow: []miniapps.Step{{
			ID: "format", Kind: "program", Title: "Format greeting",
			Language: miniapps.VMVersion, Entry: "main",
			Functions: map[string][]miniapps.Instruction{"main": {
				{Op: "const", Arg: "Hello, "},
				{Op: "ref.get", Arg: "inputs.name"},
				{Op: "string.concat"},
				{Op: "const", Arg: "!"},
				{Op: "string.concat"},
				{Op: "return"},
			}},
			Limits: miniapps.ProgramLimits{Instructions: 100, StackDepth: 16, CallDepth: 4},
		}},
		Success: miniapps.SuccessSpec{Mode: "all", Checks: []miniapps.SuccessCheck{{
			Kind: "step", Step: "format", Status: "succeeded",
		}}},
		Outputs: []miniapps.Output{{
			ID: "text", Type: "text",
			Value: miniapps.Ref{Ref: "steps.format.outputs.result"}, Renderer: "text",
		}},
		Runtime: miniapps.RuntimePolicy{
			LogScope: "global", OperatorEventLevel: "status",
			DiagnosticToolEvents: "sanitized", PersistAgentReasoning: false,
		},
	}
}

func initializeMiniAppsScenario(scenario *godog.ScenarioContext) {
	state := &miniAppsFeatureState{}
	scenario.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, state.reset()
	})
	scenario.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		state.close()
		return ctx, nil
	})

	scenario.Step(`^a completed FoxxyCode session about formatting a greeting$`, state.completedSession)
	scenario.Step(`^I distill the session into a mini app$`, state.distillSession)
	scenario.Step(`^an editable mini app draft is created$`, state.draftCreated)
	scenario.Step(`^I replace the draft with a deterministic greeting workflow$`, state.replaceDraft)
	scenario.Step(`^I generate an expected result for "([^"]+)"$`, state.generateExpectedResult)
	scenario.Step(`^the draft contains a model-verified expected result$`, state.draftHasExpectedResult)
	scenario.Step(`^I select logical model "([^"]+)" for the mini app$`, state.selectLogicalModel)
	scenario.Step(`^the draft uses logical model "([^"]+)" for model steps$`, state.draftUsesLogicalModel)
	scenario.Step(`^I ask the mini app authoring assistant to add a style input and decoration step$`, state.askAuthoringAssistant)
	scenario.Step(`^the assistant tool edits are saved in the draft$`, state.assistantToolEditsSaved)
	scenario.Step(`^I test the draft with the name "([^"]+)"$`, state.testWithName)
	scenario.Step(`^the test run succeeds with the text "([^"]+)"$`, state.runHasText)
	scenario.Step(`^I release the tested draft$`, state.releaseDraft)
	scenario.Step(`^I run released version "([^"]+)" with the name "([^"]+)"$`, state.runRelease)
	scenario.Step(`^the released run succeeds with the text "([^"]+)"$`, state.runHasText)
}

func TestMiniAppsFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "miniapps", ScenarioInitializer: initializeMiniAppsScenario,
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/miniapps.feature"},
			TestingT: t, Strict: true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("miniapps feature suite failed")
	}
}
