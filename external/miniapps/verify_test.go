//go:build miniapps

package miniapps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyReplayUsesAcceptedMetricsAndReportsMismatch(t *testing.T) {
	app := verificationTestApp()
	evidence := SourceEvidence{Metrics: map[string]any{"accepted_result": "hello"}}
	passed := VerifyReplay(context.Background(), app, evidence, Run{
		ID: "run-pass", AppID: app.ID, Revision: app.Revision, Status: RunSucceeded,
		Outputs: map[string]any{"result": "hello"},
	})
	if !passed.Passed || passed.Status != VerificationPassed || len(passed.Discrepancies) != 0 {
		t.Fatalf("passed report = %+v", passed)
	}
	mismatch := VerifyReplay(context.Background(), app, evidence, Run{
		ID: "run-fail", AppID: app.ID, Revision: app.Revision, Status: RunSucceeded,
		Outputs: map[string]any{"result": "goodbye"},
	})
	if mismatch.Passed || mismatch.Status != VerificationFailed || len(mismatch.Discrepancies) != 1 {
		t.Fatalf("mismatch report = %+v", mismatch)
	}
	if mismatch.Discrepancies[0].Kind != "result_mismatch" {
		t.Fatalf("discrepancy = %+v", mismatch.Discrepancies[0])
	}
}

func TestVerifyReplayPrefersDeterministicArtifactHashOverAssistantProse(t *testing.T) {
	content := []byte("hello Ada\n")
	sum := sha256.Sum256(content)
	evidence := SourceEvidence{
		AcceptedResult: "Greeting file written successfully.",
		SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{Name: "write_file", Status: TraceActionSucceeded, Artifacts: []TraceArtifact{{
			Path: "greeting.txt", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content)),
		}}}}},
	}
	app := verificationTestApp()
	run := Run{ID: "artifact-run", AppID: app.ID, Revision: app.Revision, Status: RunSucceeded,
		Outputs: map[string]any{"result": "created greeting.txt"}, Artifacts: []RunArtifact{{
			Path: "greeting.txt", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content)),
		}}}
	report := VerifyReplay(context.Background(), app, evidence, run)
	if !report.Passed || report.Status != VerificationPassed || report.ArtifactsCompared != 1 {
		t.Fatalf("artifact report = %+v", report)
	}
	// Keep a real file in the fixture shape as a guard against accidentally
	// switching this test back to prose-only comparison.
	path := filepath.Join(t.TempDir(), "greeting.txt")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyReplayRequiresExactArtifactPath(t *testing.T) {
	content := []byte("same bytes")
	sum := sha256.Sum256(content)
	app := verificationTestApp()
	evidence := SourceEvidence{SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{
		Name: "write", Status: TraceActionSucceeded,
		Artifacts: []TraceArtifact{{Path: "right/result.txt", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))}},
	}}}}
	run := Run{ID: "wrong-path", AppID: app.ID, Revision: app.Revision, Status: RunSucceeded,
		Artifacts: []RunArtifact{{Path: "wrong/result.txt", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))}}}
	report := VerifyReplay(context.Background(), app, evidence, run)
	if report.Passed || !hasDiscrepancyKind(report, "missing_artifact") {
		t.Fatalf("wrong-path artifact passed verification: %+v", report)
	}
}

func TestVerifyReplayUsesLatestSourceArtifactForRepeatedPath(t *testing.T) {
	oldContent, newContent := []byte("old"), []byte("new")
	oldSum, newSum := sha256.Sum256(oldContent), sha256.Sum256(newContent)
	app := verificationTestApp()
	evidence := SourceEvidence{SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{
		{Status: TraceActionSucceeded, Artifacts: []TraceArtifact{{Path: "result.txt", SHA256: hex.EncodeToString(oldSum[:]), SizeBytes: int64(len(oldContent))}}},
		{Status: TraceActionSucceeded, Artifacts: []TraceArtifact{{Path: "result.txt", SHA256: hex.EncodeToString(newSum[:]), SizeBytes: int64(len(newContent))}}},
	}}}
	run := Run{ID: "latest-artifact", AppID: app.ID, Status: RunSucceeded, Artifacts: []RunArtifact{{Path: "result.txt", SHA256: hex.EncodeToString(newSum[:]), SizeBytes: int64(len(newContent))}}}
	report := VerifyReplay(context.Background(), app, evidence, run)
	if !report.Passed || report.ArtifactsCompared != 1 {
		t.Fatalf("latest repeated artifact report = %+v", report)
	}
}

func hasDiscrepancyKind(report VerificationReport, kind string) bool {
	for _, discrepancy := range report.Discrepancies {
		if discrepancy.Kind == kind {
			return true
		}
	}
	return false
}

func TestVerifyReplayEnforcesExplicitExpectedResultWithArtifacts(t *testing.T) {
	content := []byte("artifact")
	sum := sha256.Sum256(content)
	app := verificationTestApp()
	app.Success.ExpectedResult = "approved output"
	evidence := SourceEvidence{
		AcceptedResult: "assistant prose is not authoritative",
		SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{Status: TraceActionSucceeded,
			Artifacts: []TraceArtifact{{Path: "result.txt", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))}}}}},
	}
	run := Run{ID: "explicit-contract", AppID: app.ID, Revision: app.Revision, Status: RunSucceeded,
		Outputs:   map[string]any{"result": "different output"},
		Artifacts: []RunArtifact{{Path: "result.txt", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(content))}}}
	report := VerifyReplay(context.Background(), app, evidence, run)
	if report.Passed || len(report.Discrepancies) != 1 || report.Discrepancies[0].Kind != "expected_result_mismatch" {
		t.Fatalf("explicit expected result was not enforced: %+v", report)
	}
	run.Outputs["result"] = "approved output"
	report = VerifyReplay(context.Background(), app, evidence, run)
	if !report.Passed {
		t.Fatalf("matching explicit result and artifact should pass: %+v", report)
	}
}

