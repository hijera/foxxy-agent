//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const MaxRepairProposals = 3

// VerificationStatus describes the result of replaying a draft against its
// private authoring evidence.
type VerificationStatus string

const (
	VerificationPassed      VerificationStatus = "passed"
	VerificationFailed      VerificationStatus = "failed"
	VerificationCancelled   VerificationStatus = "cancelled"
	VerificationInterrupted VerificationStatus = "interrupted"
)

// Discrepancy is an operator-safe explanation of a replay mismatch. Values
// are copied from sanitized evidence and run outputs; provider reasoning is
// never included here.
type Discrepancy struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
}

// VerificationReport is persisted by the service as the review artifact for
// a same-data replay. A report is revision-bound and therefore cannot be used
// to release a later draft revision.
type VerificationReport struct {
	AppID             string             `json:"app_id"`
	Revision          string             `json:"revision,omitempty"`
	RunID             string             `json:"run_id,omitempty"`
	Status            VerificationStatus `json:"status"`
	Passed            bool               `json:"passed"`
	Expected          any                `json:"expected,omitempty"`
	Actual            any                `json:"actual,omitempty"`
	Discrepancies     []Discrepancy      `json:"discrepancies,omitempty"`
	ComparedAt        time.Time          `json:"compared_at"`
	FixturePresent    bool               `json:"fixture_present"`
	ArtifactsCompared int                `json:"artifacts_compared,omitempty"`
}

// RepairOperation is a deliberately small JSON-pointer-like edit. Paths are
// restricted to editable draft fields by applyRepairOperation; arbitrary
// document replacement is never accepted.
type RepairOperation struct {
	Path   string `json:"path"`
	Value  any    `json:"value"`
	Reason string `json:"reason,omitempty"`
}

// RepairProposal is immutable after acceptance. Applying it requires the
// exact BaseRevision and always creates a new draft revision in Store.
type RepairProposal struct {
	ID               string            `json:"id"`
	AppID            string            `json:"app_id"`
	BaseRevision     string            `json:"base_revision"`
	Summary          string            `json:"summary"`
	Operations       []RepairOperation `json:"operations"`
	DiscrepancyPaths []string          `json:"discrepancy_paths,omitempty"`
	Accepted         bool              `json:"accepted"`
	AcceptedAt       time.Time         `json:"accepted_at,omitempty"`
}

type DraftUpdater interface {
	GetDraft(string) (MiniApp, error)
	UpdateDraft(string, string, MiniApp) (MiniApp, error)
}

// VerifyReplay compares a completed run with the accepted result from private
// source evidence. It is intentionally independent of Store and is useful for
// deterministic unit tests as well as the asynchronous service.
func VerifyReplay(_ context.Context, app MiniApp, evidence SourceEvidence, run Run) VerificationReport {
	report := VerificationReport{
		AppID: app.ID, Revision: app.Revision, RunID: run.ID,
		ComparedAt: time.Now().UTC(), FixturePresent: evidence.SourceFixture != nil || evidence.SanitizedTrace != nil || len(evidence.FixtureFiles) > 0,
	}
	if run.Status != RunSucceeded {
		report.Status = verificationStatusForRun(run.Status)
		report.Discrepancies = []Discrepancy{{
			Path: "/run/status", Kind: "run_status", Severity: "error",
			Message:  fmt.Sprintf("replay finished with status %q", run.Status),
			Expected: string(RunSucceeded), Actual: string(run.Status),
		}}
		return report
	}
	artifactOK, artifactDiscrepancies, artifactCount := compareReplayArtifacts(run, evidence)
	report.ArtifactsCompared = artifactCount
	actual := runResult(run)
	explicitExpected, explicitExpectedPresent := explicitReplayExpectedResult(app)
	explicitExpectedOK := !explicitExpectedPresent || replayValuesEqual(explicitExpected, actual)
	var explicitDiscrepancy *Discrepancy
	if !explicitExpectedOK {
		discrepancy := Discrepancy{
			Path: "/success/expected_result", Kind: "expected_result_mismatch", Severity: "error",
			Message:  "replay result differs from the explicit Mini App expectation",
			Expected: sanitizeVerificationValue(explicitExpected), Actual: sanitizeVerificationValue(actual),
		}
		explicitDiscrepancy = &discrepancy
	}

	expected, ok := acceptedReplayResult(evidence)
	if !ok {
		if artifactCount > 0 && artifactOK && explicitExpectedOK {
			report.Status, report.Passed = VerificationPassed, true
			return report
		}
		report.Status = VerificationFailed
		if artifactCount == 0 || !artifactOK {
			report.Discrepancies = append(report.Discrepancies, Discrepancy{
				Path: "/accepted_result", Kind: "missing_expected_result", Severity: "error",
				Message: "source evidence does not contain an accepted result",
			})
		}
		report.Discrepancies = append(report.Discrepancies, artifactDiscrepancies...)
		if explicitDiscrepancy != nil {
			report.Discrepancies = append(report.Discrepancies, *explicitDiscrepancy)
		}
		return report
	}
	report.Expected = sanitizeVerificationValue(expected)
	report.Actual = sanitizeVerificationValue(actual)
	valuesMatch := replayValuesEqual(expected, actual)
	sourceEvidenceOK := (valuesMatch && (artifactCount == 0 || artifactOK)) || (!valuesMatch && artifactCount > 0 && artifactOK)
	if sourceEvidenceOK && explicitExpectedOK {
		report.Status = VerificationPassed
		report.Passed = true
		return report
	}
	report.Status = VerificationFailed
	if !valuesMatch && (artifactCount == 0 || !artifactOK) {
		report.Discrepancies = append(report.Discrepancies, Discrepancy{
			Path: "/outputs/result", Kind: "result_mismatch", Severity: "error",
			Message:  "replay result differs from the accepted source result",
			Expected: report.Expected, Actual: report.Actual,
		})
	}
	report.Discrepancies = append(report.Discrepancies, artifactDiscrepancies...)
	if explicitDiscrepancy != nil {
		report.Discrepancies = append(report.Discrepancies, *explicitDiscrepancy)
	}
	return report
}

