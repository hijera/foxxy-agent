//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	toolcmd "github.com/hijera/foxxycode-agent/internal/tools/cmdprofile"
)

type commandEvidenceFeature struct {
	t         *testing.T
	input     DistillInput
	config    *config.Config
	file      bool
	result    AsyncJob
	parser    MiniApp
	sanitized SanitizationReport
}

func (s *commandEvidenceFeature) parserProfile() error {
	s.parser = coreValidApp()
	s.parser.Requirements.Commands = []cmdprofile.ProfileSpec{{Name: "parser", Binary: "python3", Permission: "allow", Template: []string{"-c", `exec("import re\ntry: re.search(r'\\s*Access', text)\nexcept Exception as e:\n    pass\n")`}}}
	return nil
}

func (s *commandEvidenceFeature) sanitizeParser() error {
	s.sanitized = Sanitize(s.parser)
	return nil
}

func (s *commandEvidenceFeature) parserReleasable() error {
	if !s.sanitized.Clean {
		return fmt.Errorf("parser regexp mistaken for a private path: %+v", s.sanitized.Findings)
	}
	return nil
}

func (s *commandEvidenceFeature) source(kind string) error {
	fake, profile := buildDocProfile(s.t)
	s.file = kind == "file"
	args := map[string]any{"input_path": "source.mp4", "output_path": "audio.mp3"}
	if s.file {
		s.t.Setenv("FOXXYCODE_FAKE_CMD_CREATE_LAST_FILE", "1")
		s.t.Setenv("FOXXYCODE_FAKE_CMD_STDOUT", "encoded")
	} else {
		s.t.Setenv("FOXXYCODE_FAKE_CMD_CREATE_LAST_FILE", "0")
		s.t.Setenv("FOXXYCODE_FAKE_CMD_STDOUT", "703")
		profile = cmdprofile.ProfileSpec{Name: "calculator", Binary: fake.Binary, Permission: "allow", Template: []string{"{a}", "{b}"}, Params: []cmdprofile.ParamSpec{{Name: "a", Type: cmdprofile.ParamInt}, {Name: "b", Type: cmdprofile.ParamInt}}}
		args = map[string]any{"a": 37, "b": 19}
	}
	s.config = &config.Config{Commands: []cmdprofile.ProfileSpec{profile}}
	s.config.Paths.Home = s.t.TempDir()
	encoded, _ := json.Marshal(args)
	result, err := toolcmd.Tool(profile).Execute(context.Background(), string(encoded), &tooling.Env{CWD: s.t.TempDir()})
	if err != nil {
		return err
	}
	s.input = DistillInput{SessionID: "command-evidence", Title: "Command evidence", CommandProfiles: []cmdprofile.ProfileSpec{profile}, Messages: []llm.Message{
		{Role: llm.RoleUser, Content: "Run the reusable calculation."},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "source-command", Name: profile.ToolName(), InputJSON: string(encoded)}}},
		{Role: llm.RoleTool, ToolCallID: "source-command", Content: result},
		{Role: llm.RoleAssistant, Content: "The requested operation completed successfully; here is an explanation."},
	}}
	return nil
}

func (s *commandEvidenceFeature) replay() error {
	_, _, candidates, err := DistillTrace(TraceInput{SessionID: s.input.SessionID, Messages: s.input.Messages, CommandProfiles: s.input.CommandProfiles})
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("source command produced no candidate")
	}
	confirmed, err := ConfirmScenario(candidates, TraceScenarioSelection{CandidateID: candidates[0].ID})
	if err != nil {
		return err
	}
	s.input.Scenario = &confirmed
	app, evidence, err := Distill(s.input)
	if err != nil {
		return err
	}
	store := NewStore(s.t.TempDir())
	if err := store.CreateDraft(app, &evidence); err != nil {
		return err
	}
	app, err = store.GetDraft(app.ID)
	if err != nil {
		return err
	}
	inputs := ReplayInputs(app, evidence, nil)
	if s.file {
		for _, field := range app.Inputs {
			if field.Type != "file" {
				continue
			}
			upload, err := store.PutUpload(app, field.ID, "source.mp4", strings.NewReader("original video fixture"))
			if err != nil {
				return err
			}
			inputs[field.ID] = map[string]any{"upload_id": upload.ID}
		}
	}
	service := NewService(store, NewRunner(store, Executors{Tool: NewLiveBuiltinToolExecutor(func() *config.Config { return s.config })}).WithWorkspaceRoot(s.t.TempDir()))
	defer service.Close()
	job, err := service.StartTestRun(app.ID, inputs, nil)
	if err != nil {
		return err
	}
	s.result = waitForJob(s.t, service, job.ID, func(job AsyncJob) bool { return job.Status == JobSucceeded || job.Status == JobFailed })
	return nil
}

func (s *commandEvidenceFeature) verified() error {
	if s.result.Status != JobSucceeded || s.result.Report == nil || !s.result.Report.Passed {
		return fmt.Errorf("command replay incorrectly rejected: status=%s report=%+v", s.result.Status, s.result.Report)
	}
	if s.file && s.result.Report.ArtifactsCompared != 1 {
		return fmt.Errorf("file replay did not compare recorded bytes: %+v", s.result.Report)
	}
	return nil
}

func TestMiniAppsCommandEvidenceFeature(t *testing.T) {
	state := commandEvidenceFeature{t: t}
	suite := godog.TestSuite{Name: "miniapps-command-evidence", Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/miniapps_command_evidence.feature"}, TestingT: t, Strict: true}, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^a successful (file|scalar) command with a conversational final answer$`, state.source)
		sc.Step(`^I create a Mini App from the command session and test the source inputs$`, state.replay)
		sc.Step(`^its source verification passes using the recorded command result$`, state.verified)
		sc.Step(`^a reviewed parser profile containing an escaped regular expression$`, state.parserProfile)
		sc.Step(`^I check the portable parser for private paths$`, state.sanitizeParser)
		sc.Step(`^its ordinary regular expression is releasable$`, state.parserReleasable)
	}}
	if suite.Run() != 0 {
		t.Fatal("command-evidence feature failed")
	}
}
