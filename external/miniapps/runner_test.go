//go:build miniapps

package miniapps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type runnerTestStore struct {
	mu   sync.Mutex
	apps map[string]MiniApp
	runs []Run
}

func (s *runnerTestStore) GetDraft(id string) (MiniApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	app, ok := s.apps[id]
	if !ok {
		return MiniApp{}, fmt.Errorf("draft %q not found", id)
	}
	return app, nil
}

func (s *runnerTestStore) GetRelease(id, version string) (MiniApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	app, ok := s.apps[id]
	if !ok || app.Version != version || app.State != StateReleased {
		return MiniApp{}, fmt.Errorf("release %s@%s not found", id, version)
	}
	return app, nil
}

func (s *runnerTestStore) SaveRun(run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, run)
	return nil
}

type runnerTestTool struct {
	calls int
	fn    func(context.Context, ToolRequest) (any, error)
}

type rejectingCapabilityTool struct{ calls int }

func (t *rejectingCapabilityTool) ExecuteTool(context.Context, ToolRequest) (any, error) {
	t.calls++
	return nil, nil
}

func (*rejectingCapabilityTool) ValidateMiniAppCapabilities(MiniApp) error {
	return errors.New("later tool unavailable")
}

func (t *runnerTestTool) ExecuteTool(ctx context.Context, req ToolRequest) (any, error) {
	t.calls++
	if t.fn == nil {
		return req.Arguments, nil
	}
	return t.fn(ctx, req)
}

type runnerTestModel struct {
	calls int
	fn    func(context.Context, ModelRequest) (any, error)
}

func (m *runnerTestModel) ExecuteModel(ctx context.Context, req ModelRequest) (any, error) {
	m.calls++
	if m.fn == nil {
		return req.Prompt, nil
	}
	return m.fn(ctx, req)
}

type runnerTestAgent struct {
	calls int
	fn    func(context.Context, AgentRequest) (any, error)
}

func (a *runnerTestAgent) ExecuteAgent(ctx context.Context, req AgentRequest) (any, error) {
	a.calls++
	if a.fn == nil {
		return req.Prompt, nil
	}
	return a.fn(ctx, req)
}

type runnerTestEvents struct {
	mu     sync.Mutex
	events []RunEvent
}

func (s *runnerTestEvents) Emit(_ context.Context, event RunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func runnerTestApp() MiniApp {
	return MiniApp{
		SchemaVersion: SchemaVersion,
		Kind:          KindMiniApp,
		ID:            "runner-test",
		State:         StateDraft,
		Metadata:      Metadata{Name: "Runner test", Goal: "Exercise runtime"},
		Permissions:   Permissions{Tools: []string{"fake"}, Apps: []string{"nested"}},
		Inputs:        []Input{{ID: "source", Type: "string", Title: "Source", Required: true, UI: InputUI{Control: "text"}}},
		Workflow: []Step{
			{ID: "first", Kind: "tool", Title: "First", Tool: "fake", Arguments: map[string]any{"value": Ref{Ref: "inputs.source"}}},
			{ID: "second", Kind: "tool", Title: "Second", Tool: "fake", Arguments: map[string]any{"value": Ref{Ref: "steps.first.outputs.result"}}},
		},
		Success: SuccessSpec{Mode: "all", Checks: []SuccessCheck{{Kind: "step", Step: "second", Status: string(RunSucceeded)}}},
		Outputs: []Output{{ID: "result", Type: "string", Value: Ref{Ref: "steps.second.outputs.result"}}},
		Runtime: RuntimePolicy{LogScope: "global", OperatorEventLevel: "status", DiagnosticToolEvents: "sanitized"},
	}
}

func TestRunnerSequentialRefsOutputsEventsAndPersistence(t *testing.T) {
	app := runnerTestApp()
	store := &runnerTestStore{apps: map[string]MiniApp{app.ID: app}}
	tool := &runnerTestTool{fn: func(_ context.Context, req ToolRequest) (any, error) {
		args, ok := req.Arguments.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("arguments type %T", req.Arguments)
		}
		return strings.ToUpper(fmt.Sprint(args["value"])), nil
	}}
	events := &runnerTestEvents{}
	runner := NewRunner(store, Executors{Tool: tool, Events: events})

	run, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "draft"}, nil)
	if err != nil {
		t.Fatalf("RunPortable() error = %v", err)
	}
	if run.Status != RunSucceeded || run.Outputs["result"] != "DRAFT" {
		t.Fatalf("run = %+v, want succeeded DRAFT", run)
	}
	if tool.calls != 2 {
		t.Fatalf("tool calls = %d, want 2", tool.calls)
	}
	if len(store.runs) < 3 {
		t.Fatalf("persisted run checkpoints = %d, want incremental checkpoints", len(store.runs))
	}
	if len(events.events) < 5 {
		t.Fatalf("events = %d, want start/step/final events", len(events.events))
	}
	if events.events[0].Type != "run.started" {
		t.Fatalf("first event = %#v, want run.started", events.events[0])
	}
	if got := events.events[len(events.events)-1].Type; got != "run.succeeded" {
		t.Fatalf("last event type = %q, want run.succeeded", got)
	}
}

