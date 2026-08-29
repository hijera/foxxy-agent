//go:build miniapps

package miniapps

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestServiceDistillRequiresScenarioThenCreatesDraft(t *testing.T) {
	store := NewStore(t.TempDir())
	service := NewService(store, nil)
	input := DistillInput{
		SessionID: "session-service-1", Title: "Greet operator",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Greet Ada."},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "greet", InputJSON: `{"name":"Ada"}`}}},
			{Role: llm.RoleTool, ToolCallID: "call-1", Content: "hello Ada"},
			{Role: llm.RoleAssistant, Content: "hello Ada"},
		},
	}
	job, err := service.StartDistillation(input)
	if err != nil {
		t.Fatal(err)
	}
	waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status == JobWaitingForScenario })
	waiting, err := service.GetJob(job.ID)
	if err != nil || len(waiting.Candidates) != 1 {
		t.Fatalf("waiting job = %+v, error=%v", waiting, err)
	}
	events, err := service.Events(job.ID, 0)
	if err != nil || len(events) < 2 {
		t.Fatalf("events = %+v, error=%v", events, err)
	}
	if events[0].Seq != 1 || events[len(events)-1].Status != JobWaitingForScenario {
		t.Fatalf("event history = %+v", events)
	}
	confirmed, err := service.ConfirmScenario(job.ID, TraceScenarioSelection{CandidateID: waiting.Candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != JobRunning {
		t.Fatalf("confirmed job = %+v, want running", confirmed)
	}
	completed := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status == JobSucceeded })
	if completed.AppID == "" {
		t.Fatalf("completed job = %+v, want app id", completed)
	}
	if _, err := store.GetDraft(completed.AppID); err != nil {
		t.Fatalf("generated draft missing: %v", err)
	}
}

func TestServiceDistillationReportsNotSuitableOutcome(t *testing.T) {
	service := NewService(NewStore(t.TempDir()), nil)
	job, err := service.StartDistillation(DistillInput{SessionID: "conversation-only", Messages: []llm.Message{
		{Role: llm.RoleUser, Content: "Explain this concept."},
		{Role: llm.RoleAssistant, Content: "Here is an explanation."},
	}})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if completed.Phase != TraceEligibilityNotSuitable || completed.Result["eligibility"] != TraceEligibilityNotSuitable {
		t.Fatalf("not-suitable outcome = %+v", completed)
	}
}

func TestServiceTestRunVerifiesAndRecordsPassingTest(t *testing.T) {
	store := NewStore(t.TempDir())
	app := verificationTestApp()
	evidence := SourceEvidence{
		AcceptedResult: "hello Ada",
		SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{Name: "greet", Status: TraceActionSucceeded, Arguments: `{"name":"Ada"}`}}},
	}
	if err := store.CreateDraft(app, &evidence); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(store, Executors{Tool: serviceGreetTool{}})
	service := NewService(store, runner)
	job, err := service.StartTestRun(app.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if completed.Status != JobSucceeded || completed.Report == nil || !completed.Report.Passed {
		t.Fatalf("test job = %+v, want verified success", completed)
	}
	if completed.RunID == "" {
		t.Fatal("test job did not persist run id")
	}
	if _, err := store.Release(app.ID, "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: appRevision(t, store, app.ID)}); err != nil {
		t.Fatalf("release after passing test = %v", err)
	}
}

func TestServiceTestRunPinsDraftRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	app := verificationTestApp()
	evidence := SourceEvidence{AcceptedResult: "hello Ada", SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{Name: "greet", Status: TraceActionSucceeded, Arguments: `{"name":"Ada"}`}}}}
	if err := store.CreateDraft(app, &evidence); err != nil {
		t.Fatal(err)
	}
	original := appRevision(t, store, app.ID)
	started, release := make(chan struct{}), make(chan struct{})
	runner := NewRunner(store, Executors{Tool: blockingResultTool{started: started, release: release, result: "hello Ada"}})
	service := NewService(store, runner)
	job, err := service.StartTestRun(app.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("test tool did not start")
	}
	current, err := store.GetDraft(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Metadata.Description = "new revision while test is running"
	updated, err := store.UpdateDraft(app.ID, original, current)
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	completed := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if completed.RunID == "" {
		t.Fatalf("completed job has no run: %+v", completed)
	}
	run, err := store.GetRun(app.ID, completed.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Revision != original || run.Revision == updated.Revision {
		t.Fatalf("run revision = %q, original %q, updated %q", run.Revision, original, updated.Revision)
	}
}

type blockingResultTool struct {
	started chan struct{}
	release chan struct{}
	result  any
}

func (t blockingResultTool) ExecuteTool(ctx context.Context, _ ToolRequest) (any, error) {
	close(t.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.release:
		return t.result, nil
	}
}

func TestServiceReleaseRunCancellationAndEventReplay(t *testing.T) {
	store := NewStore(t.TempDir())
	app := verificationTestApp()
	app.Inputs = nil
	app.Workflow[0].Arguments = map[string]any{"name": "Ada"}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "greet", Status: string(RunSucceeded)}}
	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}
	// A release needs a passing test; this fixture is manually recorded to keep
	// the cancellation test focused on the asynchronous release worker.
	stored := appRevision(t, store, app.ID)
	if err := store.RecordPassingTest(app.ID, Run{ID: "test-run", AppID: app.ID, Revision: stored, Test: true, Status: RunSucceeded}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Release(app.ID, "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: stored}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	runner := NewRunner(store, Executors{Tool: blockingTool{started: started}})
	service := NewService(store, runner)
	job, err := service.StartReleaseRun(app.ID, "1.0.0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("release tool did not start")
	}
	cancelled, err := service.Cancel(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != JobCancelled {
		t.Fatalf("cancelled job = %+v", cancelled)
	}
	events, err := service.Events(job.ID, 0)
	if err != nil || len(events) == 0 || events[len(events)-1].Status != JobCancelled {
		t.Fatalf("cancel event history = %+v, error=%v", events, err)
	}
	// A fresh service reads the append-only event history and job record.
	reloaded := NewService(store, runner)
	replayed, err := reloaded.Events(job.ID, 1)
	if err != nil || len(replayed) == 0 {
		t.Fatalf("replayed events = %+v, error=%v", replayed, err)
	}
}

func TestServiceConfirmRunResumesWaitingConfirmation(t *testing.T) {
	store := NewStore(t.TempDir())
	app := verificationTestApp()
	app.ID = "confirm-service"
	app.Inputs = nil
	app.Workflow = []Step{
		{ID: "confirm", Kind: "confirm", Title: "Confirm", Message: "Proceed?"},
		{ID: "confirm-output", Kind: "confirm", Title: "Confirm output", Message: "Write the result?"},
		{ID: "greet", Kind: "tool", Title: "Greet", Tool: "greet", Arguments: map[string]any{"name": "Ada"}},
	}
	app.Success.Checks = []SuccessCheck{{Kind: "step", Step: "greet", Status: string(RunSucceeded)}}
	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}
	revision := appRevision(t, store, app.ID)
	if err := store.RecordPassingTest(app.ID, Run{ID: "test-confirm", AppID: app.ID, Revision: revision, Test: true, Status: RunSucceeded}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Release(app.ID, "1.0.0", ReleaseOptions{Approved: true, ExpectedRevision: revision}); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, NewRunner(store, Executors{Tool: serviceGreetTool{}}))
	job, err := service.StartReleaseRun(app.ID, "1.0.0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status == JobWaitingForConfirm })
	if waiting.Confirmation == nil || waiting.Confirmation.ID != "confirm" || waiting.Confirmation.Message != "Proceed?" {
		t.Fatalf("pending confirmation = %+v", waiting.Confirmation)
	}
	if _, err := service.ConfirmRun(job.ID, &OperatorDecisions{Confirmations: map[string]bool{"confirm": true}}); err != nil {
		t.Fatal(err)
	}
	waiting = waitForJob(t, service, job.ID, func(item AsyncJob) bool {
		return item.Status == JobWaitingForConfirm && item.Confirmation != nil && item.Confirmation.ID == "confirm-output"
	})
	if _, err := service.ConfirmRun(job.ID, &OperatorDecisions{Confirmations: map[string]bool{"confirm-output": true}}); err != nil {
		t.Fatal(err)
	}
	completed := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() && item.Status != JobWaitingForConfirm })
	if completed.Status != JobSucceeded {
		t.Fatalf("resumed job = %+v", completed)
	}
}

func TestServiceRecoversPersistedRunningJobAsInterrupted(t *testing.T) {
	store := NewStore(t.TempDir())
	service := NewService(store, nil)
	job := AsyncJob{ID: "job-recover", Kind: JobTestRun, Status: JobRunning, Phase: "running", CreatedAt: time.Now().UTC()}
	service.persistJob(job)
	reloaded := NewService(store, nil)
	recovered, err := reloaded.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != JobInterrupted || recovered.Error == "" {
		t.Fatalf("recovered job = %+v", recovered)
	}
}

func TestServiceNeverPersistsRawSecretToolErrors(t *testing.T) {
	store := NewStore(t.TempDir())
	app := verificationTestApp()
	app.ID = "secret-error-service"
	app.Inputs[0].Type = "secret"
	app.Inputs[0].UI.Control = "password"
	evidence := SourceEvidence{AcceptedResult: "unused", SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{Name: "greet", Status: TraceActionSucceeded, Arguments: `{"name":"[REDACTED]"}`}}}}
	if err := store.CreateDraft(app, &evidence); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, NewRunner(store, Executors{Tool: secretErrorTool{}}))
	const secret = "runtime-secret-value"
	job, err := service.StartTestRun(app.ID, map[string]any{"name": secret}, nil)
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if strings.Contains(failed.Error, secret) || !strings.Contains(failed.Error, "REDACTED") {
		t.Fatalf("job error was not redacted: %q", failed.Error)
	}
	events, err := service.Events(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(event.Message, secret) {
			t.Fatalf("event leaked secret: %+v", event)
		}
	}
}

func waitForJob(t *testing.T, service *Service, id string, done func(AsyncJob) bool) AsyncJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := service.GetJob(id)
		if err == nil && done(job) {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := service.GetJob(id)
	t.Fatalf("job %s did not reach expected state: %+v", id, job)
	return AsyncJob{}
}

func appRevision(t *testing.T, store *Store, id string) string {
	t.Helper()
	app, err := store.GetDraft(id)
	if err != nil {
		t.Fatal(err)
	}
	return app.Revision
}

type serviceGreetTool struct{}

func (serviceGreetTool) ExecuteTool(_ context.Context, req ToolRequest) (any, error) {
	args, ok := req.Arguments.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("arguments type %T", req.Arguments)
	}
	return "hello " + fmt.Sprint(args["name"]), nil
}

type blockingTool struct{ started chan struct{} }

type secretErrorTool struct{}

func (secretErrorTool) ExecuteTool(_ context.Context, req ToolRequest) (any, error) {
	args := req.Arguments.(map[string]any)
	return nil, fmt.Errorf("provider rejected credential %s", args["name"])
}

func (tool blockingTool) ExecuteTool(ctx context.Context, _ ToolRequest) (any, error) {
	select {
	case tool.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