func TestReplayInputsExtractsSanitizedTraceValuesWithoutSecrets(t *testing.T) {
	app := verificationTestApp()
	app.Inputs = append(app.Inputs, Input{ID: "token", Type: "secret", Title: "Token", Required: false, UI: InputUI{Control: "password"}})
	evidence := SourceEvidence{SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{
		Name: "greet", Status: TraceActionSucceeded,
		Arguments: `{"name":"Ada","token":"[REDACTED]"}`,
	}}}}
	inputs := ReplayInputs(app, evidence, map[string]any{"name": "override"})
	if inputs["name"] != "override" {
		t.Fatalf("explicit input was overwritten: %#v", inputs)
	}
	if _, ok := inputs["token"]; ok {
		t.Fatalf("sanitized secret was reconstructed: %#v", inputs)
	}
}

func TestRepairRequiresAcceptanceAndRevisionMatches(t *testing.T) {
	store := NewStore(t.TempDir())
	app := verificationTestApp()
	if err := store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetDraft(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Success.ExpectedResult = "stale expectation"
	stored, err = store.UpdateDraft(stored.ID, stored.Revision, stored)
	if err != nil {
		t.Fatal(err)
	}
	evidence := SourceEvidence{AcceptedResult: "hello"}
	run := Run{ID: "repair-run", AppID: stored.ID, Revision: stored.Revision, Status: RunSucceeded, Outputs: map[string]any{"result": "hello"}}
	report := VerifyReplay(context.Background(), stored, evidence, run)
	if report.Passed || len(report.Discrepancies) != 1 || report.Discrepancies[0].Kind != "expected_result_mismatch" {
		t.Fatalf("repair source report = %+v", report)
	}
	proposals := GenerateRepairProposals(stored, report)
	if len(proposals) != 1 || len(proposals[0].Operations) != 1 || len(proposals) > MaxRepairProposals {
		t.Fatalf("proposals = %+v", proposals)
	}
	if _, err := ApplyRepair(store, proposals[0]); err == nil {
		t.Fatal("unaccepted repair applied")
	}
	if err := AcceptRepair(&proposals[0]); err != nil {
		t.Fatal(err)
	}
	updated, err := ApplyRepair(store, proposals[0])
	if err != nil {
		t.Fatalf("ApplyRepair() error = %v", err)
	}
	if updated.Revision == stored.Revision || updated.Success.ExpectedResult != "hello" {
		t.Fatalf("updated draft = %+v, want new revision and expected result", updated)
	}
	run.Revision = updated.Revision
	if repaired := VerifyReplay(context.Background(), updated, evidence, run); !repaired.Passed {
		t.Fatalf("accepted repair did not fix verification: %+v", repaired)
	}
	if _, err := ApplyRepair(store, proposals[0]); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale repair error = %v, want revision conflict", err)
	}
}

func TestRepairDoesNotProposeIneffectiveResultMismatchPatch(t *testing.T) {
	app := verificationTestApp()
	app.Revision = "revision"
	report := VerificationReport{AppID: app.ID, Revision: app.Revision, Expected: "source", Actual: "different",
		Discrepancies: []Discrepancy{{Path: "/outputs/result", Kind: "result_mismatch"}}}
	if proposals := GenerateRepairProposals(app, report); len(proposals) != 0 {
		t.Fatalf("ineffective proposals = %+v", proposals)
	}
}

func verificationTestApp() MiniApp {
	return MiniApp{
		SchemaVersion: SchemaVersion, Kind: KindMiniApp, ID: "verify-test", State: StateDraft,
		Metadata:    Metadata{Name: "Verification test", Goal: "Verify"},
		Permissions: Permissions{Tools: []string{"greet"}},
		Inputs:      []Input{{ID: "name", Type: "text", Title: "Name", Required: true, UI: InputUI{Control: "text"}}},
		Workflow:    []Step{{ID: "greet", Kind: "tool", Title: "Greet", Tool: "greet", Arguments: map[string]any{"name": Ref{Ref: "inputs.name"}}}},
		Success:     SuccessSpec{Mode: "all", Checks: []SuccessCheck{{Kind: "step", Step: "greet", Status: string(RunSucceeded)}}},
		Outputs:     []Output{{ID: "result", Type: "string", Value: Ref{Ref: "steps.greet.outputs.result"}}},
		Runtime:     RuntimePolicy{LogScope: "global", OperatorEventLevel: "status", DiagnosticToolEvents: "sanitized"},
	}
}

func TestRepairProposalDoesNotAcceptArbitraryPath(t *testing.T) {
	app := verificationTestApp()
	app.Revision = "revision"
	proposal := RepairProposal{ID: "repair-1", AppID: app.ID, BaseRevision: "revision", Accepted: true,
		Operations: []RepairOperation{{Path: "/permissions/tools", Value: []any{"shell"}}}}
	store := &memoryDraftUpdater{app: app}
	if _, err := ApplyRepair(store, proposal); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("arbitrary repair path error = %v", err)
	}
}

type memoryDraftUpdater struct{ app MiniApp }

func (m *memoryDraftUpdater) GetDraft(_ string) (MiniApp, error) { return m.app, nil }
func (m *memoryDraftUpdater) UpdateDraft(_, expected string, app MiniApp) (MiniApp, error) {
	if expected != m.app.Revision {
		return MiniApp{}, ErrRevisionConflict
	}
	m.app = app
	return app, nil
}