func TestRunnerKeepsWorkspaceBesidePersistedRun(t *testing.T) {
	app := runnerTestApp()
	runRoot := t.TempDir()
	store := NewStoreWithRunRoot(t.TempDir(), runRoot)
	var workspace string
	tool := &runnerTestTool{fn: func(_ context.Context, req ToolRequest) (any, error) {
		workspace = req.Workspace
		if err := os.WriteFile(filepath.Join(req.Workspace, "result.txt"), []byte("artifact"), 0o600); err != nil {
			return nil, err
		}
		return "ok", nil
	}}
	runner := NewRunner(store, Executors{Tool: tool})
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "value"}, nil)
	if err != nil {
		t.Fatalf("RunPortable() error = %v", err)
	}
	want := filepath.Join(runRoot, app.ID, "runs", run.ID, "workspace")
	if workspace != want {
		t.Fatalf("workspace = %q, want %q", workspace, want)
	}
	if _, err := store.GetRun(app.ID, run.ID); err != nil {
		t.Fatalf("persisted run missing beside workspace: %v", err)
	}
	runDir := filepath.Dir(want)
	if run.EventsPath != filepath.Join(runDir, "events.jsonl") || run.LogPath != filepath.Join(runDir, "execution.log") {
		t.Fatalf("run paths = events %q log %q, want siblings of workspace", run.EventsPath, run.LogPath)
	}
	if len(run.Artifacts) != 1 || run.Artifacts[0].Path != "result.txt" || run.Artifacts[0].SizeBytes != int64(len("artifact")) {
		t.Fatalf("artifacts = %#v", run.Artifacts)
	}
}