func explicitReplayExpectedResult(app MiniApp) (any, bool) {
	if strings.TrimSpace(app.Success.ExpectedResult) == "" {
		return nil, false
	}
	return app.Success.ExpectedResult, true
}

// compareReplayArtifacts prefers the deterministic hashes recorded by the
// source trace. This avoids treating assistant prose as the file-task result.
func compareReplayArtifacts(run Run, evidence SourceEvidence) (bool, []Discrepancy, int) {
	expectedByPath := make(map[string]TraceArtifact)
	for _, action := range sourceActions(evidence) {
		if action.Status == TraceActionSucceeded {
			for _, artifact := range action.Artifacts {
				expectedByPath[cleanArtifactPath(artifact.Path)] = artifact
			}
		}
	}
	if len(expectedByPath) == 0 {
		return true, nil, 0
	}
	actual := make(map[string]RunArtifact, len(run.Artifacts))
	for _, artifact := range run.Artifacts {
		actual[cleanArtifactPath(artifact.Path)] = artifact
	}
	discrepancies := make([]Discrepancy, 0)
	matched := 0
	paths := make([]string, 0, len(expectedByPath))
	for path := range expectedByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		artifact := expectedByPath[path]
		actualArtifact, found := actual[path]
		if !found {
			discrepancies = append(discrepancies, Discrepancy{Path: "/artifacts/" + path, Kind: "missing_artifact", Severity: "error", Message: "expected artifact was not produced", Expected: artifact.Path})
			continue
		}
		if artifact.SHA256 != "" && !strings.EqualFold(artifact.SHA256, actualArtifact.SHA256) {
			discrepancies = append(discrepancies, Discrepancy{Path: "/artifacts/" + path, Kind: "artifact_hash_mismatch", Severity: "error", Message: "artifact content differs from source evidence", Expected: artifact.SHA256, Actual: actualArtifact.SHA256})
			continue
		}
		if artifact.SizeBytes > 0 && artifact.SizeBytes != actualArtifact.SizeBytes {
			discrepancies = append(discrepancies, Discrepancy{Path: "/artifacts/" + path, Kind: "artifact_size_mismatch", Severity: "error", Message: "artifact size differs from source evidence", Expected: artifact.SizeBytes, Actual: actualArtifact.SizeBytes})
			continue
		}
		matched++
	}
	return len(discrepancies) == 0 && matched == len(expectedByPath), discrepancies, len(expectedByPath)
}

func cleanArtifactPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimPrefix(path, "./")
	return filepath.ToSlash(filepath.Clean(path))
}

