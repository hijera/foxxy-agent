//go:build miniapps

package miniapps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
	"github.com/hijera/foxxycode-agent/internal/config"
)

// buildDocProfile compiles a recording binary and returns the portable
// (bare-name) profile a document would embed, plus the exact local profile.
func buildDocProfile(t *testing.T) (cmdtest.Fake, cmdprofile.ProfileSpec) {
	t.Helper()
	fake, err := cmdtest.Build(t.TempDir(), "fakeenc")
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	profile := cmdprofile.ProfileSpec{
		Name: "fakeenc_convert", Binary: fake.Binary, Permission: "allow",
		Template: []string{"-i", "{input_path}", "{output_path}"},
		Params: []cmdprofile.ParamSpec{
			{Name: "input_path", Type: cmdprofile.ParamFile},
			{Name: "output_path", Type: cmdprofile.ParamFile},
		},
	}
	return fake, profile
}

func liveExecutorWithHome(t *testing.T, cfg *config.Config) (*BuiltinToolExecutor, string) {
	t.Helper()
	home := t.TempDir()
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.Paths.Home = home
	return NewLiveBuiltinToolExecutor(func() *config.Config { return cfg }), home
}

func commandToolRequest(profile cmdprofile.ProfileSpec, workspace string) ToolRequest {
	return ToolRequest{
		AppID: "app", RunID: "run", StepID: "step",
		Tool: profile.ToolName(), Workspace: workspace,
		Arguments:      map[string]any{"input_path": "in.mp4", "output_path": "out.mp3"},
		CommandProfile: &profile,
	}
}

// An embedded profile the machine has never seen must pause for the operator:
// the executor signals the existing waiting-for-confirmation flow.
func TestExecuteToolPausesForAnUntrustedDocumentProfile(t *testing.T) {
	fake, profile := buildDocProfile(t)
	executor, _ := liveExecutorWithHome(t, nil)

	_, err := executor.ExecuteTool(context.Background(), commandToolRequest(profile, t.TempDir()))
	if !errors.Is(err, errWaitingForConfirmation) {
		t.Fatalf("untrusted profile error = %v, want errWaitingForConfirmation", err)
	}
	if calls, _ := fake.Calls(); len(calls) != 0 {
		t.Fatalf("the binary ran before trust: %#v", calls)
	}
}

// A recorded trust approval (hash + resolved path) lets the run proceed.
func TestExecuteToolRunsATrustedDocumentProfile(t *testing.T) {
	fake, profile := buildDocProfile(t)
	executor, home := liveExecutorWithHome(t, nil)

	resolved, err := cmdprofile.ResolveBinary(profile, "")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := cmdprofile.CanonicalHash(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmdprofile.NewTrustStore(home).Record(hash, resolved); err != nil {
		t.Fatal(err)
	}

	out, err := executor.ExecuteTool(context.Background(), commandToolRequest(profile, t.TempDir()))
	if err != nil {
		t.Fatalf("trusted profile run failed: %v", err)
	}
	_ = out
	calls, err := fake.Calls()
	if err != nil || len(calls) != 1 {
		t.Fatalf("calls = %#v, err %v", calls, err)
	}
	if strings.Join(calls[0].Args, " ") != "-i in.mp4 out.mp3" {
		t.Fatalf("argv = %#v", calls[0].Args)
	}
}

// A profile whose content hash matches a config-declared profile is the
// operator's own declaration and needs no separate trust record.
func TestExecuteToolTrustsAConfigDeclaredProfileByHash(t *testing.T) {
	fake, profile := buildDocProfile(t)
	cfg := &config.Config{Commands: []cmdprofile.ProfileSpec{profile}}
	executor, _ := liveExecutorWithHome(t, cfg)

	_, err := executor.ExecuteTool(context.Background(), commandToolRequest(profile, t.TempDir()))
	if err != nil {
		t.Fatalf("config-declared profile run failed: %v", err)
	}
	if calls, _ := fake.Calls(); len(calls) != 1 {
		t.Fatalf("calls = %#v", calls)
	}
}

// Trust is void once the profile content changes: the edited document must
// pause again even though an approval for the old hash exists.
func TestExecuteToolRetrustsAnEditedProfile(t *testing.T) {
	_, profile := buildDocProfile(t)
	executor, home := liveExecutorWithHome(t, nil)
	resolved, _ := cmdprofile.ResolveBinary(profile, "")
	hash, _ := cmdprofile.CanonicalHash(profile)
	if err := cmdprofile.NewTrustStore(home).Record(hash, resolved); err != nil {
		t.Fatal(err)
	}
	edited := profile.Clone()
	edited.Template = append(edited.Template, "-fast")
	request := commandToolRequest(edited, t.TempDir())
	if _, err := executor.ExecuteTool(context.Background(), request); !errors.Is(err, errWaitingForConfirmation) {
		t.Fatalf("edited profile error = %v, want a fresh trust pause", err)
	}
}

// Only allow-profiles may run unattended; an embedded ask-profile is a hard
// error, not a pause (approving it once would not make later runs attended).
func TestExecuteToolRejectsAnAskDocumentProfile(t *testing.T) {
	_, profile := buildDocProfile(t)
	profile.Permission = "ask"
	executor, _ := liveExecutorWithHome(t, nil)
	_, err := executor.ExecuteTool(context.Background(), commandToolRequest(profile, t.TempDir()))
	if err == nil || errors.Is(err, errWaitingForConfirmation) || !strings.Contains(err.Error(), "allow") {
		t.Fatalf("ask profile error = %v, want a hard allow-only rejection", err)
	}
}

// A missing binary is an actionable install error, not a trust pause.
func TestExecuteToolReportsAMissingDocumentBinary(t *testing.T) {
	_, profile := buildDocProfile(t)
	profile.Binary = "definitely-not-installed-xyz"
	executor, _ := liveExecutorWithHome(t, nil)
	_, err := executor.ExecuteTool(context.Background(), commandToolRequest(profile, t.TempDir()))
	if err == nil || errors.Is(err, errWaitingForConfirmation) || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("missing binary error = %v", err)
	}
}