func TestRunnerInputValidationRetriesTimeoutAndCancellation(t *testing.T) {
	app := runnerTestApp()
	minLength := 3
	app.Inputs[0].Validation.MinLength = &minLength
	app.Workflow = []Step{{ID: "retry", Kind: "tool", Title: "Retry", Tool: "fake", Arguments: map[string]any{"value": Ref{Ref: "inputs.source"}}, Retry: RetryPolicy{MaxAttempts: 2}}}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "retry", Status: string(RunSucceeded)}}
	app.Outputs = nil
	store := &runnerTestStore{apps: map[string]MiniApp{app.ID: app}}
	tool := &runnerTestTool{}
	tool.fn = func(_ context.Context, req ToolRequest) (any, error) {
		if tool.calls == 1 {
			return nil, errors.New("transient")
		}
		return req.Arguments, nil
	}
	runner := NewRunner(store, Executors{Tool: tool})

	waiting, err := runner.RunPortable(context.Background(), app, map[string]any{}, nil)
	if err == nil || waiting.Status != RunWaitingForInput || !IsMissingInput(err) {
		t.Fatalf("missing input run = %#v, error=%v", waiting, err)
	}

	bad, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "x"}, nil)
	if err == nil || bad.Status != RunFailed || !strings.Contains(bad.Error, "too short") {
		t.Fatalf("invalid input run = %#v, error=%v", bad, err)
	}

	good, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "okay"}, nil)
	if err != nil {
		t.Fatalf("retry run error = %v", err)
	}
	if got := good.Steps["retry"].Attempts; got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}

	app.Workflow[0].TimeoutSeconds = 1
	tool.fn = func(ctx context.Context, _ ToolRequest) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	started := time.Now()
	timed, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "okay"}, nil)
	if err == nil || timed.Status != RunCancelled || time.Since(started) > 3*time.Second {
		t.Fatalf("timeout run = %#v, error=%v", timed, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled, err := runner.RunPortable(ctx, app, map[string]any{"source": "okay"}, nil)
	if err == nil || cancelled.Status != RunCancelled {
		t.Fatalf("cancelled run = %#v, error=%v", cancelled, err)
	}
}

func TestRunnerConfirmationBranchAndNestedDepth(t *testing.T) {
	app := runnerTestApp()
	app.Permissions.Apps = []string{"nested"}
	app.Inputs = []Input{{ID: "enabled", Type: "boolean", Title: "Enabled", Required: true, UI: InputUI{Control: "checkbox"}}}
	app.Workflow = []Step{
		{ID: "confirm", Kind: "confirm", Title: "Confirm", Message: "Proceed?"},
		{ID: "branch", Kind: "branch", Title: "Branch", If: &Condition{Op: "eq", Left: Ref{Ref: "inputs.enabled"}, Right: true}, Then: []Step{{ID: "then", Kind: "tool", Title: "Then", Tool: "fake", Arguments: map[string]any{"value": "then"}}}, Else: []Step{{ID: "else", Kind: "tool", Title: "Else", Tool: "fake", Arguments: map[string]any{"value": "else"}}}},
	}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "branch", Status: string(RunSucceeded)}}
	app.Outputs = nil
	store := &runnerTestStore{apps: map[string]MiniApp{app.ID: app}}
	tool := &runnerTestTool{}
	runner := NewRunner(store, Executors{Tool: tool})

	waiting, err := runner.RunPortable(context.Background(), app, map[string]any{"enabled": true}, nil)
	if err == nil || waiting.Status != RunWaitingForConfirmation {
		t.Fatalf("confirmation run = %#v, error=%v; want waiting", waiting, err)
	}
	completed, err := runner.RunPortable(context.Background(), app, map[string]any{"enabled": true}, &OperatorDecisions{Confirmations: map[string]bool{"confirm": true}})
	if err != nil || completed.Status != RunSucceeded {
		t.Fatalf("confirmed run = %#v, error=%v", completed, err)
	}
	if _, ok := completed.Steps["then"]; !ok {
		t.Fatalf("then branch did not execute: %#v", completed.Steps)
	}
	if _, ok := completed.Steps["else"]; ok {
		t.Fatal("else branch executed unexpectedly")
	}

	// A self-referencing release must stop at the bounded nesting limit rather
	// than recurse until the process stack is exhausted.
	self := runnerTestApp()
	self.ID, self.State, self.Version = "nested", StateReleased, "1.0.0"
	self.Permissions = Permissions{Apps: []string{"nested"}}
	self.Inputs = nil
	self.Workflow = []Step{{ID: "loop", Kind: "miniapp", Title: "Loop", AppID: "nested", AppVersion: "1.0.0"}}
	self.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{Kind: "step", Step: "loop", Status: string(RunSucceeded)}}}
	self.Outputs = nil
	store.apps["nested"] = self
	_, err = runner.RunRelease(context.Background(), "nested", "1.0.0", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("nested release error = %v, want bounded nesting error", err)
	}
}

func TestRunnerConfirmationPreflightPreventsDuplicateSideEffects(t *testing.T) {
	app := runnerTestApp()
	app.Workflow = []Step{
		{ID: "write-first", Kind: "tool", Title: "Write first", Tool: "fake", Arguments: map[string]any{"value": "side effect"}},
		{ID: "approve", Kind: "confirm", Title: "Approve", Message: "Proceed?"},
	}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "write-first", Status: string(RunSucceeded)}}
	app.Outputs = nil
	tool := &runnerTestTool{}
	runner := NewRunner(&runnerTestStore{apps: map[string]MiniApp{app.ID: app}}, Executors{Tool: tool})

	waiting, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "value"}, nil)
	if err == nil || waiting.Status != RunWaitingForConfirmation {
		t.Fatalf("waiting run = %+v, error=%v", waiting, err)
	}
	if tool.calls != 0 {
		t.Fatalf("tool calls before confirmation = %d, want 0", tool.calls)
	}

	completed, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "value"}, &OperatorDecisions{Confirmations: map[string]bool{"approve": true}})
	if err != nil || completed.Status != RunSucceeded {
		t.Fatalf("confirmed run = %+v, error=%v", completed, err)
	}
	if tool.calls != 1 {
		t.Fatalf("tool calls after confirmation = %d, want exactly 1", tool.calls)
	}
}

