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
	testJobID, confirmationID     string
	assistantProposal             map[string]any
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
		// The assistant binding resolves a provider; the scenario stubs the
		// factory, so nothing reaches the network.
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

// ensureDraft fetches the generated draft on demand so a scenario that never
// asserts its shape can still edit, release, and run it.
func (s *miniAppsFeatureState) ensureDraft() error {
	if s.draft != nil {
		return nil
	}
	draft, err := s.fetchDraft()
	if err != nil {
		return err
	}
	s.draft = draft
	return nil
}

func (s *miniAppsFeatureState) fetchDraft() (map[string]any, error) {
	response, err := http.Get(s.ts.URL + "/foxxycode/miniapps/" + s.appID + "/draft")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("draft status %d", response.StatusCode)
	}
	draft := map[string]any{}
	if err := json.NewDecoder(response.Body).Decode(&draft); err != nil {
		return nil, err
	}
	return draft, nil
}

// addConfirmationCheckpoint is the author edit an operator makes in the editor
// when a generated workflow should pause before it touches anything.
func (s *miniAppsFeatureState) addConfirmationCheckpoint() error {
	if err := s.ensureDraft(); err != nil {
		return err
	}
	revision, _ := s.draft["revision"].(string)
	if revision == "" {
		return fmt.Errorf("draft revision missing")
	}
	workflow, _ := s.draft["workflow"].([]any)
	checkpoint := map[string]any{
		"id": "approve-write", "kind": "confirm", "title": "Approve the write",
		"message": "Write the greeting file?",
	}
	s.draft["workflow"] = append([]any{checkpoint}, workflow...)
	response, err := s.do(http.MethodPut, "/foxxycode/miniapps/"+s.appID+"/draft", s.draft,
		map[string]string{"If-Match": revision})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("draft update status %d: %s", response.StatusCode, response.Body)
	}
	s.draft = nil
	return json.Unmarshal([]byte(response.Body), &s.draft)
}

// askAssistantForProjectInput stubs the model so the scenario stays offline and
// deterministic, then drives the real editor-assistant round trip.
func (s *miniAppsFeatureState) askAssistantForProjectInput() error {
	if err := s.ensureDraft(); err != nil {
		return err
	}
	proposed := map[string]any{}
	for key, value := range s.draft {
		proposed[key] = value
	}
	inputs, _ := s.draft["inputs"].([]any)
	proposed["inputs"] = append(append([]any{}, inputs...), map[string]any{
		"id": "project", "type": "string", "title": "Project", "required": false,
		"ui": map[string]any{"control": "text"},
	})
	reply, err := json.Marshal(map[string]any{
		"reply":   "Added a project input.",
		"changes": []string{"Added the project input"},
		"draft":   proposed,
	})
	if err != nil {
		return err
	}
	s.srv.miniAppsHTTPState().assistant.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) {
		return &httpMiniAppAssistantTestProvider{content: string(reply)}, nil
	})

	response, err := s.post("/foxxycode/miniapps/"+s.appID+"/assistant", map[string]any{
		"message": "Add a project input",
		"draft":   s.draft,
	})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("assistant status %d: %s", response.StatusCode, response.Body)
	}
	s.assistantProposal = map[string]any{}
	return json.Unmarshal([]byte(response.Body), &s.assistantProposal)
}

func (s *miniAppsFeatureState) assistantProposedWithoutSaving() error {
	if strings.TrimSpace(fmt.Sprint(s.assistantProposal["reply"])) == "" {
		return fmt.Errorf("assistant returned no reply: %v", s.assistantProposal)
	}
	proposed, _ := s.assistantProposal["draft"].(map[string]any)
	if !miniAppsDraftHasInput(proposed, "project") {
		return fmt.Errorf("proposal does not carry the project input: %v", proposed)
	}
	stored, err := s.fetchDraft()
	if err != nil {
		return err
	}
	if miniAppsDraftHasInput(stored, "project") {
		return fmt.Errorf("the assistant saved its proposal")
	}
	return nil
}

func (s *miniAppsFeatureState) saveProposedDraft() error {
	proposed, _ := s.assistantProposal["draft"].(map[string]any)
	if proposed == nil {
		return fmt.Errorf("there is no assistant proposal to save")
	}
	revision, _ := proposed["revision"].(string)
	response, err := s.do(http.MethodPut, "/foxxycode/miniapps/"+s.appID+"/draft", proposed,
		map[string]string{"If-Match": revision})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("draft update status %d: %s", response.StatusCode, response.Body)
	}
	s.draft = nil
	return nil
}

func (s *miniAppsFeatureState) storedDraftCarriesProjectInput() error {
	stored, err := s.fetchDraft()
	if err != nil {
		return err
	}
	if !miniAppsDraftHasInput(stored, "project") {
		return fmt.Errorf("stored draft has no project input: %v", stored["inputs"])
	}
	return nil
}

func miniAppsDraftHasInput(draft map[string]any, id string) bool {
	inputs, _ := draft["inputs"].([]any)
	for _, value := range inputs {
		input, _ := value.(map[string]any)
		if input["id"] == id {
			return true
		}
	}
	return false
}

func (s *miniAppsFeatureState) startTestRun() error {
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
	if s.testJobID == "" {
		return fmt.Errorf("test run job id missing")
	}
	return nil
}

