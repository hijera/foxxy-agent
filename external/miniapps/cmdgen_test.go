//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// cmdgenProvider replays scripted responses and records the prompts.
type cmdgenProvider struct {
	responses []string
	calls     int
	prompts   []string
}

func (p *cmdgenProvider) Complete(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	if len(messages) > 0 {
		p.prompts = append(p.prompts, messages[len(messages)-1].Content)
	}
	index := p.calls
	if index >= len(p.responses) {
		index = len(p.responses) - 1
	}
	p.calls++
	return &llm.Response{Content: p.responses[index]}, nil
}

func (*cmdgenProvider) Stream(context.Context, []llm.Message, []llm.ToolDefinition, func(llm.StreamChunk)) (*llm.Response, error) {
	return nil, nil
}

func cmdgenExecutor(t *testing.T, provider llm.Provider) *ProviderModelExecutor {
	t.Helper()
	executor := NewProviderModelExecutor(miniAppModelTestConfig())
	executor.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return provider, nil })
	return executor
}

func goodGeneratedEnvelope(permission string) string {
	envelope := map[string]any{
		"profile": map[string]any{
			"name": "magick_strip", "binary": "magick", "permission": permission,
			"description": "Strip metadata from an image.",
			"template":    []string{"{input_path}", "-strip", "{output_path}"},
			"params": []map[string]any{
				{"name": "input_path", "type": "file"},
				{"name": "output_path", "type": "file"},
			},
		},
		"arguments": map[string]string{"input_path": "photo.jpg", "output_path": "clean.jpg"},
	}
	raw, _ := json.Marshal(envelope)
	return string(raw)
}

func TestGenerateCommandProfileAcceptsAVerifiedEnvelope(t *testing.T) {
	provider := &cmdgenProvider{responses: []string{goodGeneratedEnvelope("allow")}}
	generated, err := cmdgenExecutor(t, provider).GenerateCommandProfile(
		context.Background(), "magick photo.jpg -strip clean.jpg", ModelBinding{})
	if err != nil {
		t.Fatalf("GenerateCommandProfile() error = %v", err)
	}
	if generated.Profile.Name != "magick_strip" || generated.Profile.Binary != "magick" {
		t.Fatalf("profile = %+v", generated.Profile)
	}
	if generated.Arguments["input_path"] != "photo.jpg" {
		t.Fatalf("arguments = %#v", generated.Arguments)
	}
	// The prompt must carry the tokenized argv as data, not ask the model to parse shell.
	if len(provider.prompts) == 0 || !strings.Contains(provider.prompts[0], `"magick"`) {
		t.Fatalf("prompt = %q", provider.prompts)
	}
}

// The model cannot mint an ask-profile into the pipeline: permission is forced
// to allow before validation, whatever the envelope said.
func TestGenerateCommandProfileForcesAllowPermission(t *testing.T) {
	provider := &cmdgenProvider{responses: []string{goodGeneratedEnvelope("ask")}}
	generated, err := cmdgenExecutor(t, provider).GenerateCommandProfile(
		context.Background(), "magick photo.jpg -strip clean.jpg", ModelBinding{})
	if err != nil {
		t.Fatalf("GenerateCommandProfile() error = %v", err)
	}
	if generated.Profile.ResolvedPermission() != cmdprofile.PermissionAllow {
		t.Fatalf("permission = %q", generated.Profile.Permission)
	}
}

// Acceptance is purely deterministic: a plausible profile that does not
// reconstruct the original argv is rejected and retried with the reason.
func TestGenerateCommandProfileRetriesOnReconstructionMismatch(t *testing.T) {
	diverging := strings.Replace(goodGeneratedEnvelope("allow"), "-strip", "-auto-orient", 1)
	provider := &cmdgenProvider{responses: []string{diverging, goodGeneratedEnvelope("allow")}}
	generated, err := cmdgenExecutor(t, provider).GenerateCommandProfile(
		context.Background(), "magick photo.jpg -strip clean.jpg", ModelBinding{})
	if err != nil {
		t.Fatalf("GenerateCommandProfile() error = %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want a retry", provider.calls)
	}
	if generated.Profile.Template[1] != "-strip" {
		t.Fatalf("template = %v", generated.Profile.Template)
	}
	// The retry prompt names the mismatch so the model can correct it.
	if !strings.Contains(provider.prompts[1], "reconstruct") {
		t.Fatalf("retry prompt = %q", provider.prompts[1])
	}
}

