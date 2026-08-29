//go:build miniapps

package miniapps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/config"
)

// A test run of a draft carrying an untrusted embedded profile must park as
// waiting_for_confirmation with typed details the UI can render, and resume to
// success once the operator's trust is recorded.
func TestServiceTestRunPausesForTrustAndResumesAfterRecording(t *testing.T) {
	fake, profile := buildDocProfile(t)
	t.Setenv("PATH", filepath.Dir(fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	portable := profile.Portable()

	home := t.TempDir()
	cfg := &config.Config{}
	cfg.Paths.Home = home
	executor := NewLiveBuiltinToolExecutor(func() *config.Config { return cfg })

	app := commandProfileApp()
	app.Permissions = Permissions{Tools: []string{portable.ToolName()}}
	app.Workflow = []Step{{
		ID: "convert", Kind: "tool", Title: "Convert", Tool: portable.ToolName(),
		Arguments: map[string]any{"input_path": "in.mp4", "output_path": "out.mp3"},
	}}
	app.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{Kind: "step", Step: "convert", Status: string(RunSucceeded)}}}
	app.Requirements.Commands = []cmdprofile.ProfileSpec{portable}
	app.Inputs = nil
	// Distilled apps get a result output automatically; a hand-built fixture
	// needs one too, or same-data verification has nothing to compare.
	app.Outputs = []Output{{
		ID: "result", Type: "json", Renderer: "json", Title: "Result",
		Value: Ref{Ref: "steps.convert.outputs.result"},
	}}

	store := NewStore(t.TempDir())
	evidence := SourceEvidence{
		AcceptedResult: "The clip is converted.",
		SanitizedTrace: &NormalizedTrace{Actions: []TraceAction{{
			Name: portable.ToolName(), Status: TraceActionSucceeded,
			Arguments: `{"input_path":"in.mp4","output_path":"out.mp3"}`,
		}}},
	}
	if err := store.CreateDraft(app, &evidence); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(store, Executors{Tool: executor}).WithWorkspaceRoot(t.TempDir())
	service := NewService(store, runner)
	defer service.Close()
	t.Setenv(cmdtestEnvStdout, "The clip is converted.")

	job, err := service.StartTestRun(app.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status == JobWaitingForConfirm })
	if waiting.Confirmation == nil {
		t.Fatalf("waiting job carries no confirmation: %+v", waiting)
	}
	details, _ := waiting.Confirmation.Details.(map[string]any)
	if details["kind"] != "command_profile" || details["name"] != portable.Name {
		t.Fatalf("confirmation details = %#v", waiting.Confirmation.Details)
	}
	hash, _ := details["hash"].(string)
	binary, _ := details["binary"].(string)
	if hash == "" || binary == "" {
		t.Fatalf("confirmation details lack hash/binary: %#v", details)
	}

	// The HTTP layer records trust before resuming; mirror that contract here.
	if err := cmdprofile.NewTrustStore(home).Record(hash, binary); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmRun(job.ID, &OperatorDecisions{Confirmations: map[string]bool{waiting.Confirmation.ID: true}}); err != nil {
		t.Fatalf("ConfirmRun() = %v", err)
	}
	done := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if done.Status != JobSucceeded {
		t.Fatalf("resumed job = %+v", done)
	}
	if calls, _ := fake.Calls(); len(calls) != 1 {
		t.Fatalf("fake calls = %#v", calls)
	}
}

// cmdtestEnvStdout mirrors cmdtest.EnvStdout without importing the test-only
// package into every service test file.
const cmdtestEnvStdout = "FOXXYCODE_FAKE_CMD_STDOUT"