func TestRunnerNamespacesNestedConfirmations(t *testing.T) {
	nested := runnerTestApp()
	nested.ID, nested.State, nested.Version = "nested-app", StateReleased, "1.0.0"
	nested.Inputs = nil
	nested.Workflow = []Step{
		{ID: "approve", Kind: "confirm", Title: "Nested approval", Message: "Approve nested action?"},
		{ID: "nested-tool", Kind: "tool", Title: "Nested tool", Tool: "fake", Arguments: map[string]any{"value": "nested"}},
	}
	nested.Success.Checks = []SuccessCheck{{Kind: "step", Step: "nested-tool", Status: string(RunSucceeded)}}
	nested.Outputs = nil

	parent := runnerTestApp()
	parent.ID = "parent-app"
	parent.Inputs = nil
	parent.Permissions.Apps = []string{nested.ID}
	parent.Workflow = []Step{
		{ID: "approve", Kind: "confirm", Title: "Parent approval", Message: "Approve parent action?"},
		{ID: "nested-step", Kind: "miniapp", Title: "Nested app", AppID: nested.ID, AppVersion: nested.Version},
	}
	parent.Success.Checks = []SuccessCheck{{Kind: "step", Step: "nested-step", Status: string(RunSucceeded)}}
	parent.Outputs = nil
	store := &runnerTestStore{apps: map[string]MiniApp{parent.ID: parent, nested.ID: nested}}
	tool := &runnerTestTool{}
	runner := NewRunner(store, Executors{Tool: tool})

	waiting, err := runner.RunPortable(context.Background(), parent, nil, &OperatorDecisions{Confirmations: map[string]bool{"approve": true}})
	if err == nil || waiting.Status != RunWaitingForConfirmation {
		t.Fatalf("nested waiting run = %+v, error=%v", waiting, err)
	}
	if _, ok := waiting.Steps["nested-step.approve"]; !ok {
		t.Fatalf("missing namespaced waiting step: %+v", waiting.Steps)
	}
	if tool.calls != 0 {
		t.Fatalf("nested tool ran before its own confirmation: %d", tool.calls)
	}

	decisions := &OperatorDecisions{Confirmations: map[string]bool{"approve": true, "nested-step.approve": true}}
	completed, err := runner.RunPortable(context.Background(), parent, nil, decisions)
	if err != nil || completed.Status != RunSucceeded {
		t.Fatalf("confirmed nested run = %+v, error=%v", completed, err)
	}
	if tool.calls != 1 {
		t.Fatalf("nested tool calls = %d, want 1", tool.calls)
	}
}

func TestRunnerOnlyRequiresConfirmationFromSelectedInputBranch(t *testing.T) {
	app := runnerTestApp()
	app.Inputs = []Input{{ID: "enabled", Type: "boolean", Title: "Enabled", Required: true, UI: InputUI{Control: "checkbox"}}}
	app.Workflow = []Step{{
		ID: "choose", Kind: "branch", Title: "Choose", If: &Condition{Op: "eq", Left: Ref{Ref: "inputs.enabled"}, Right: true},
		Then: []Step{{ID: "approve-enabled", Kind: "confirm", Title: "Approve enabled", Message: "Approve enabled path"}},
		Else: []Step{{ID: "approve-disabled", Kind: "confirm", Title: "Approve disabled", Message: "Approve disabled path"}},
	}}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "choose", Status: string(RunSucceeded)}}
	app.Outputs = nil
	runner := NewRunner(&runnerTestStore{apps: map[string]MiniApp{app.ID: app}}, Executors{})
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"enabled": true}, &OperatorDecisions{Confirmations: map[string]bool{"approve-enabled": true}})
	if err != nil || run.Status != RunSucceeded {
		t.Fatalf("selected branch confirmation run = %+v, error=%v", run, err)
	}
}

func TestRunnerSuccessSchemaAndModelSteps(t *testing.T) {
	app := runnerTestApp()
	app.Permissions = Permissions{Models: []string{"binding"}}
	app.Workflow = []Step{{ID: "classify", Kind: "llm", Title: "Classify", ModelBinding: "binding", Prompt: "classify {{ inputs.source }}", OutputSchema: map[string]any{"type": "object", "required": []any{"label"}}}}
	app.Requirements.ModelBindings = []ModelBinding{{ID: "binding", Selection: "fixed", Provider: ProviderIdentity{Type: "openai", BaseURL: "https://example.test"}, Model: "fake-model"}}
	app.Success.Checks = []SuccessCheck{{Kind: "schema", Value: Ref{Ref: "steps.classify.outputs.result"}, Schema: map[string]any{"type": "object", "required": []any{"label"}}}}
	app.Outputs = []Output{{ID: "label", Type: "string", Value: Ref{Ref: "steps.classify.outputs.result.label"}}}
	store := &runnerTestStore{apps: map[string]MiniApp{app.ID: app}}
	model := &runnerTestModel{fn: func(_ context.Context, req ModelRequest) (any, error) {
		if req.Prompt != "classify source" {
			return nil, fmt.Errorf("unexpected prompt %q", req.Prompt)
		}
		return map[string]any{"label": "ok"}, nil
	}}
	runner := NewRunner(store, Executors{Model: model})
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "source"}, nil)
	if err != nil || run.Status != RunSucceeded || run.Outputs["label"] != "ok" {
		t.Fatalf("model run = %#v, error=%v", run, err)
	}
	if model.calls != 1 {
		t.Fatalf("model calls = %d, want 1", model.calls)
	}
}

