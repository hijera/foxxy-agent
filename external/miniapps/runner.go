//go:build miniapps

package miniapps

import (
	"github.com/hijera/foxxycode-agent/internal/cmdprofile"

	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// AppStore is the storage contract required by the runtime. Store implements
// this interface; keeping the runtime on the narrow contract makes execution
// straightforward to test without a filesystem-backed catalog.
type AppStore interface {
	GetDraft(string) (MiniApp, error)
	GetRelease(string, string) (MiniApp, error)
	SaveRun(Run) error
}

// ToolRequest is the sanitized runtime boundary for deterministic tools.
// Arguments are already resolved against the current run references.
type ToolRequest struct {
	AppID     string
	RunID     string
	StepID    string
	Tool      string
	Arguments any
	Workspace string
	// CommandProfile is the document-embedded profile backing a cmd_* step,
	// attached by the runner from app.Requirements.Commands. Nil for ordinary
	// builtins and for cmd_* tools resolved from the live registry.
	CommandProfile *cmdprofile.ProfileSpec
}

// ModelRequest is a tool-free model invocation.
type ModelRequest struct {
	AppID        string
	RunID        string
	StepID       string
	Binding      ModelBinding
	Prompt       string
	OutputSchema map[string]any
}

// AgentRequest is a bounded ReAct invocation. Tools is an explicit allowlist;
// an empty list is intentionally not widened by the runtime.
type AgentRequest struct {
	AppID        string
	RunID        string
	StepID       string
	Binding      ModelBinding
	Prompt       string
	Tools        []string
	MaxTurns     int
	OutputSchema map[string]any
	Workspace    string
}

type ToolExecutor interface {
	ExecuteTool(context.Context, ToolRequest) (any, error)
}

type ModelExecutor interface {
	ExecuteModel(context.Context, ModelRequest) (any, error)
}

type AgentExecutor interface {
	ExecuteAgent(context.Context, AgentRequest) (any, error)
}

// RuntimeCapabilityChecker lets concrete adapters reject unavailable
// dependencies before the runner creates a workspace or executes any step.
type RuntimeCapabilityChecker interface {
	ValidateMiniAppCapabilities(MiniApp) error
}

// RunEvent is the operator-safe incremental event contract. Data must never
// contain raw secret input values; Runner sanitizes it before emission.
type RunEvent struct {
	Type      string         `json:"type"`
	RunID     string         `json:"run_id"`
	AppID     string         `json:"app_id"`
	StepID    string         `json:"step_id,omitempty"`
	Status    RunStatus      `json:"status,omitempty"`
	Attempt   int            `json:"attempt,omitempty"`
	Message   string         `json:"message,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type EventSink interface {
	Emit(context.Context, RunEvent) error
}

type EventSinkFunc func(context.Context, RunEvent) error

func (f EventSinkFunc) Emit(ctx context.Context, event RunEvent) error {
	if f == nil {
		return nil
	}
	return f(ctx, event)
}

// Executors groups the capabilities a Mini App may invoke. A missing
// capability fails closed when a corresponding step is encountered.
type Executors struct {
	Tool   ToolExecutor
	Model  ModelExecutor
	Agent  AgentExecutor
	Events EventSink
}

type OperatorDecisions struct {
	Confirmations map[string]bool `json:"confirmations,omitempty"`
}

// maxMiniAppNesting bounds nested release composition, including self-calls.
const maxMiniAppNesting = 8

type miniAppNestingKey struct{}

type Runner struct {
	store         AppStore
	executors     Executors
	workspaceRoot string
	pausedMu      sync.Mutex
	paused        map[string]*runExecution
}

func NewRunner(store AppStore, executors Executors) *Runner {
	return &Runner{store: store, executors: executors}
}

// WithWorkspaceRoot places run workspaces below root. It is useful for an
// operator-controlled scratch volume; the default derives a private root from
// the configured Store when possible.
func (r *Runner) WithWorkspaceRoot(root string) *Runner {
	if r != nil {
		r.workspaceRoot = strings.TrimSpace(root)
	}
	return r
}

// Paused reports whether a run has an in-memory continuation. Preflight
// confirmations retain the same run id without starting any workflow steps.
func (r *Runner) Paused(runID string) bool {
	if r == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	r.pausedMu.Lock()
	defer r.pausedMu.Unlock()
	_, ok := r.paused[runID]
	return ok
}

// Discard drops a paused continuation and its temporary workspace.
func (r *Runner) Discard(runID string) {
	if r == nil || strings.TrimSpace(runID) == "" {
		return
	}
	r.pausedMu.Lock()
	exec := r.paused[runID]
	delete(r.paused, runID)
	r.pausedMu.Unlock()
	if exec != nil {
		if exec.run.Status == RunWaitingForConfirmation {
			exec.run.Status = RunCancelled
			exec.run.FinishedAt = time.Now().UTC()
			if r.store != nil {
				_ = r.store.SaveRun(*exec.run)
			}
		}
		_ = os.RemoveAll(exec.workspace)
		for _, nestedRunID := range exec.nestedRuns {
			r.Discard(nestedRunID)
		}
	}
}

func (r *Runner) takePaused(runID string) *runExecution {
	r.pausedMu.Lock()
	defer r.pausedMu.Unlock()
	exec := r.paused[runID]
	delete(r.paused, runID)
	return exec
}

func (r *Runner) rememberPaused(exec *runExecution) {
	if r == nil || exec == nil || exec.run == nil {
		return
	}
	r.pausedMu.Lock()
	if r.paused == nil {
		r.paused = make(map[string]*runExecution)
	}
	r.paused[exec.run.ID] = exec
	r.pausedMu.Unlock()
}

func (r *Runner) RunDraft(ctx context.Context, id string, inputs map[string]any, decisions *OperatorDecisions) (Run, error) {
	if r == nil || r.store == nil {
		return Run{}, errors.New("mini app store is unavailable")
	}
	app, err := r.store.GetDraft(strings.TrimSpace(id))
	if err != nil {
		return Run{}, err
	}
	return r.runAndPersist(ctx, app, inputs, decisions, true, nil)
}

func (r *Runner) RunRelease(ctx context.Context, id, version string, inputs map[string]any, decisions *OperatorDecisions) (Run, error) {
	if r == nil || r.store == nil {
		return Run{}, errors.New("mini app store is unavailable")
	}
	app, err := r.store.GetRelease(strings.TrimSpace(id), strings.TrimSpace(version))
	if err != nil {
		return Run{}, err
	}
	return r.runAndPersist(ctx, app, inputs, decisions, false, nil)
}

// RunPortable executes a supplied draft/release document. It still persists a
// run when a Store is configured, which lets verification use the exact same
// runtime path as an operator run.
func (r *Runner) RunPortable(ctx context.Context, app MiniApp, inputs map[string]any, decisions *OperatorDecisions) (Run, error) {
	if r == nil {
		return Run{}, errors.New("mini app runner is nil")
	}
	return r.runAndPersist(ctx, app, inputs, decisions, app.State != StateReleased, nil)
}

// RunPortableWithFixture executes a pinned draft snapshot after materializing
// its private, sanitized source fixture into the isolated run workspace.
func (r *Runner) RunPortableWithFixture(ctx context.Context, app MiniApp, inputs map[string]any, decisions *OperatorDecisions, fixture map[string][]byte) (Run, error) {
	if r == nil {
		return Run{}, errors.New("mini app runner is nil")
	}
	return r.runAndPersist(ctx, app, inputs, decisions, app.State != StateReleased, fixture)
}

// Resume continues a run whose confirmation became reachable only after an
// earlier step produced a value. Successful steps and the workspace are kept,
// so answering the operator never repeats their side effects.
func (r *Runner) Resume(ctx context.Context, runID string, decisions *OperatorDecisions) (Run, error) {
	if r == nil {
		return Run{}, errors.New("mini app runner is nil")
	}
	exec := r.takePaused(strings.TrimSpace(runID))
	if exec == nil {
		return Run{}, fmt.Errorf("%w: paused run %s", ErrNotFound, runID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if decisions == nil {
		decisions = &OperatorDecisions{}
	}
	if exec.workspace == "" {
		run, err := r.runWithID(ctx, exec.app, exec.inputs, decisions, exec.run.Test, exec.fixture, exec.run.ID)
		run.StartedAt = exec.run.StartedAt
		return r.persistResult(run, err)
	}
	exec.ctx, exec.decisions = ctx, decisions
	exec.run.Status, exec.run.Error, exec.run.FinishedAt = RunRunning, "", time.Time{}
	defer func() {
		if exec.run.Status != RunSucceeded && exec.run.Status != RunWaitingForConfirmation {
			_ = os.RemoveAll(exec.workspace)
		}
	}()
	if err := exec.emit(ctx, RunEvent{Type: "run.resumed", Status: RunRunning}); err != nil {
		run, finishErr := exec.finish(err)
		return r.persistResult(run, finishErr)
	}
	if err := exec.executeSteps(exec.app.Workflow); err != nil {
		run, finishErr := exec.finish(err)
		if run.Status == RunWaitingForConfirmation {
			r.rememberPaused(exec)
		}
		return r.persistResult(run, finishErr)
	}
	run, err := exec.complete()
	return r.persistResult(run, err)
}

func (r *Runner) runAndPersist(ctx context.Context, app MiniApp, inputs map[string]any, decisions *OperatorDecisions, test bool, fixture map[string][]byte) (Run, error) {
	run, err := r.run(ctx, app, inputs, decisions, test, fixture)
	return r.persistResult(run, err)
}

func (r *Runner) persistResult(run Run, err error) (Run, error) {
	if r.store != nil {
		if saveErr := r.store.SaveRun(run); err == nil && saveErr != nil {
			err = saveErr
			run.Status = RunFailed
			run.Error = redactString(saveErr.Error(), nil)
		}
	}
	return run, err
}

type runExecution struct {
	inputs     map[string]any
	fixture    map[string][]byte
	runner     *Runner
	ctx        context.Context
	app        MiniApp
	run        *Run
	refs       map[string]any
	decisions  *OperatorDecisions
	secretVals []string
	workspace  string
	eventsPath string
	nestedRuns map[string]string
	persistErr error
	fileMu     sync.Mutex
}

func (r *Runner) run(ctx context.Context, app MiniApp, inputs map[string]any, decisions *OperatorDecisions, test bool, fixture map[string][]byte) (Run, error) {
	return r.runWithID(ctx, app, inputs, decisions, test, fixture, "")
}

func (r *Runner) runWithID(ctx context.Context, app MiniApp, inputs map[string]any, decisions *OperatorDecisions, test bool, fixture map[string][]byte, id string) (Run, error) {
	if id == "" {
		id = newID("run")
	}
	run := Run{
		ID: id, AppID: app.ID, Version: app.Version, Revision: app.Revision,
		Test: test, Status: RunRunning, Inputs: map[string]any{}, Steps: map[string]StepResult{},
		StartedAt: time.Now().UTC(),
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if decisions == nil {
		decisions = &OperatorDecisions{}
	}
	if err := ctx.Err(); err != nil {
		return finishMiniAppRun(run, err, nil)
	}
	if report := Validate(app); !report.Valid {
		return finishMiniAppRun(run, fmt.Errorf("%w: %s", ErrInvalid, report.Issues[0].Message), nil)
	}
	for _, executor := range []any{r.executors.Tool, r.executors.Model, r.executors.Agent} {
		if checker, ok := executor.(RuntimeCapabilityChecker); ok && checker != nil {
			if err := checker.ValidateMiniAppCapabilities(app); err != nil {
				return finishMiniAppRun(run, fmt.Errorf("%w: %v", ErrInvalid, err), nil)
			}
		}
	}

	values := make(map[string]any, len(app.Inputs))
	secretVals := make([]string, 0)
	for _, input := range app.Inputs {
		value, ok := inputs[input.ID]
		if !ok && input.Default != nil {
			value = deepCopyValue(input.Default)
			ok = true
		}
		if ok {
			values[input.ID] = value
			if input.Type == "secret" && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
				secretVals = append(secretVals, fmt.Sprint(value))
			}
		}
	}
	uploads, uploadErr := r.prepareUploadedInputs(app, values)
	if uploadErr != nil {
		return finishMiniAppRun(run, uploadErr, secretVals)
	}
	inputRefs := map[string]any{"inputs": values}
	if err := validateInputs(app.Inputs, values, inputRefs); err != nil {
		return finishMiniAppRun(run, err, secretVals)
	}
	run.Inputs = redactValueMap(values, secretVals)

	preflightRefs := map[string]any{"inputs": values, "steps": map[string]any{}, "app": map[string]any{"id": app.ID, "version": app.Version}}
	if stepID, missing, err := r.firstMissingConfirmation(app.Workflow, app.Requirements.Commands, decisions, "", 0, preflightRefs); err != nil {
		return finishMiniAppRun(run, err, secretVals)
	} else if missing {
		now := time.Now().UTC()
		run.Steps[stepID] = StepResult{ID: stepID, Status: RunWaitingForConfirmation, StartedAt: now, FinishedAt: now}
		run.Status = RunWaitingForConfirmation
		r.rememberPaused(&runExecution{runner: r, app: app, run: &run, inputs: cloneMap(inputs), fixture: fixture})
		return finishMiniAppRun(run, errWaitingForConfirmation, secretVals)
	}

	workspace, err := r.makeWorkspace(app, run.ID)
	if err != nil {
		return finishMiniAppRun(run, err, secretVals)
	}
	defer func() {
		if run.Status != RunSucceeded && run.Status != RunWaitingForConfirmation {
			_ = os.RemoveAll(workspace)
		}
	}()
	if err := materializeMiniAppFixture(workspace, fixture); err != nil {
		return finishMiniAppRun(run, err, secretVals)
	}
	if err := r.copyUploads(ctx, workspace, uploads); err != nil {
		return finishMiniAppRun(run, err, secretVals)
	}
	runDir := filepath.Dir(workspace)
	outputDir := filepath.Join(runDir, "artifacts")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return finishMiniAppRun(run, err, secretVals)
	}
	run.LogPath = filepath.Join(runDir, "execution.log")
	run.EventsPath = filepath.Join(runDir, "events.jsonl")
	exec := &runExecution{
		runner: r, ctx: ctx, app: app, run: &run, decisions: decisions,
		secretVals: secretVals, workspace: workspace, eventsPath: run.EventsPath,
		nestedRuns: make(map[string]string),
		refs: map[string]any{
			"inputs": values,
			"steps":  map[string]any{},
			"run":    map[string]any{"id": run.ID, "workspace": workspace, "output_dir": outputDir},
			"app":    map[string]any{"id": app.ID, "version": app.Version},
		},
	}
	if err := exec.emit(ctx, RunEvent{Type: "run.started", Status: RunRunning}); err != nil {
		return exec.finish(err)
	}
	if err := exec.executeSteps(app.Workflow); err != nil {
		pausedRun, finishErr := exec.finish(err)
		if pausedRun.Status == RunWaitingForConfirmation {
			r.rememberPaused(exec)
		}
		return pausedRun, finishErr
	}
	return exec.complete()
}

func (e *runExecution) complete() (Run, error) {
	if err := e.evaluateSuccess(); err != nil {
		return e.finish(err)
	}
	outputs := make(map[string]any, len(e.app.Outputs))
	for _, output := range e.app.Outputs {
		value, err := ResolveValue(output.Value, e.refs)
		if err != nil {
			return e.finish(fmt.Errorf("output %s: %w", output.ID, err))
		}
		if err := validateJSONType(value, output.Schema); err != nil {
			return e.finish(fmt.Errorf("output %s: %w", output.ID, err))
		}
		outputs[output.ID] = value
	}
	files, err := e.resultFiles(outputs)
	if err != nil {
		return e.finish(err)
	}
	e.run.Files = files
	e.run.Outputs = redactValueMap(outputs, e.secretVals)
	artifacts, err := collectRunArtifacts(e.workspace)
	if err != nil {
		return e.finish(err)
	}
	e.run.Artifacts = artifacts
	for index := range e.run.Artifacts {
		e.run.Artifacts[index].Path = redactString(e.run.Artifacts[index].Path, e.secretVals)
	}
	e.run.Status = RunSucceeded
	e.run.FinishedAt = time.Now().UTC()
	if err := e.emit(e.ctx, RunEvent{Type: "run.succeeded", Status: RunSucceeded, Data: map[string]any{"outputs": outputs}}); err != nil {
		return e.finish(err)
	}
	if e.persistErr != nil {
		return e.finish(e.persistErr)
	}
	return *e.run, nil
}

func materializeMiniAppFixture(workspace string, fixture map[string][]byte) error {
	for name, content := range fixture {
		clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(name)))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.VolumeName(clean) != "" {
			return fmt.Errorf("fixture path %q escapes the run workspace", name)
		}
		target := filepath.Join(workspace, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) firstMissingConfirmation(steps []Step, commands []cmdprofile.ProfileSpec, decisions *OperatorDecisions, prefix string, depth int, refs map[string]any) (string, bool, error) {
	for _, step := range steps {
		if step.When != nil {
			selected, err := ExecuteCondition(context.Background(), *step.When, refs)
			if err != nil || !selected {
				// A condition that needs a previous output becomes decidable only
				// during execution. Its checkpoint is handled by Resume instead of
				// asking for approval before the operator can know the branch.
				continue
			}
		}
		if step.Kind == "confirm" {
			decisionID := prefix + step.ID
			if decisions == nil || decisions.Confirmations == nil {
				return decisionID, true, nil
			}
			approved, decided := decisions.Confirmations[decisionID]
			if !decided {
				return decisionID, true, nil
			}
			if !approved {
				return decisionID, false, fmt.Errorf("confirmation %q was rejected", decisionID)
			}
		}
		if step.Kind == "branch" {
			if step.If != nil {
				if selected, err := ExecuteCondition(context.Background(), *step.If, refs); err == nil {
					branch := step.Else
					if selected {
						branch = step.Then
					}
					if id, missing, err := r.firstMissingConfirmation(branch, commands, decisions, prefix, depth, refs); err != nil || missing {
						return id, missing, err
					}
				}
			}
			continue
		}
		if step.Kind == "tool" {
			if profile, declared := commandProfileByToolName(commands, step.Tool); declared {
				if checker, ok := r.executors.Tool.(interface {
					PreflightCommandProfile(cmdprofile.ProfileSpec) error
				}); ok {
					if err := checker.PreflightCommandProfile(profile); err != nil {
						if errors.Is(err, errWaitingForConfirmation) {
							return prefix + step.ID, true, nil
						}
						return "", false, err
					}
				}
			}
		}
		if step.Kind == "miniapp" {
			if depth >= maxMiniAppNesting {
				return "", false, fmt.Errorf("mini app nesting limit of %d is exceeded", maxMiniAppNesting)
			}
			if r.store == nil {
				return "", false, errors.New("mini app store is unavailable")
			}
			nested, err := r.store.GetRelease(step.AppID, step.AppVersion)
			if err != nil {
				return "", false, err
			}
			nestedRefs := map[string]any{"inputs": map[string]any{}, "steps": map[string]any{}, "app": map[string]any{"id": nested.ID, "version": nested.Version}}
			if mapped, mapErr := ResolveValue(step.InputMap, refs); mapErr == nil {
				if inputs, ok := mapped.(map[string]any); ok {
					nestedRefs["inputs"] = inputs
				}
			}
			if id, missing, err := r.firstMissingConfirmation(nested.Workflow, nested.Requirements.Commands, decisions, prefix+step.ID+".", depth+1, nestedRefs); err != nil || missing {
				return id, missing, err
			}
		}
	}
	return "", false, nil
}

const (
	maxRunArtifactBytes = MaxUploadBytes
	maxRunArtifactsSize = maxUploadStorageBytes
)

func collectRunArtifacts(workspace string) ([]RunArtifact, error) {
	artifacts := make([]RunArtifact, 0)
	var total int64
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > maxRunArtifactBytes || total+info.Size() > maxRunArtifactsSize {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, err := io.Copy(hash, io.LimitReader(file, maxRunArtifactBytes+1))
		_ = file.Close()
		if err != nil {
			return err
		}
		if size != info.Size() {
			return errors.New("artifact changed while collecting it")
		}
		relative, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, RunArtifact{
			Path: filepath.ToSlash(relative), SHA256: fmt.Sprintf("%x", hash.Sum(nil)), SizeBytes: info.Size(),
		})
		total += info.Size()
		return nil
	})
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, err
}

func (r *Runner) makeWorkspace(app MiniApp, runID string) (string, error) {
	root := strings.TrimSpace(r.workspaceRoot)
	if root == "" {
		if provider, ok := r.store.(interface{ RunRoot() string }); ok {
			root = strings.TrimSpace(provider.RunRoot())
		}
	}
	if root == "" {
		root = os.TempDir()
	}
	path := filepath.Join(filepath.Clean(root), app.ID, "runs", runID, "workspace")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func (e *runExecution) finish(err error) (Run, error) {
	if err == nil {
		return *e.run, nil
	}
	status := RunFailed
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		status = RunCancelled
	}
	if errors.Is(err, errWaitingForConfirmation) {
		status = RunWaitingForConfirmation
	}
	if IsMissingInput(err) {
		status = RunWaitingForInput
	}
	e.run.Status = status
	e.run.Error = redactString(err.Error(), e.secretVals)
	e.run.FinishedAt = time.Now().UTC()
	_ = e.emit(e.ctx, RunEvent{Type: runEventType(status), Status: status, Message: e.run.Error})
	if e.persistErr != nil && err == nil {
		err = e.persistErr
	}
	return *e.run, err
}

func runEventType(status RunStatus) string {
	switch status {
	case RunSucceeded:
		return "run.succeeded"
	case RunCancelled:
		return "run.cancelled"
	case RunWaitingForConfirmation:
		return "run.waiting_for_confirmation"
	case RunInterrupted:
		return "run.interrupted"
	default:
		return "run.failed"
	}
}

func finishMiniAppRun(run Run, err error, secrets []string) (Run, error) {
	if err == nil {
		run.Status = RunSucceeded
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		run.Status = RunCancelled
	} else if IsMissingInput(err) {
		run.Status = RunWaitingForInput
	} else if errors.Is(err, errWaitingForConfirmation) {
		run.Status = RunWaitingForConfirmation
	} else {
		run.Status = RunFailed
	}
	run.Error = redactString(errString(err), secrets)
	run.FinishedAt = time.Now().UTC()
	return run, err
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (e *runExecution) emit(ctx context.Context, event RunEvent) error {
	event.RunID, event.AppID = e.run.ID, e.app.ID
	event.Timestamp = time.Now().UTC()
	if event.Data != nil {
		event.Data = redactValueMap(event.Data, e.secretVals)
	}
	switch e.app.Runtime.DiagnosticToolEvents {
	case "none", "metadata":
		// Status and attempt metadata remain visible, but step payloads never do.
		event.Data = nil
	}
	event.Message = redactString(event.Message, e.secretVals)
	if e.runner.executors.Events != nil {
		// Event sinks are observability extensions; a slow or unavailable sink
		// must not turn an otherwise valid tool run into a side effect rollback.
		// The persisted Run remains authoritative, so the error is dropped.
		_ = e.runner.executors.Events.Emit(ctx, event)
	}
	if e.eventsPath != "" && e.app.Runtime.LogScope != "local" {
		e.fileMu.Lock()
		defer e.fileMu.Unlock()
		if raw, err := json.Marshal(event); err == nil {
			if file, err := os.OpenFile(e.eventsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
				_, _ = file.Write(append(raw, '\n'))
				_ = file.Close()
			}
		}
	}
	if e.runner.store != nil {
		if err := e.runner.store.SaveRun(*e.run); err != nil && e.persistErr == nil {
			e.persistErr = err
		}
	}
	return nil
}

func (e *runExecution) executeSteps(steps []Step) error {
	for _, step := range steps {
		if err := e.ctx.Err(); err != nil {
			return err
		}
		if previous, ok := e.run.Steps[step.ID]; ok && previous.Status == RunSucceeded {
			continue
		}
		if step.When != nil {
			ok, err := ExecuteCondition(e.ctx, *step.When, e.refs)
			if err != nil {
				return fmt.Errorf("step %s condition: %w", step.ID, err)
			}
			if !ok {
				continue
			}
		}
		started := time.Now().UTC()
		result := StepResult{ID: step.ID, Status: RunRunning, StartedAt: started}
		e.run.Steps[step.ID] = result
		_ = e.emit(e.ctx, RunEvent{Type: "step.started", StepID: step.ID, Status: RunRunning})
		attempts := step.Retry.MaxAttempts
		if attempts <= 0 {
			attempts = 1
		}
		var output any
		var execErr error
		for attempt := 1; attempt <= attempts; attempt++ {
			result.Attempts = attempt
			_ = e.emit(e.ctx, RunEvent{Type: "step.attempt", StepID: step.ID, Status: RunRunning, Attempt: attempt})
			stepCtx := e.ctx
			cancel := func() {}
			if step.TimeoutSeconds > 0 {
				stepCtx, cancel = context.WithTimeout(e.ctx, time.Duration(step.TimeoutSeconds)*time.Second)
			}
			output, execErr = e.executeStep(stepCtx, step)
			cancel()
			if execErr == nil {
				break
			}
			if errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) {
				break
			}
			// A confirmation pause is not a transient failure: retrying would
			// re-raise it and burn the attempts without the operator seeing it.
			if errors.Is(execErr, errWaitingForConfirmation) {
				break
			}
			if attempt < attempts && step.Retry.DelayMS > 0 {
				timer := time.NewTimer(time.Duration(step.Retry.DelayMS) * time.Millisecond)
				select {
				case <-e.ctx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					execErr = e.ctx.Err()
				case <-timer.C:
				}
			}
		}
		result.FinishedAt = time.Now().UTC()
		if execErr != nil {
			if errors.Is(execErr, errWaitingForConfirmation) && step.Kind == "branch" {
				delete(e.run.Steps, step.ID)
				return execErr
			}
			result.Status = RunFailed
			if errors.Is(execErr, errWaitingForConfirmation) {
				result.Status = RunWaitingForConfirmation
			}
			result.Error = redactString(execErr.Error(), e.secretVals)
			resultStepID := step.ID
			var nestedPause *nestedConfirmationPause
			if errors.As(execErr, &nestedPause) && nestedPause.stepID != "" {
				resultStepID = nestedPause.stepID
			}
			result.ID = resultStepID
			if resultStepID != step.ID {
				delete(e.run.Steps, step.ID)
			}
			e.run.Steps[resultStepID] = result
			_ = e.emit(e.ctx, RunEvent{Type: "step.failed", StepID: resultStepID, Status: result.Status, Attempt: result.Attempts, Message: result.Error})
			return fmt.Errorf("step %s: %w", step.ID, execErr)
		}
		if err := validateJSONType(output, step.OutputSchema); err != nil {
			result.Status = RunFailed
			result.Error = err.Error()
			result.FinishedAt = time.Now().UTC()
			e.run.Steps[step.ID] = result
			_ = e.emit(e.ctx, RunEvent{Type: "step.failed", StepID: step.ID, Status: RunFailed, Message: result.Error})
			return fmt.Errorf("step %s output: %w", step.ID, err)
		}
		result.Status = RunSucceeded
		result.Outputs = redactValueMap(map[string]any{"result": output}, e.secretVals)
		for id, stepResult := range e.run.Steps {
			if strings.HasPrefix(id, step.ID+".") && stepResult.Status == RunWaitingForConfirmation {
				delete(e.run.Steps, id)
			}
		}
		e.run.Steps[step.ID] = result
		e.refs["steps"].(map[string]any)[step.ID] = map[string]any{
			"outputs": map[string]any{"result": output}, "status": string(result.Status),
		}
		_ = e.emit(e.ctx, RunEvent{Type: "step.succeeded", StepID: step.ID, Status: RunSucceeded, Attempt: result.Attempts, Data: map[string]any{"output": output}})
	}
	return nil
}

type nestedConfirmationPause struct{ stepID string }

func (e *nestedConfirmationPause) Error() string { return errWaitingForConfirmation.Error() }
func (e *nestedConfirmationPause) Unwrap() error { return errWaitingForConfirmation }

func waitingConfirmationStep(steps map[string]StepResult) string {
	ids := make([]string, 0, 1)
	for id, result := range steps {
		if result.Status == RunWaitingForConfirmation {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return ids[0]
}

func (e *runExecution) executeStep(ctx context.Context, step Step) (any, error) {
	switch step.Kind {
	case "tool":
		if e.runner.executors.Tool == nil {
			return nil, errors.New("tool executor is unavailable")
		}
		if !containsString(e.app.Permissions.Tools, step.Tool) {
			return nil, fmt.Errorf("tool %q is not declared in app permissions", step.Tool)
		}
		args, err := ResolveValue(step.Arguments, e.refs)
		if err != nil {
			return nil, err
		}
		request := ToolRequest{AppID: e.app.ID, RunID: e.run.ID, StepID: step.ID, Tool: step.Tool, Arguments: args, Workspace: e.workspace}
		if profile, declared := commandProfileByToolName(e.app.Requirements.Commands, step.Tool); declared {
			attached := profile.Clone()
			request.CommandProfile = &attached
		}
		return e.runner.executors.Tool.ExecuteTool(ctx, request)
	case "llm":
		if e.runner.executors.Model == nil {
			return nil, errors.New("model executor is unavailable")
		}
		binding, err := e.modelBinding(step.ModelBinding)
		if err != nil {
			return nil, err
		}
		if !modelPermissionDeclared(e.app.Permissions.Models, binding) {
			return nil, fmt.Errorf("model binding %q is not declared in app permissions", binding.ID)
		}
		prompt, err := ResolveValue(step.Prompt, e.refs)
		if err != nil {
			return nil, err
		}
		return e.runner.executors.Model.ExecuteModel(ctx, ModelRequest{AppID: e.app.ID, RunID: e.run.ID, StepID: step.ID, Binding: binding, Prompt: fmt.Sprint(prompt), OutputSchema: step.OutputSchema})
	case "agent":
		if e.runner.executors.Agent == nil {
			return nil, errors.New("agent executor is unavailable")
		}
		binding, err := e.modelBinding(step.ModelBinding)
		if err != nil {
			return nil, err
		}
		if !modelPermissionDeclared(e.app.Permissions.Models, binding) {
			return nil, fmt.Errorf("model binding %q is not declared in app permissions", binding.ID)
		}
		for _, tool := range step.Tools {
			if !containsString(e.app.Permissions.Tools, tool) {
				return nil, fmt.Errorf("agent tool %q is not declared in app permissions", tool)
			}
		}
		prompt, err := ResolveValue(step.Prompt, e.refs)
		if err != nil {
			return nil, err
		}
		return e.runner.executors.Agent.ExecuteAgent(ctx, AgentRequest{AppID: e.app.ID, RunID: e.run.ID, StepID: step.ID, Binding: binding, Prompt: fmt.Sprint(prompt), Tools: append([]string(nil), step.Tools...), MaxTurns: step.MaxTurns, OutputSchema: step.OutputSchema, Workspace: e.workspace})
	case "confirm":
		if e.decisions == nil || e.decisions.Confirmations == nil {
			return nil, errWaitingForConfirmation
		}
		if !e.decisions.Confirmations[step.ID] {
			return nil, fmt.Errorf("confirmation %q was rejected", step.ID)
		}
		return true, nil
	case "branch":
		if step.If == nil {
			return nil, errors.New("branch step requires if condition")
		}
		ok, err := ExecuteCondition(ctx, *step.If, e.refs)
		if err != nil {
			return nil, err
		}
		chosen := step.Else
		if ok {
			chosen = step.Then
		}
		if err := e.executeSteps(chosen); err != nil {
			return nil, err
		}
		return ok, nil
	case "miniapp":
		if !containsString(e.app.Permissions.Apps, step.AppID) {
			return nil, fmt.Errorf("nested app %q is not declared in app permissions", step.AppID)
		}
		depth, _ := ctx.Value(miniAppNestingKey{}).(int)
		if depth >= maxMiniAppNesting {
			return nil, fmt.Errorf("mini app nesting limit of %d is exceeded", maxMiniAppNesting)
		}
		if e.runner.store == nil {
			return nil, errors.New("mini app store is unavailable")
		}
		mapped := make(map[string]any, len(step.InputMap))
		for key, value := range step.InputMap {
			resolved, err := ResolveValue(value, e.refs)
			if err != nil {
				return nil, err
			}
			mapped[key] = resolved
		}
		nestedCtx := context.WithValue(ctx, miniAppNestingKey{}, depth+1)
		nestedDecisions := nestedOperatorDecisions(e.decisions, step.ID+".")
		var nested Run
		var err error
		if nestedRunID := e.nestedRuns[step.ID]; nestedRunID != "" {
			nested, err = e.runner.Resume(nestedCtx, nestedRunID, nestedDecisions)
		} else {
			nested, err = e.runner.RunRelease(nestedCtx, step.AppID, step.AppVersion, mapped, nestedDecisions)
		}
		if nested.Status == RunWaitingForConfirmation {
			e.nestedRuns[step.ID] = nested.ID
			if waitingID := waitingConfirmationStep(nested.Steps); waitingID != "" {
				return nil, &nestedConfirmationPause{stepID: step.ID + "." + waitingID}
			}
		}
		if err != nil {
			return nil, err
		}
		delete(e.nestedRuns, step.ID)
		return nested.Outputs, nil
	default:
		return nil, fmt.Errorf("unsupported step kind %q", step.Kind)
	}
}

func nestedOperatorDecisions(decisions *OperatorDecisions, prefix string) *OperatorDecisions {
	if decisions == nil || decisions.Confirmations == nil {
		return nil
	}
	nested := &OperatorDecisions{Confirmations: make(map[string]bool)}
	for id, approved := range decisions.Confirmations {
		if strings.HasPrefix(id, prefix) {
			nested.Confirmations[strings.TrimPrefix(id, prefix)] = approved
		}
	}
	return nested
}

func (e *runExecution) modelBinding(id string) (ModelBinding, error) {
	for _, binding := range e.app.Requirements.ModelBindings {
		if binding.ID == id {
			return binding, nil
		}
	}
	return ModelBinding{}, fmt.Errorf("model binding %q not found", id)
}

func (e *runExecution) evaluateSuccess() error {
	spec := e.app.Success
	if spec.Mode == "" {
		spec.Mode = "all"
	}
	if spec.Mode != "all" && spec.Mode != "any" {
		return fmt.Errorf("unsupported success mode %q", spec.Mode)
	}
	passed := 0
	for _, check := range spec.Checks {
		ok, err := e.evaluateCheck(check)
		if err != nil {
			return fmt.Errorf("success check %s: %w", check.Kind, err)
		}
		if ok {
			passed++
		} else if spec.Mode == "all" {
			return fmt.Errorf("success check %q failed", check.Kind)
		}
	}
	if spec.Mode == "any" && passed == 0 {
		return errors.New("all success checks failed")
	}
	return nil
}

func (e *runExecution) evaluateCheck(check SuccessCheck) (bool, error) {
	switch check.Kind {
	case "step":
		result, ok := e.run.Steps[check.Step]
		return ok && string(result.Status) == check.Status, nil
	case "schema":
		value, err := ResolveValue(check.Value, e.refs)
		if err != nil {
			return false, err
		}
		return validateJSONType(value, check.Schema) == nil, nil
	case "prompt":
		if e.runner.executors.Model == nil {
			return false, errors.New("model executor is unavailable")
		}
		binding, err := e.modelBinding(check.ModelBinding)
		if err != nil {
			return false, err
		}
		if !modelPermissionDeclared(e.app.Permissions.Models, binding) {
			return false, fmt.Errorf("model binding %q is not declared in app permissions", binding.ID)
		}
		actual, err := ResolveValue(check.Value, e.refs)
		if err != nil {
			return false, err
		}
		actualJSON, err := json.Marshal(actual)
		if err != nil {
			return false, err
		}
		prompt := "Verify the Mini App success criterion. Return JSON only: {\"passed\":true|false}.\nEXPECTED_RESULT: " + e.app.Success.ExpectedResult + "\nACCEPTANCE_CRITERION: " + check.Prompt + "\nACTUAL_RESULT: " + string(actualJSON)
		result, err := e.runner.executors.Model.ExecuteModel(e.ctx, ModelRequest{AppID: e.app.ID, RunID: e.run.ID, StepID: check.Step, Binding: binding, Prompt: prompt})
		if err != nil {
			return false, err
		}
		return parseSuccessVerdict(result)
	default:
		return false, fmt.Errorf("unsupported success check %q", check.Kind)
	}
}

var errWaitingForConfirmation = errors.New("operator confirmation is required")

func parseSuccessVerdict(value any) (bool, error) {
	if verdict, ok := value.(map[string]any); ok {
		passed, ok := verdict["passed"].(bool)
		if !ok {
			return false, errors.New("model verdict must contain boolean passed")
		}
		return passed, nil
	}
	if text, ok := value.(string); ok {
		var verdict struct {
			Passed bool `json:"passed"`
		}
		if err := json.Unmarshal([]byte(text), &verdict); err != nil {
			return false, err
		}
		return verdict.Passed, nil
	}
	return false, fmt.Errorf("model verdict has unsupported type %T", value)
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == strings.TrimSpace(want) {
			return true
		}
	}
	return false
}

func modelPermissionDeclared(permissions []string, binding ModelBinding) bool {
	return containsString(permissions, binding.ID) ||
		containsString(permissions, binding.Selection) ||
		containsString(permissions, binding.Model)
}

func redactValueMap(value map[string]any, secrets []string) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = redactValue(item, secrets, isSensitiveKey(key))
	}
	return out
}

func redactValue(value any, secrets []string, force bool) any {
	if force {
		return "REDACTED"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = redactValue(item, secrets, isSensitiveKey(key))
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = redactValue(item, secrets, false)
		}
		return out
	case string:
		return redactString(typed, secrets)
	default:
		return value
	}
}

func redactString(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "REDACTED")
		}
	}
	return value
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"), " ", "_"))
	if strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "api_key") || strings.Contains(key, "apikey") || strings.Contains(key, "cookie") {
		return true
	}
	return key == "authorization" || key == "credential" || key == "credentials"
}