func verificationStatusForRun(status RunStatus) VerificationStatus {
	switch status {
	case RunCancelled:
		return VerificationCancelled
	case RunInterrupted:
		return VerificationInterrupted
	default:
		return VerificationFailed
	}
}

func runResult(run Run) any {
	if value, ok := run.Outputs["result"]; ok {
		return value
	}
	if len(run.Outputs) == 1 {
		for _, value := range run.Outputs {
			return value
		}
	}
	return run.Outputs
}

func acceptedReplayResult(evidence SourceEvidence) (any, bool) {
	if evidence.AcceptedResult != nil {
		return evidence.AcceptedResult, true
	}
	for _, key := range []string{"accepted_result", "expected_result", "source_result"} {
		if value, ok := evidence.Metrics[key]; ok && value != nil {
			return value, true
		}
		if value, ok := evidence.SourceFixture[key]; ok && value != nil {
			return value, true
		}
	}
	if evidence.SanitizedTrace != nil && strings.TrimSpace(evidence.SanitizedTrace.LastAssistantResult) != "" {
		return evidence.SanitizedTrace.LastAssistantResult, true
	}
	if value, ok := evidence.SourceFixture["last_assistant_result"]; ok && value != nil {
		return value, true
	}
	return nil, false
}

func replayValuesEqual(expected, actual any) bool {
	if reflect.DeepEqual(expected, actual) {
		return true
	}
	// Tool and assistant adapters often represent structured output as JSON
	// text. Compare decoded values before falling back to exact strings.
	decode := func(value any) (any, bool) {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return value, false
		}
		var decoded any
		if json.Unmarshal([]byte(text), &decoded) != nil {
			return value, false
		}
		return decoded, true
	}
	left, leftDecoded := decode(expected)
	right, rightDecoded := decode(actual)
	return (leftDecoded || rightDecoded) && reflect.DeepEqual(left, right)
}

func sanitizeVerificationValue(value any) any {
	return redactValue(value, nil, false)
}

// ReplayInputs reconstructs non-secret source inputs from the sanitized trace
// when possible. Secret values are intentionally not reconstructed: callers
// must provide them through runtime input handles. Explicit inputs take
// precedence over inferred values.
func ReplayInputs(app MiniApp, evidence SourceEvidence, explicit map[string]any) map[string]any {
	result := make(map[string]any, len(explicit)+len(app.Inputs))
	for key, value := range explicit {
		result[key] = value
	}
	actions := sourceActions(evidence)
	if len(actions) == 0 {
		return result
	}
	actionIndex := 0
	for _, step := range app.Workflow {
		if step.Kind != "tool" {
			continue
		}
		for actionIndex < len(actions) && (actions[actionIndex].Status != TraceActionSucceeded || actions[actionIndex].Name != step.Tool) {
			actionIndex++
		}
		if actionIndex >= len(actions) {
			break
		}
		observed := decodeObservedArguments(actions[actionIndex].Arguments)
		collectInputRefs(step.Arguments, observed, result)
		actionIndex++
	}
	for _, input := range app.Inputs {
		if input.Type == "secret" {
			// A sanitized source fixture contains only a redaction marker and is
			// not a valid secret runtime value.
			if value, ok := result[input.ID]; ok && isTraceRedactionMarker(fmt.Sprint(value)) {
				delete(result, input.ID)
			}
		}
	}
	return result
}

func sourceActions(evidence SourceEvidence) []TraceAction {
	if evidence.SanitizedTrace != nil {
		return evidence.SanitizedTrace.Actions
	}
	value, ok := evidence.SourceFixture["actions"]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var actions []TraceAction
	if json.Unmarshal(raw, &actions) != nil {
		return nil
	}
	return actions
}

func decodeObservedArguments(raw string) any {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return nil
	}
	return value
}

func collectInputRefs(template, observed any, result map[string]any) {
	switch typed := template.(type) {
	case Ref:
		collectRefValue(typed.Ref, observed, result)
	case *Ref:
		if typed != nil {
			collectRefValue(typed.Ref, observed, result)
		}
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok {
			collectRefValue(ref, observed, result)
			return
		}
		observedMap, _ := observed.(map[string]any)
		for key, value := range typed {
			collectInputRefs(value, observedMap[key], result)
		}
	case []any:
		observedList, _ := observed.([]any)
		for index, value := range typed {
			if index < len(observedList) {
				collectInputRefs(value, observedList[index], result)
			}
		}
	}
}