func TestGenerateCommandProfileGivesUpAfterRetries(t *testing.T) {
	diverging := strings.Replace(goodGeneratedEnvelope("allow"), "-strip", "-auto-orient", 1)
	provider := &cmdgenProvider{responses: []string{diverging}}
	_, err := cmdgenExecutor(t, provider).GenerateCommandProfile(
		context.Background(), "magick photo.jpg -strip clean.jpg", ModelBinding{})
	if err == nil {
		t.Fatal("a never-reconstructing generation was accepted")
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want initial + 2 retries", provider.calls)
	}
}

func TestGenerateCommandProfileAcceptsFencedOutput(t *testing.T) {
	provider := &cmdgenProvider{responses: []string{"```json\n" + goodGeneratedEnvelope("allow") + "\n```"}}
	if _, err := cmdgenExecutor(t, provider).GenerateCommandProfile(
		context.Background(), "magick photo.jpg -strip clean.jpg", ModelBinding{}); err != nil {
		t.Fatalf("fenced envelope rejected: %v", err)
	}
}

func TestGenerateCommandProfileRejectsShellSyntaxWithoutCallingTheModel(t *testing.T) {
	provider := &cmdgenProvider{responses: []string{goodGeneratedEnvelope("allow")}}
	_, err := cmdgenExecutor(t, provider).GenerateCommandProfile(
		context.Background(), "magick photo.jpg -strip clean.jpg && rm -rf .", ModelBinding{})
	if !errors.Is(err, cmdprofile.ErrShellComplex) {
		t.Fatalf("shell command error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatal("a shell command reached the model")
	}
}

// Service integration: an unmatched simple command in the confirmed scenario
// is turned into a generated, embedded (untrusted-by-construction) profile.
func TestServiceGeneratesAProfileForAnUnmatchedCommand(t *testing.T) {
	provider := &cmdgenProvider{responses: []string{goodGeneratedEnvelope("allow")}}
	model := cmdgenExecutor(t, provider)
	store := NewStore(t.TempDir())
	service := NewService(store, NewRunner(store, Executors{Model: model}))
	defer service.Close()

	input := runCommandTraceInput("magick photo.jpg -strip clean.jpg")
	input.CommandProfiles = nil // nothing declared: the matcher cannot help
	job, err := service.StartDistillation(DistillInput{
		SessionID: input.SessionID, Messages: input.Messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status == JobWaitingForScenario })
	if len(waiting.Candidates) == 0 {
		t.Fatalf("job = %+v", waiting)
	}
	if _, err := service.ConfirmScenario(job.ID, TraceScenarioSelection{CandidateID: waiting.Candidates[0].ID}); err != nil {
		t.Fatalf("ConfirmScenario() = %v", err)
	}
	done := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if done.Status != JobSucceeded {
		t.Fatalf("job = %+v", done)
	}
	app, err := store.GetDraft(done.AppID)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.Requirements.Commands) != 1 || app.Requirements.Commands[0].Name != "magick_strip" {
		t.Fatalf("requirements.commands = %+v", app.Requirements.Commands)
	}
	if app.Workflow[0].Tool != "cmd_magick_strip" {
		t.Fatalf("workflow = %+v", app.Workflow)
	}
}

// A confirmed scenario whose command carries shell syntax fails with an
// actionable message instead of producing a doomed run_command step.
func TestServiceFailsDistillationForAShellCommand(t *testing.T) {
	provider := &cmdgenProvider{responses: []string{goodGeneratedEnvelope("allow")}}
	model := cmdgenExecutor(t, provider)
	store := NewStore(t.TempDir())
	service := NewService(store, NewRunner(store, Executors{Model: model}))
	defer service.Close()

	input := runCommandTraceInput("magick photo.jpg -strip clean.jpg && rm -rf .")
	job, err := service.StartDistillation(DistillInput{
		SessionID: input.SessionID, Messages: input.Messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	waiting := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status == JobWaitingForScenario })
	if _, err := service.ConfirmScenario(job.ID, TraceScenarioSelection{CandidateID: waiting.Candidates[0].ID}); err != nil {
		t.Fatalf("ConfirmScenario() = %v", err)
	}
	done := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if done.Status != JobFailed {
		t.Fatalf("job = %+v", done)
	}
	if !strings.Contains(done.Error, "shell operators") {
		t.Fatalf("error = %q, want the shell-operators explanation", done.Error)
	}
	if provider.calls != 0 {
		t.Fatal("a shell command reached the model")
	}
}