func (s *miniAppsFeatureState) testRunWaitsForApproval() error {
	job, err := s.waitRunJob(s.testJobID, "waiting_for_confirmation")
	if err != nil {
		return err
	}
	confirmation, _ := job["confirmation"].(map[string]any)
	s.confirmationID, _ = confirmation["id"].(string)
	if s.confirmationID == "" {
		return fmt.Errorf("waiting run exposed no confirmation id: %v", job["confirmation"])
	}
	return nil
}

func (s *miniAppsFeatureState) approvePendingConfirmation() error {
	response, err := s.post("/foxxycode/miniapp-runs/"+s.testJobID+"/confirmation",
		map[string]any{"approved": true, "confirmation_id": s.confirmationID})
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("confirmation status %d: %s", response.StatusCode, response.Body)
	}
	return nil
}

func (s *miniAppsFeatureState) testRunFinishesSuccessfully() error {
	_, err := s.waitRunJob(s.testJobID, "succeeded")
	return err
}

func (s *miniAppsFeatureState) catalogListsReleasedVersion(version string) error {
	response, err := http.Get(s.ts.URL + "/foxxycode/miniapps")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("catalog status %d", response.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return err
	}
	items, _ := body["apps"].([]any)
	if items == nil {
		items, _ = body["items"].([]any)
	}
	for _, value := range items {
		entry, _ := value.(map[string]any)
		if entry["id"] != s.appID {
			continue
		}
		if entry["state"] != "released" || entry["version"] != version {
			return fmt.Errorf("catalog entry state=%v version=%v", entry["state"], entry["version"])
		}
		return nil
	}
	return fmt.Errorf("catalog does not list %q", s.appID)
}

func (s *miniAppsFeatureState) runHistoryListsRun() error {
	response, err := http.Get(s.ts.URL + "/foxxycode/miniapps/" + s.appID + "/runs")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("run history status %d", response.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return err
	}
	items, _ := body["items"].([]any)
	for _, value := range items {
		run, _ := value.(map[string]any)
		if run["id"] == s.releaseRunID {
			return nil
		}
	}
	return fmt.Errorf("run history does not list %q", s.releaseRunID)
}

func (s *miniAppsFeatureState) retestAndRelease(version string) error {
	// A release is bound to a passing test for the current revision, so the
	// second version is tested again before it is cut.
	if err := s.testDraft(); err != nil {
		return err
	}
	return s.release(version)
}

func (s *miniAppsFeatureState) bothReleasesRetrievable() error {
	for _, version := range []string{"1.0.0", "1.1.0"} {
		response, err := http.Get(s.ts.URL + "/foxxycode/miniapps/" + s.appID + "/versions/" + version)
		if err != nil {
			return err
		}
		var released map[string]any
		decodeErr := json.NewDecoder(response.Body).Decode(&released)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("release %s status %d", version, response.StatusCode)
		}
		if decodeErr != nil {
			return decodeErr
		}
		if released["version"] != version || released["state"] != "released" {
			return fmt.Errorf("release %s reports version=%v state=%v", version, released["version"], released["state"])
		}
	}
	return nil
}

func (s *miniAppsFeatureState) release(version string) error {
	if err := s.ensureDraft(); err != nil {
		return err
	}
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
	if err := s.ensureDraft(); err != nil {
		return err
	}
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

type miniAppsResponse struct {
	StatusCode int
	Body       string
}

func (s *miniAppsFeatureState) post(path string, payload any) (miniAppsResponse, error) {
	return s.do(http.MethodPost, path, payload, nil)
}

func (s *miniAppsFeatureState) do(method, path string, payload any, headers map[string]string) (miniAppsResponse, error) {
	data, _ := json.Marshal(payload)
	request, err := http.NewRequest(method, s.ts.URL+path, strings.NewReader(string(data)))
	if err != nil {
		return miniAppsResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return miniAppsResponse{}, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return miniAppsResponse{response.StatusCode, string(body)}, nil
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
	sc.Step(`^I add a confirmation checkpoint to the generated draft$`, s.addConfirmationCheckpoint)
	sc.Step(`^I start a test run of the draft$`, s.startTestRun)
	sc.Step(`^the test run waits for my approval$`, s.testRunWaitsForApproval)
	sc.Step(`^I approve the pending confirmation$`, s.approvePendingConfirmation)
	sc.Step(`^the test run finishes successfully$`, s.testRunFinishesSuccessfully)
	sc.Step(`^the catalog lists the Mini App as released at version "([^"]+)"$`, s.catalogListsReleasedVersion)
	sc.Step(`^the run history for the Mini App lists that run$`, s.runHistoryListsRun)
	sc.Step(`^I retest and release the draft as version "([^"]+)"$`, s.retestAndRelease)
	sc.Step(`^both released versions stay retrievable$`, s.bothReleasesRetrievable)
	sc.Step(`^I ask the editor assistant to add a project input$`, s.askAssistantForProjectInput)
	sc.Step(`^the assistant answers with a proposal and leaves the draft untouched$`, s.assistantProposedWithoutSaving)
	sc.Step(`^I save the proposed draft$`, s.saveProposedDraft)
	sc.Step(`^the stored draft carries the project input$`, s.storedDraftCarriesProjectInput)
}

func TestMiniAppsFeature(t *testing.T) {
	suite := godog.TestSuite{Name: "miniapps", ScenarioInitializer: initializeMiniAppsScenario, Options: &godog.Options{
		Format: "pretty", Paths: []string{"../../features/miniapps.feature"}, TestingT: t, Strict: true,
	}}
	if suite.Run() != 0 {
		t.Fatal("miniapps feature suite failed")
	}
}