func TestModelPermissionDeclaredAcceptsBindingAliases(t *testing.T) {
	binding := ModelBinding{ID: "binding", Selection: "fixed", Model: "fake-model"}
	for _, permissions := range [][]string{{"binding"}, {"fixed"}, {"fake-model"}} {
		if !modelPermissionDeclared(permissions, binding) {
			t.Fatalf("permissions %v did not match model binding aliases", permissions)
		}
	}
}

func TestRunnerSafeRedactionNeverPersistsSecretValues(t *testing.T) {
	app := runnerTestApp()
	app.Inputs = []Input{{ID: "token", Type: "secret", Title: "Token", Required: true, UI: InputUI{Control: "password"}}}
	app.Workflow = []Step{{ID: "secret", Kind: "tool", Title: "Secret", Tool: "fake", Arguments: map[string]any{"api_token": Ref{Ref: "inputs.token"}}}}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "secret", Status: string(RunSucceeded)}}
	app.Outputs = nil
	store := &runnerTestStore{apps: map[string]MiniApp{app.ID: app}}
	tool := &runnerTestTool{fn: func(_ context.Context, req ToolRequest) (any, error) {
		return req.Arguments, nil
	}}
	events := &runnerTestEvents{}
	runner := NewRunner(store, Executors{Tool: tool, Events: events})
	const secret = "super-secret-token"
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"token": secret}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf("%#v", run) + fmt.Sprintf("%#v", store.runs) + fmt.Sprintf("%#v", events.events)
	if strings.Contains(raw, secret) {
		t.Fatalf("secret leaked into persisted/event values: %s", raw)
	}
	if !reflect.DeepEqual(run.Inputs["token"], "REDACTED") {
		t.Fatalf("redacted input = %#v", run.Inputs["token"])
	}
}

func TestRunnerHonorsDiagnosticEventPolicy(t *testing.T) {
	app := runnerTestApp()
	app.Runtime.DiagnosticToolEvents = "none"
	events := &runnerTestEvents{}
	runner := NewRunner(&runnerTestStore{apps: map[string]MiniApp{app.ID: app}}, Executors{
		Tool: &runnerTestTool{fn: func(context.Context, ToolRequest) (any, error) {
			return map[string]any{"private": "payload"}, nil
		}},
		Events: events,
	})
	if _, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "value"}, nil); err != nil {
		t.Fatal(err)
	}
	for _, event := range events.events {
		if event.Data != nil {
			t.Fatalf("diagnostic event data persisted under none policy: %#v", event)
		}
	}
}

func TestRunnerCapabilityPreflightPreventsSideEffects(t *testing.T) {
	app := runnerTestApp()
	tool := &rejectingCapabilityTool{}
	runner := NewRunner(&runnerTestStore{apps: map[string]MiniApp{app.ID: app}}, Executors{Tool: tool})
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "value"}, nil)
	if err == nil || !strings.Contains(err.Error(), "later tool unavailable") {
		t.Fatalf("preflight run = %+v, error=%v", run, err)
	}
	if tool.calls != 0 {
		t.Fatalf("tool executed before failed capability preflight: %d", tool.calls)
	}
}

func TestRunnerMaterializesPrivateFixtureInIsolatedWorkspace(t *testing.T) {
	app := runnerTestApp()
	tool := &runnerTestTool{fn: func(_ context.Context, req ToolRequest) (any, error) {
		raw, err := os.ReadFile(filepath.Join(req.Workspace, "source", "input.txt"))
		return string(raw), err
	}}
	runner := NewRunner(&runnerTestStore{apps: map[string]MiniApp{app.ID: app}}, Executors{Tool: tool})
	run, err := runner.RunPortableWithFixture(context.Background(), app, map[string]any{"source": "source/input.txt"}, nil, map[string][]byte{"source/input.txt": []byte("fixture data\n")})
	if err != nil || run.Status != RunSucceeded {
		t.Fatalf("fixture run = %+v, error=%v", run, err)
	}
	if got := run.Outputs["result"]; got != "fixture data\n" {
		t.Fatalf("fixture result = %#v", got)
	}
	if strings.Contains(fmt.Sprintf("%#v", run), "fixture data") {
		// The workflow output is expected to contain tool output, but fixture
		// bytes must never be copied into inputs or path metadata implicitly.
		if strings.Contains(fmt.Sprintf("%#v", run.Inputs), "fixture data") {
			t.Fatalf("fixture bytes leaked into persisted inputs: %#v", run.Inputs)
		}
	}
}
