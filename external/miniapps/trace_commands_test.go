//go:build miniapps

package miniapps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func commandTestProfile() cmdprofile.ProfileSpec {
	return cmdprofile.ProfileSpec{
		Name: "ffmpeg_extract_audio", Binary: "ffmpeg", Permission: "allow",
		Template: []string{"-i", "{input_path}", "-vn", "-acodec", "{codec}", "{output_path}"},
		Params: []cmdprofile.ParamSpec{
			{Name: "input_path", Type: cmdprofile.ParamFile},
			{Name: "codec", Type: cmdprofile.ParamEnum, Enum: []string{"libmp3lame", "aac"}},
			{Name: "output_path", Type: cmdprofile.ParamFile},
		},
		Install: cmdprofile.InstallSpec{Winget: "Gyan.FFmpeg"},
	}
}

func runCommandTraceInput(command string) TraceInput {
	return TraceInput{
		SessionID: "session-cmd",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Вытащи аудио из видео " + command},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "call-1", Name: "run_command",
				InputJSON: `{"command":` + jsonString(command) + `}`,
			}}},
			{Role: llm.RoleTool, ToolCallID: "call-1", Content: "audio extracted"},
			{Role: llm.RoleAssistant, Content: "Готово, аудио извлечено."},
		},
		CommandProfiles: []cmdprofile.ProfileSpec{commandTestProfile()},
	}
}

func jsonString(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

// A run_command action matching a declared profile is rewritten in place into
// the profile tool call with typed JSON arguments, before eligibility and
// candidate generation — so the whole existing pipeline sees cmd_* actions.
func TestDistillTraceRewritesMatchingRunCommand(t *testing.T) {
	trace, eligibility, candidates, err := DistillTrace(runCommandTraceInput("ffmpeg -i clip.mp4 -vn -acodec libmp3lame clip.mp3"))
	if err != nil {
		t.Fatalf("DistillTrace() error = %v", err)
	}
	if !eligibility.Eligible {
		t.Fatalf("eligibility = %+v", eligibility)
	}
	if len(candidates) == 0 {
		t.Fatal("no scenario candidates")
	}
	if len(trace.Actions) != 1 {
		t.Fatalf("actions = %+v", trace.Actions)
	}
	action := trace.Actions[0]
	if action.Name != "cmd_ffmpeg_extract_audio" {
		t.Fatalf("action name = %q", action.Name)
	}
	var arguments map[string]string
	if err := json.Unmarshal([]byte(action.Arguments), &arguments); err != nil {
		t.Fatalf("arguments %q: %v", action.Arguments, err)
	}
	if arguments["input_path"] != "clip.mp4" || arguments["codec"] != "libmp3lame" || arguments["output_path"] != "clip.mp3" {
		t.Fatalf("arguments = %#v", arguments)
	}
}

func TestDistillTraceLeavesUnmatchedRunCommandUntouched(t *testing.T) {
	for name, command := range map[string]string{
		"different binary":  "magick in.png out.jpg",
		"shell operators":   "ffmpeg -i in.mp4 out.mp3 && rm -rf .",
		"template mismatch": "ffmpeg -version",
	} {
		t.Run(name, func(t *testing.T) {
			trace, _, _, err := DistillTrace(runCommandTraceInput(command))
			if err != nil {
				t.Fatalf("DistillTrace() error = %v", err)
			}
			if trace.Actions[0].Name != "run_command" {
				t.Fatalf("action name = %q, want run_command kept", trace.Actions[0].Name)
			}
		})
	}
}

// The full synthesis path: the rewritten action becomes a cmd_* workflow step,
// its file params become operator inputs, and the matched profile is embedded
// into the portable document so the app carries its own declaration.
func TestDistillEmbedsTheMatchedProfileIntoTheDocument(t *testing.T) {
	input := runCommandTraceInput("ffmpeg -i clip.mp4 -vn -acodec libmp3lame clip.mp3")
	app, evidence, err := Distill(DistillInput{
		SessionID:       input.SessionID,
		Title:           "Извлечение аудио",
		Messages:        input.Messages,
		CommandProfiles: input.CommandProfiles,
		Scenario: &TraceConfirmedScenario{
			Task:            "Извлечь аудио из видео",
			AcceptedOutcome: "Готово, аудио извлечено.",
			ActionIndexes:   []int{0},
		},
	})
	if err != nil {
		t.Fatalf("Distill() error = %v", err)
	}
	if len(app.Workflow) != 1 || app.Workflow[0].Tool != "cmd_ffmpeg_extract_audio" {
		t.Fatalf("workflow = %+v", app.Workflow)
	}
	if len(app.Requirements.Commands) != 1 || app.Requirements.Commands[0].Name != "ffmpeg_extract_audio" {
		t.Fatalf("requirements.commands = %+v", app.Requirements.Commands)
	}
	if !containsString(app.Permissions.Tools, "cmd_ffmpeg_extract_audio") {
		t.Fatalf("permissions.tools = %v", app.Permissions.Tools)
	}
	// The typed params must surface as operator inputs rather than being baked
	// into the step as fixed literals (generated IDs carry the param name).
	surfaced := map[string]bool{}
	for _, appInput := range app.Inputs {
		for _, param := range []string{"input_path", "output_path", "codec"} {
			if strings.Contains(appInput.ID, param) {
				surfaced[param] = true
			}
		}
	}
	if len(surfaced) != 3 {
		t.Fatalf("inputs = %+v, want all three params surfaced (got %v)", app.Inputs, surfaced)
	}
	if evidence.SanitizedTrace == nil || evidence.SanitizedTrace.Actions[0].Name != "cmd_ffmpeg_extract_audio" {
		t.Fatal("evidence trace does not carry the rewritten action")
	}
}

// The service copies TraceInput field by field; forgetting the new profiles
// field would silently disable rewriting for the HTTP path. This pins the copy.
func TestServiceThreadsCommandProfilesIntoTheTraceStage(t *testing.T) {
	store := NewStore(t.TempDir())
	service := NewService(store, NewRunner(store, Executors{}))
	defer service.Close()

	input := runCommandTraceInput("ffmpeg -i clip.mp4 -vn -acodec libmp3lame clip.mp3")
	job, err := service.StartDistillation(DistillInput{
		SessionID:       input.SessionID,
		Messages:        input.Messages,
		CommandProfiles: input.CommandProfiles,
	})
	if err != nil {
		t.Fatalf("StartDistillation() error = %v", err)
	}
	waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status == JobWaitingForScenario })
	current, err := service.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Candidates) == 0 {
		t.Fatalf("job = %+v", current)
	}
	service.mu.Lock()
	pending := service.pending[job.ID]
	service.mu.Unlock()
	if len(pending.Trace.Actions) == 0 || pending.Trace.Actions[0].Name != "cmd_ffmpeg_extract_audio" {
		t.Fatalf("pending trace action = %+v", pending.Trace.Actions)
	}
}