func collectRefValue(ref string, observed any, result map[string]any) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(ref), "inputs."), ".")
	if len(parts) == 0 || parts[0] == "" || strings.HasPrefix(ref, "steps.") {
		return
	}
	if _, exists := result[parts[0]]; !exists {
		result[parts[0]] = observed
	}
}

func GenerateRepairProposals(app MiniApp, report VerificationReport) []RepairProposal {
	if report.Passed || len(report.Discrepancies) == 0 || app.ID == "" || app.Revision == "" {
		return nil
	}
	paths := make([]string, 0, len(report.Discrepancies))
	var repairedValue any
	for _, discrepancy := range report.Discrepancies {
		if discrepancy.Kind != "expected_result_mismatch" {
			return nil
		}
		if discrepancy.Path != "" {
			paths = append(paths, discrepancy.Path)
		}
		repairedValue = discrepancy.Actual
	}
	if repairedValue == nil {
		return nil
	}
	proposal := RepairProposal{
		ID: newID("repair"), AppID: app.ID, BaseRevision: app.Revision,
		Summary:          "Align the explicit result expectation with the verified replay",
		DiscrepancyPaths: paths,
		Operations: []RepairOperation{{
			Path: "/success/expected_result", Value: repairedValue,
			Reason: "update the enforced expectation to the accepted replay result",
		}},
	}
	proposals := []RepairProposal{proposal}
	if len(proposals) > MaxRepairProposals {
		proposals = proposals[:MaxRepairProposals]
	}
	return proposals
}

// AcceptRepair records explicit operator consent. Applying an unaccepted
// proposal is rejected even when the proposal itself is otherwise valid.
func AcceptRepair(proposal *RepairProposal) error {
	if proposal == nil {
		return errors.New("repair proposal is nil")
	}
	if proposal.ID == "" || proposal.AppID == "" || proposal.BaseRevision == "" || len(proposal.Operations) == 0 {
		return errors.New("repair proposal is incomplete")
	}
	proposal.Accepted = true
	proposal.AcceptedAt = time.Now().UTC()
	return nil
}

func ApplyRepair(store DraftUpdater, proposal RepairProposal) (MiniApp, error) {
	if store == nil {
		return MiniApp{}, errors.New("draft store is unavailable")
	}
	if !proposal.Accepted {
		return MiniApp{}, errors.New("repair proposal requires explicit acceptance")
	}
	if proposal.AppID == "" || proposal.BaseRevision == "" || len(proposal.Operations) == 0 {
		return MiniApp{}, errors.New("repair proposal is incomplete")
	}
	app, err := store.GetDraft(proposal.AppID)
	if err != nil {
		return MiniApp{}, err
	}
	if app.Revision != proposal.BaseRevision {
		return MiniApp{}, fmt.Errorf("%w: repair proposal targets %q, current %q", ErrRevisionConflict, proposal.BaseRevision, app.Revision)
	}
	for _, operation := range proposal.Operations {
		if err := applyRepairOperation(&app, operation); err != nil {
			return MiniApp{}, err
		}
	}
	app.State = StateDraft
	app.Version = ""
	app.Revision = proposal.BaseRevision
	if report := Validate(app); !report.Valid {
		return MiniApp{}, fmt.Errorf("%w: repaired draft is invalid: %s", ErrInvalid, report.Issues[0].Message)
	}
	return store.UpdateDraft(proposal.AppID, proposal.BaseRevision, app)
}

func applyRepairOperation(app *MiniApp, operation RepairOperation) error {
	if app == nil {
		return errors.New("mini app is nil")
	}
	path := strings.TrimSpace(operation.Path)
	if path == "" {
		return errors.New("repair operation path is required")
	}
	// Keep repair review intentionally narrow. In particular, identity,
	// permissions, workflow kind, and release state cannot be changed by an
	// automatically generated proposal.
	if path != "/success/expected_result" {
		return fmt.Errorf("repair path %q is not allowed", path)
	}
	switch path {
	case "/success/expected_result":
		if text, ok := operation.Value.(string); ok {
			app.Success.ExpectedResult = text
		} else {
			raw, err := json.Marshal(operation.Value)
			if err != nil {
				return fmt.Errorf("expected result repair value: %w", err)
			}
			app.Success.ExpectedResult = string(raw)
		}
	}
	return nil
}
