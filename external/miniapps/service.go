//go:build miniapps

package miniapps

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// JobKind identifies the asynchronous operation represented by AsyncJob.
type JobKind string

const (
	JobDistillation JobKind = "distillation"
	JobTestRun      JobKind = "test_run"
	JobReleaseRun   JobKind = "release_run"
)

// JobStatus is deliberately a superset of RunStatus: distillation has a
// scenario-review state, while run jobs retain waiting states for operators.
type JobStatus string

const (
	JobQueued             JobStatus = "queued"
	JobRunning            JobStatus = "running"
	JobWaitingForScenario JobStatus = "waiting_for_scenario"
	JobWaitingForInput    JobStatus = "waiting_for_input"
	JobWaitingForConfirm  JobStatus = "waiting_for_confirmation"
	JobSucceeded          JobStatus = "succeeded"
	JobFailed             JobStatus = "failed"
	JobCancelled          JobStatus = "cancelled"
	JobInterrupted        JobStatus = "interrupted"
)

func (s JobStatus) terminal() bool {
	return s == JobSucceeded || s == JobFailed || s == JobCancelled || s == JobInterrupted
}

// AsyncJob is the persisted status envelope used by distillation and run
// endpoints. Result contains only identifiers and sanitized summaries.
type AsyncJob struct {
	ID           string                   `json:"id"`
	Kind         JobKind                  `json:"kind"`
	SessionID    string                   `json:"session_id,omitempty"`
	AppID        string                   `json:"app_id,omitempty"`
	Version      string                   `json:"version,omitempty"`
	RunID        string                   `json:"run_id,omitempty"`
	Revision     string                   `json:"revision,omitempty"`
	Status       JobStatus                `json:"status"`
	Phase        string                   `json:"phase,omitempty"`
	Progress     int                      `json:"progress,omitempty"`
	Summary      string                   `json:"summary,omitempty"`
	Error        string                   `json:"error,omitempty"`
	Candidates   []TraceScenarioCandidate `json:"candidates,omitempty"`
	Report       *VerificationReport      `json:"report,omitempty"`
	Result       map[string]any           `json:"result,omitempty"`
	Confirmation *PendingConfirmation     `json:"confirmation,omitempty"`
	CreatedAt    time.Time                `json:"created_at"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

type PendingConfirmation struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// JobEvent is append-only history suitable for replaying an SSE stream. Seq
// starts at one for each job and is persisted alongside the job record.
type JobEvent struct {
	Seq       uint64         `json:"seq"`
	JobID     string         `json:"job_id"`
	Kind      JobKind        `json:"kind"`
	Type      string         `json:"type"`
	Status    JobStatus      `json:"status,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Progress  int            `json:"progress,omitempty"`
	Message   string         `json:"message,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type pendingDistillation struct {
	Input      DistillInput
	Trace      NormalizedTrace
	Candidates []TraceScenarioCandidate
}

type pendingRun struct {
	kind      JobKind
	app       MiniApp
	appID     string
	version   string
	inputs    map[string]any
	decisions *OperatorDecisions
	evidence  SourceEvidence
}

// Service owns asynchronous Mini App lifecycle work. It intentionally has no
// HTTP dependency; transports can expose GetJob and Events as JSON or SSE.
type Service struct {
	store  *Store
	runner *Runner

	mu      sync.Mutex
	jobs    map[string]AsyncJob
	events  map[string][]JobEvent
	cancels map[string]context.CancelFunc
	pending map[string]pendingDistillation
	runs    map[string]pendingRun
	workers map[string]chan struct{}
}

func NewService(store *Store, runner *Runner) *Service {
	service := &Service{
		store: store, runner: runner,
		jobs: make(map[string]AsyncJob), events: make(map[string][]JobEvent),
		cancels: make(map[string]context.CancelFunc), pending: make(map[string]pendingDistillation), runs: make(map[string]pendingRun), workers: make(map[string]chan struct{}),
	}
	if service.runner == nil && store != nil {
		service.runner = NewRunner(store, Executors{})
	}
	service.RecoverInterruptedJobs()
	return service
}

func (s *Service) jobDir() string {
	if s == nil || s.store == nil {
		return ""
	}
	return filepath.Join(s.store.Root(), ".jobs")
}

func (s *Service) jobPath(id string) string {
	return filepath.Join(s.jobDir(), id+".json")
}

func (s *Service) eventsPath(id string) string {
	return filepath.Join(s.jobDir(), id+".events.jsonl")
}

// StartDistillation first normalizes and assesses the trace. A candidate list
// is persisted in the waiting state; only ConfirmScenario can start workflow
// synthesis, which prevents accidental generation from an ambiguous trace.
func (s *Service) StartDistillation(input DistillInput) (AsyncJob, error) {
	if s == nil || s.store == nil {
		return AsyncJob{}, errors.New("mini app service is unavailable")
	}
	if strings.TrimSpace(input.SessionID) == "" {
		return AsyncJob{}, errors.New("session id is required")
	}
	job, ctx := s.newJob(JobDistillation, input.SessionID, "", "")
	s.mu.Lock()
	s.pending[job.ID] = pendingDistillation{Input: input}
	s.mu.Unlock()
	s.launchWorker(job.ID, func() { s.runDistillationTrace(ctx, job.ID) })
	return job, nil
}

func (s *Service) runDistillationTrace(ctx context.Context, id string) {
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress = JobRunning, "analyzing_session", 10
	})
	s.mu.Lock()
	pending, ok := s.pending[id]
	s.mu.Unlock()
	if !ok {
		s.failJob(id, errors.New("distillation input is unavailable"))
		return
	}
	if err := ctx.Err(); err != nil {
		s.finishContextJob(id, err)
		return
	}
	trace := pending.Trace
	var eligibility TraceEligibility
	var candidates []TraceScenarioCandidate
	var err error
	if len(trace.Messages) == 0 && len(trace.Actions) == 0 {
		trace, eligibility, candidates, err = DistillTrace(TraceInput{
			SessionID: pending.Input.SessionID, Messages: pending.Input.Messages,
			Evidence: pending.Input.Evidence, TurnActive: pending.Input.TurnActive,
		})
	} else {
		eligibility = AssessTraceEligibility(trace, pending.Input.TurnActive)
		candidates = GenerateScenarioCandidates(trace)
	}
	if err != nil {
		s.failJob(id, err)
		return
	}
	if !eligibility.Eligible {
		s.updateJob(id, func(job *AsyncJob) {
			job.Status, job.Phase, job.Progress = JobFailed, eligibility.Status, 100
			job.Error = redactString(eligibility.Reason, nil)
			job.Result = map[string]any{"eligibility": eligibility.Status, "reason": eligibility.Reason, "tool_calls": trace.ToolCallCount}
		})
		s.clearActive(id)
		return
	}
	s.mu.Lock()
	pending.Trace, pending.Candidates, pending.Input.Trace = trace, candidates, &trace
	s.pending[id] = pending
	s.mu.Unlock()
	s.updateJob(id, func(job *AsyncJob) {
		job.Candidates = append([]TraceScenarioCandidate(nil), candidates...)
		job.Result = map[string]any{"eligibility": eligibility.Status, "tool_calls": trace.ToolCallCount}
		job.Progress = 45
	})
	if pending.Input.Scenario == nil {
		s.updateJob(id, func(job *AsyncJob) {
			job.Status, job.Phase, job.Progress = JobWaitingForScenario, "scenario_review", 50
		})
		return
	}
	// Callers that supply a scenario still pass through the same confirmation
	// validator; the HTTP/UI flow normally calls ConfirmScenario separately.
	selection := TraceScenarioSelection{CandidateID: pending.Input.Scenario.CandidateID,
		Correction: &TraceScenarioCorrection{Task: pending.Input.Scenario.Task,
			AcceptedOutcome: pending.Input.Scenario.AcceptedOutcome,
			ActionIndexes:   append([]int(nil), pending.Input.Scenario.ActionIndexes...),
			Boundaries:      append([]string(nil), pending.Input.Scenario.Boundaries...)}}
	if _, err := ConfirmScenario(candidates, selection); err != nil {
		s.failJob(id, err)
		return
	}
	s.runDistillationSynthesis(id, ctx)
}

// ConfirmScenario resumes a waiting distillation job after validating the
// selected candidate or operator correction.
func (s *Service) ConfirmScenario(id string, selection TraceScenarioSelection) (AsyncJob, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return AsyncJob{}, err
	}
	if job.Kind != JobDistillation || job.Status != JobWaitingForScenario {
		return AsyncJob{}, fmt.Errorf("distillation job %q is not waiting for a scenario", id)
	}
	s.mu.Lock()
	pending, ok := s.pending[id]
	s.mu.Unlock()
	if !ok {
		return AsyncJob{}, errors.New("distillation scenario state is unavailable")
	}
	scenario, err := ConfirmScenario(pending.Candidates, selection)
	if err != nil {
		return AsyncJob{}, err
	}
	pending.Input.Trace = &pending.Trace
	pending.Input.Scenario = &scenario
	s.mu.Lock()
	s.pending[id] = pending
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel == nil {
		return AsyncJob{}, errors.New("distillation job is no longer active")
	}
	s.updateJob(id, func(item *AsyncJob) {
		item.Status, item.Phase, item.Progress = JobRunning, "synthesizing_workflow", 60
	})
	// The original context is cancelled only by the explicit job cancel path;
	// use a fresh worker context so a completed trace-analysis goroutine does
	// not accidentally terminate synthesis.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[id] = workerCancel
	s.mu.Unlock()
	s.launchWorker(id, func() { s.runDistillationSynthesis(id, workerCtx) })
	return s.GetJob(id)
}

func (s *Service) runDistillationSynthesis(id string, ctx context.Context) {
	s.mu.Lock()
	pending, ok := s.pending[id]
	s.mu.Unlock()
	if !ok {
		s.failJob(id, errors.New("distillation input is unavailable"))
		return
	}
	if err := ctx.Err(); err != nil {
		s.finishContextJob(id, err)
		return
	}
	app, evidence, err := Distill(pending.Input)
	if err == nil {
		err = s.store.CreateDraft(app, &evidence)
	}
	if err != nil {
		if ctx.Err() != nil {
			s.finishContextJob(id, ctx.Err())
		} else {
			s.failJob(id, err)
		}
		return
	}
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress = JobSucceeded, "draft_ready", 100
		job.AppID, job.Summary = app.ID, DistillationSummary(app)
		job.Result = map[string]any{"app_id": app.ID, "revision": app.Revision}
	})
	s.clearActive(id)
}

func (s *Service) StartTestRun(appID string, inputs map[string]any, decisions *OperatorDecisions) (AsyncJob, error) {
	if s == nil || s.store == nil || s.runner == nil {
		return AsyncJob{}, errors.New("mini app service is unavailable")
	}
	app, err := s.store.GetDraft(strings.TrimSpace(appID))
	if err != nil {
		return AsyncJob{}, err
	}
	evidence, err := s.store.GetSourceEvidence(app.ID)
	if err != nil {
		return AsyncJob{}, err
	}
	inputs = ReplayInputs(app, evidence, inputs)
	job, ctx := s.newJob(JobTestRun, "", app.ID, "")
	job.Revision = app.Revision
	s.updateJob(job.ID, func(item *AsyncJob) { item.Revision = app.Revision; item.Phase = "queued" })
	s.mu.Lock()
	s.runs[job.ID] = pendingRun{kind: JobTestRun, app: app, appID: app.ID, inputs: cloneMap(inputs), decisions: cloneDecisions(decisions), evidence: evidence}
	s.mu.Unlock()
	s.launchWorker(job.ID, func() { s.runTestJob(ctx, job.ID, app, inputs, decisions, evidence) })
	return s.GetJob(job.ID)
}

func (s *Service) runTestJob(ctx context.Context, id string, app MiniApp, inputs map[string]any, decisions *OperatorDecisions, evidence SourceEvidence) {
	s.updateJob(id, func(job *AsyncJob) { job.Status, job.Phase, job.Progress = JobRunning, "running_draft", 10 })
	run, err := s.runner.RunPortableWithFixture(ctx, app, inputs, decisions, evidence.FixtureFiles)
	s.updateJob(id, func(job *AsyncJob) { job.RunID, job.Progress = run.ID, 70 })
	if err != nil && strings.TrimSpace(run.Error) != "" {
		err = errors.New(run.Error)
	}
	if err != nil && run.Status == RunSucceeded {
		s.failJob(id, err)
		return
	}
	report := VerifyReplay(ctx, app, evidence, run)
	s.updateJob(id, func(job *AsyncJob) { job.Report = &report })
	if err != nil || !report.Passed {
		if ctx.Err() != nil || run.Status == RunCancelled {
			s.finishContextJob(id, ctx.Err())
		} else if run.Status == RunWaitingForConfirmation {
			confirmation := s.pendingConfirmation(app, run, inputs)
			s.updateJob(id, func(job *AsyncJob) {
				job.Status, job.Phase, job.Confirmation = JobWaitingForConfirm, "awaiting_confirmation", confirmation
			})
		} else if run.Status == RunWaitingForInput {
			s.updateJob(id, func(job *AsyncJob) { job.Status, job.Phase = JobWaitingForInput, "awaiting_input" })
		} else {
			if err == nil {
				err = errors.New("same-data verification failed")
			}
			s.failJob(id, err)
		}
		return
	}
	if err := s.store.RecordPassingTest(app.ID, run); err != nil {
		s.failJob(id, err)
		return
	}
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress = JobSucceeded, "verified", 100
		job.Result = map[string]any{"run_id": run.ID, "revision": run.Revision}
	})
	s.clearActive(id)
}

func (s *Service) StartReleaseRun(appID, version string, inputs map[string]any, decisions *OperatorDecisions) (AsyncJob, error) {
	if s == nil || s.store == nil || s.runner == nil {
		return AsyncJob{}, errors.New("mini app service is unavailable")
	}
	app, err := s.store.GetRelease(strings.TrimSpace(appID), strings.TrimSpace(version))
	if err != nil {
		return AsyncJob{}, err
	}
	job, ctx := s.newJob(JobReleaseRun, "", app.ID, app.Version)
	job.Revision = app.Revision
	s.updateJob(job.ID, func(item *AsyncJob) { item.Revision = app.Revision; item.Phase = "queued" })
	s.mu.Lock()
	s.runs[job.ID] = pendingRun{kind: JobReleaseRun, app: app, appID: app.ID, version: app.Version, inputs: cloneMap(inputs), decisions: cloneDecisions(decisions)}
	s.mu.Unlock()
	s.launchWorker(job.ID, func() { s.runReleaseJob(ctx, job.ID, app, inputs, decisions) })
	return s.GetJob(job.ID)
}

func (s *Service) runReleaseJob(ctx context.Context, id string, app MiniApp, inputs map[string]any, decisions *OperatorDecisions) {
	s.updateJob(id, func(job *AsyncJob) { job.Status, job.Phase, job.Progress = JobRunning, "running_release", 10 })
	run, err := s.runner.RunPortable(ctx, app, inputs, decisions)
	s.updateJob(id, func(job *AsyncJob) { job.RunID, job.Progress = run.ID, 80 })
	if err != nil && strings.TrimSpace(run.Error) != "" {
		err = errors.New(run.Error)
	}
	if err != nil {
		if ctx.Err() != nil || run.Status == RunCancelled {
			s.finishContextJob(id, ctx.Err())
		} else if run.Status == RunWaitingForConfirmation {
			confirmation := s.pendingConfirmation(app, run, inputs)
			s.updateJob(id, func(job *AsyncJob) {
				job.Status, job.Phase, job.Confirmation = JobWaitingForConfirm, "awaiting_confirmation", confirmation
			})
		} else if run.Status == RunWaitingForInput {
			s.updateJob(id, func(job *AsyncJob) { job.Status, job.Phase = JobWaitingForInput, "awaiting_input" })
		} else {
			s.failJob(id, err)
		}
		return
	}
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress = JobSucceeded, "completed", 100
		job.Result = map[string]any{"run_id": run.ID, "revision": run.Revision}
	})
	s.clearActive(id)
}

// ConfirmRun resumes a run that paused at an explicit confirm step. The
// original app/version/inputs are retained in memory until the job reaches a
// terminal state; the caller supplies only the new operator decisions.
func (s *Service) ConfirmRun(id string, decisions *OperatorDecisions) (AsyncJob, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return AsyncJob{}, err
	}
	if job.Kind != JobTestRun && job.Kind != JobReleaseRun {
		return AsyncJob{}, fmt.Errorf("job %q is not a run", id)
	}
	if job.Status != JobWaitingForConfirm {
		return AsyncJob{}, fmt.Errorf("run job %q is not waiting for confirmation", id)
	}
	s.mu.Lock()
	pending, ok := s.runs[id]
	s.mu.Unlock()
	if !ok {
		return AsyncJob{}, errors.New("run continuation state is unavailable")
	}
	if job.Confirmation == nil || decisions == nil {
		return AsyncJob{}, errors.New("a pending confirmation decision is required")
	}
	if _, decided := decisions.Confirmations[job.Confirmation.ID]; !decided {
		return AsyncJob{}, fmt.Errorf("confirmation %q decision is required", job.Confirmation.ID)
	}
	pending.decisions = mergeDecisions(pending.decisions, decisions)
	s.mu.Lock()
	s.runs[id] = pending
	s.mu.Unlock()
	workerCtx, workerCancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[id] = workerCancel
	s.mu.Unlock()
	s.updateJob(id, func(item *AsyncJob) {
		item.Status, item.Phase, item.Progress, item.Error, item.Confirmation = JobRunning, "resuming_after_confirmation", 10, "", nil
	})
	if pending.kind == JobTestRun {
		s.launchWorker(id, func() {
			s.runTestJob(workerCtx, id, pending.app, pending.inputs, pending.decisions, pending.evidence)
		})
	} else {
		s.launchWorker(id, func() {
			s.runReleaseJob(workerCtx, id, pending.app, pending.inputs, pending.decisions)
		})
	}
	return s.GetJob(id)
}

func (s *Service) pendingConfirmation(app MiniApp, run Run, inputs map[string]any) *PendingConfirmation {
	id := ""
	for stepID, result := range run.Steps {
		if result.Status == RunWaitingForConfirmation {
			id = stepID
			break
		}
	}
	if id == "" {
		return nil
	}
	step, found := s.findConfirmationStep(app.Workflow, id, "", 0)
	confirmation := &PendingConfirmation{ID: id, Message: "Approve this workflow action?"}
	if !found {
		return confirmation
	}
	refs := map[string]any{"inputs": inputs, "steps": map[string]any{}, "app": map[string]any{"id": app.ID, "version": app.Version}}
	if message, err := ResolveValue(step.Message, refs); err == nil {
		confirmation.Message = redactString(fmt.Sprint(message), nil)
	}
	if details, err := ResolveValue(step.Details, refs); err == nil {
		confirmation.Details = redactValue(details, nil, false)
	}
	return confirmation
}

func (s *Service) findConfirmationStep(steps []Step, want, prefix string, depth int) (Step, bool) {
	for _, step := range steps {
		if step.Kind == "confirm" && prefix+step.ID == want {
			return step, true
		}
		if found, ok := s.findConfirmationStep(step.Then, want, prefix, depth); ok {
			return found, true
		}
		if found, ok := s.findConfirmationStep(step.Else, want, prefix, depth); ok {
			return found, true
		}
		if step.Kind == "miniapp" && depth < maxMiniAppNesting && s.store != nil {
			if nested, err := s.store.GetRelease(step.AppID, step.AppVersion); err == nil {
				if found, ok := s.findConfirmationStep(nested.Workflow, want, prefix+step.ID+".", depth+1); ok {
					return found, true
				}
			}
		}
	}
	return Step{}, false
}

func (s *Service) newJob(kind JobKind, sessionID, appID, version string) (AsyncJob, context.Context) {
	now := time.Now().UTC()
	job := AsyncJob{ID: newID("job"), Kind: kind, SessionID: sessionID, AppID: appID,
		Version: version, Status: JobQueued, Phase: "queued", CreatedAt: now, UpdatedAt: now}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.cancels[job.ID] = cancel
	s.mu.Unlock()
	s.persistJob(job)
	s.emitJobEvent(job, "job.queued", "queued", nil)
	return job, ctx
}

func (s *Service) launchWorker(id string, work func()) {
	done := make(chan struct{})
	s.mu.Lock()
	s.workers[id] = done
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			if current, ok := s.workers[id]; ok && current == done {
				delete(s.workers, id)
			}
			s.mu.Unlock()
			close(done)
		}()
		work()
	}()
}

func (s *Service) updateJob(id string, update func(*AsyncJob)) {
	s.mu.Lock()
	job, ok := s.jobs[id]
	if !ok {
		loaded, err := s.loadJobLocked(id)
		if err != nil {
			s.mu.Unlock()
			return
		}
		job, ok = loaded, true
	}
	if ok && job.Status.terminal() {
		s.mu.Unlock()
		return
	}
	update(&job)
	job.UpdatedAt = time.Now().UTC()
	// Publish the new state before releasing the lock. Persisting while this
	// critical section is held preserves update order on disk as well as in
	// memory; job JSON is small and writes are bounded local I/O.
	s.jobs[id] = job
	s.persistJob(job)
	s.appendJobEventLocked(job, "job.updated", job.Phase, nil)
	s.mu.Unlock()
}

func (s *Service) failJob(id string, err error) {
	message := "job failed"
	if err != nil {
		message = redactString(err.Error(), nil)
	}
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress, job.Error = JobFailed, "failed", 100, message
	})
	s.clearActive(id)
}

func (s *Service) finishContextJob(id string, err error) {
	status := JobCancelled
	phase := "cancelled"
	if errors.Is(err, context.DeadlineExceeded) {
		status, phase = JobInterrupted, "interrupted"
	}
	s.updateJob(id, func(job *AsyncJob) {
		job.Status, job.Phase, job.Progress = status, phase, 100
		if err != nil {
			job.Error = redactString(err.Error(), nil)
		}
	})
	s.clearActive(id)
}

func (s *Service) clearActive(id string) {
	s.mu.Lock()
	delete(s.cancels, id)
	delete(s.pending, id)
	delete(s.runs, id)
	s.mu.Unlock()
}

// Close cancels active work as an interrupted shutdown. Unlike Cancel, it
// does not present a user-requested cancellation to operators.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.cancels))
	cancels := make(map[string]context.CancelFunc, len(s.cancels))
	workers := make(map[string]chan struct{}, len(s.workers))
	for id, cancel := range s.cancels {
		ids = append(ids, id)
		cancels[id] = cancel
	}
	for id, done := range s.workers {
		workers[id] = done
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.updateJob(id, func(job *AsyncJob) {
			if !job.Status.terminal() {
				job.Status, job.Phase, job.Progress = JobInterrupted, "interrupted", 100
				job.Error = "job interrupted by service shutdown"
			}
		})
	}
	for _, cancel := range cancels {
		if cancel != nil {
			cancel()
		}
	}
	for _, done := range workers {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	s.mu.Lock()
	s.cancels = make(map[string]context.CancelFunc)
	s.pending = make(map[string]pendingDistillation)
	s.runs = make(map[string]pendingRun)
	s.mu.Unlock()
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = deepCopyValue(value)
	}
	return output
}

func cloneDecisions(input *OperatorDecisions) *OperatorDecisions {
	if input == nil {
		return nil
	}
	output := &OperatorDecisions{Confirmations: make(map[string]bool, len(input.Confirmations))}
	for key, value := range input.Confirmations {
		output.Confirmations[key] = value
	}
	return output
}

func mergeDecisions(base, additions *OperatorDecisions) *OperatorDecisions {
	merged := cloneDecisions(base)
	if merged == nil {
		merged = &OperatorDecisions{Confirmations: map[string]bool{}}
	}
	if merged.Confirmations == nil {
		merged.Confirmations = map[string]bool{}
	}
	if additions != nil {
		for id, approved := range additions.Confirmations {
			merged.Confirmations[id] = approved
		}
	}
	return merged
}

// Cancel requests cancellation and immediately records a terminal status.
// The runner still receives the context cancellation and persists its own
// final run record; late worker updates cannot overwrite this terminal job.
func (s *Service) Cancel(id string) (AsyncJob, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return AsyncJob{}, err
	}
	if job.Status.terminal() {
		return job, nil
	}
	s.mu.Lock()
	cancel := s.cancels[id]
	done := s.workers[id]
	s.mu.Unlock()
	s.updateJob(id, func(item *AsyncJob) {
		item.Status, item.Phase, item.Progress = JobCancelled, "cancelled", 100
	})
	if cancel != nil {
		cancel()
	}
	s.clearActive(id)
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	return s.GetJob(id)
}

func (s *Service) GetJob(id string) (AsyncJob, error) {
	id = strings.TrimSpace(id)
	if !portableIDPattern.MatchString(id) {
		return AsyncJob{}, ErrInvalidIdentifier
	}
	s.mu.Lock()
	if job, ok := s.jobs[id]; ok {
		s.mu.Unlock()
		return job, nil
	}
	job, err := s.loadJobLocked(id)
	if err == nil {
		s.jobs[id] = job
	}
	s.mu.Unlock()
	return job, err
}

// Events returns persisted history after the supplied sequence number. The
// returned slice is a copy and is safe for an SSE writer to retain.
func (s *Service) Events(id string, after uint64) ([]JobEvent, error) {
	if _, err := s.GetJob(id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	items := append([]JobEvent(nil), s.events[id]...)
	s.mu.Unlock()
	if len(items) == 0 {
		items = readJobEvents(s.eventsPath(id))
	}
	filtered := items[:0]
	for _, item := range items {
		if item.Seq > after {
			filtered = append(filtered, item)
		}
	}
	return append([]JobEvent(nil), filtered...), nil
}

// RecoverInterruptedJobs marks queued/running/waiting records from a previous
// process as interrupted. In-memory continuation state is intentionally not
// reconstructed from disk, so an operator must start a fresh distillation.
func (s *Service) RecoverInterruptedJobs() int {
	if s == nil || s.jobDir() == "" {
		return 0
	}
	entries, err := os.ReadDir(s.jobDir())
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".events.json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		job, err := readJobFile(s.jobPath(id))
		if err != nil {
			continue
		}
		s.mu.Lock()
		s.jobs[id] = job
		s.mu.Unlock()
		if job.Status == JobQueued || job.Status == JobRunning || job.Status == JobWaitingForScenario || job.Status == JobWaitingForInput || job.Status == JobWaitingForConfirm {
			job.Status, job.Phase, job.Progress = JobInterrupted, "interrupted", 100
			job.Error = "job was interrupted by process shutdown"
			job.UpdatedAt = time.Now().UTC()
			s.mu.Lock()
			s.jobs[id] = job
			s.mu.Unlock()
			s.persistJob(job)
			s.emitJobEvent(job, "job.interrupted", job.Phase, nil)
			count++
		}
	}
	return count
}

func (s *Service) persistJob(job AsyncJob) {
	if s == nil || s.jobDir() == "" {
		return
	}
	_ = writeJSONAtomic(s.jobPath(job.ID), job, 0o600)
	if job.Kind == JobDistillation {
		status := DistillationFailed
		switch job.Status {
		case JobQueued:
			status = DistillationQueued
		case JobRunning:
			status = DistillationAnalyzing
		case JobWaitingForScenario:
			status = DistillationWaitingScenario
		case JobSucceeded:
			status = DistillationCompleted
		case JobCancelled:
			status = DistillationCancelled
		}
		_ = s.store.SaveDistillation(DistillationJob{ID: job.ID, SessionID: job.SessionID,
			Status: status, Phase: job.Phase, Progress: job.Progress, AppID: job.AppID,
			Summary: job.Summary, Error: job.Error, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt})
	}
}

func (s *Service) emitJobEvent(job AsyncJob, eventType, phase string, data map[string]any) {
	s.mu.Lock()
	s.appendJobEventLocked(job, eventType, phase, data)
	s.mu.Unlock()
}

func (s *Service) appendJobEventLocked(job AsyncJob, eventType, phase string, data map[string]any) {
	if data != nil {
		data = redactValueMap(data, nil)
	}
	event := JobEvent{JobID: job.ID, Kind: job.Kind, Type: eventType, Status: job.Status,
		Phase: phase, Progress: job.Progress, Message: job.Error, Data: data, Timestamp: time.Now().UTC()}
	history := s.events[job.ID]
	if len(history) == 0 {
		history = readJobEvents(s.eventsPath(job.ID))
	}
	event.Seq = uint64(len(history) + 1)
	s.events[job.ID] = append(history, event)
	if path := s.eventsPath(job.ID); path != "" {
		if file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			_, _ = file.WriteString(mustJSONLine(event))
			_ = file.Close()
		}
	}
}

func (s *Service) loadJobLocked(id string) (AsyncJob, error) {
	if s.jobDir() == "" {
		return AsyncJob{}, ErrNotFound
	}
	return readJobFile(s.jobPath(id))
}

func readJobFile(path string) (AsyncJob, error) {
	var job AsyncJob
	if err := readJSON(path, &job); err != nil {
		if os.IsNotExist(err) {
			return AsyncJob{}, ErrNotFound
		}
		return AsyncJob{}, err
	}
	return job, nil
}

func readJobEvents(path string) []JobEvent {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()
	var events []JobEvent
	decoder := json.NewDecoder(bufio.NewReader(file))
	for {
		var event JobEvent
		if decoder.Decode(&event) != nil {
			break
		}
		events = append(events, event)
	}
	return events
}

// The small wrappers below keep service.go independent from the HTTP JSON
// encoder while still making event persistence line-oriented.
func mustJSONLine(value any) string {
	data, _ := json.Marshal(value)
	return string(append(data, '\n'))
}