// ValidateMiniAppCapabilities must accept an app whose cmd_* tools come from
// the document rather than the registry.
func TestValidateMiniAppCapabilitiesAcceptsDocumentProfiles(t *testing.T) {
	_, profile := buildDocProfile(t)
	executor, _ := liveExecutorWithHome(t, nil)
	app := commandProfileApp()
	app.Permissions = Permissions{Tools: []string{profile.ToolName()}}
	app.Workflow[0].Tool = profile.ToolName()
	app.Requirements.Commands = []cmdprofile.ProfileSpec{profile.Portable()}
	if err := executor.ValidateMiniAppCapabilities(app); err != nil {
		t.Fatalf("ValidateMiniAppCapabilities() = %v", err)
	}
}

// The retry loop must not re-raise a trust pause: one pause parks the run.
func TestRunnerParksAnUntrustedProfileDespiteRetries(t *testing.T) {
	fake, profile := buildDocProfile(t)
	// The runner attaches the document (portable, bare-name) profile, so the
	// binary must resolve the way it would on a real machine: via PATH.
	t.Setenv("PATH", filepath.Dir(fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	executor, _ := liveExecutorWithHome(t, nil)
	store := NewStore(t.TempDir())
	runner := NewRunner(store, Executors{Tool: executor}).WithWorkspaceRoot(t.TempDir())

	app := commandProfileApp()
	app.Permissions = Permissions{Tools: []string{profile.ToolName()}}
	app.Workflow = []Step{{
		ID: "convert", Kind: "tool", Title: "Convert", Tool: profile.ToolName(),
		Arguments: map[string]any{"input_path": "in.mp4", "output_path": "out.mp3"},
		Retry:     RetryPolicy{MaxAttempts: 3, DelayMS: 1},
	}}
	app.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{Kind: "step", Step: "convert", Status: string(RunSucceeded)}}}
	app.Requirements.Commands = []cmdprofile.ProfileSpec{profile.Portable()}
	app.Inputs = nil

	run, err := runner.RunPortable(context.Background(), app, map[string]any{}, nil)
	if !errors.Is(err, errWaitingForConfirmation) {
		t.Fatalf("run error = %v, want waiting-for-confirmation", err)
	}
	step := run.Steps["convert"]
	if step.Status != RunWaitingForConfirmation {
		t.Fatalf("step status = %q", step.Status)
	}
	if step.Attempts != 1 {
		t.Fatalf("attempts = %d, want the pause to stop retries", step.Attempts)
	}
}
