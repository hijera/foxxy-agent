package cmdprofile

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func testProfile(binary string) cmdprofile.ProfileSpec {
	return cmdprofile.ProfileSpec{
		Name: "ffmpeg_extract_audio", Binary: binary, Permission: "allow",
		Template: []string{"-i", "{input_path}", "-vn", "-acodec", "{codec}", "{output_path}"},
		Params: []cmdprofile.ParamSpec{
			{Name: "input_path", Type: cmdprofile.ParamFile},
			{Name: "codec", Type: cmdprofile.ParamEnum, Enum: []string{"libmp3lame", "aac"}},
			{Name: "output_path", Type: cmdprofile.ParamFile},
		},
		Install: cmdprofile.InstallSpec{Winget: "Gyan.FFmpeg", Scoop: "ffmpeg"},
	}
}

func collectTools(cfg *config.Config) map[string]*tooling.Tool {
	tools := map[string]*tooling.Tool{}
	RegisterBuiltins(func(tool *tooling.Tool) { tools[tool.Definition.Name] = tool }, cfg)
	return tools
}

func TestRegisterBuiltinsMapsPermissionVariantB(t *testing.T) {
	ask := testProfile("ffmpeg")
	ask.Permission = "ask"
	allow := testProfile("magick")
	allow.Name = "magick_strip"
	allow.Template = []string{"{input_path}", "-strip", "{output_path}"}
	allow.Params = []cmdprofile.ParamSpec{
		{Name: "input_path", Type: cmdprofile.ParamFile},
		{Name: "output_path", Type: cmdprofile.ParamFile},
	}
	cfg := &config.Config{Commands: []cmdprofile.ProfileSpec{ask, allow}}

	tools := collectTools(cfg)
	if len(tools) != 2 {
		t.Fatalf("tools = %v", tools)
	}
	if !tools["cmd_ffmpeg_extract_audio"].RequiresPermission {
		t.Fatal("an ask profile must require permission in chat")
	}
	if tools["cmd_magick_strip"].RequiresPermission {
		t.Fatal("an allow profile must not require permission")
	}
	schema, _ := tools["cmd_ffmpeg_extract_audio"].Definition.InputSchema.(map[string]interface{})
	if schema == nil {
		t.Fatal("input schema is not an object")
	}
	properties, _ := schema["properties"].(map[string]interface{})
	if properties == nil || properties["codec"] == nil || properties["input_path"] == nil {
		t.Fatalf("schema = %#v", schema)
	}
}

func TestRegisteredToolExecutesTheProfile(t *testing.T) {
	fake, err := cmdtest.Build(t.TempDir(), "ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	t.Setenv(cmdtest.EnvStdout, "done")
	cfg := &config.Config{Commands: []cmdprofile.ProfileSpec{testProfile(fake.Binary)}}

	tool := collectTools(cfg)["cmd_ffmpeg_extract_audio"]
	if tool == nil {
		t.Fatal("tool not registered")
	}
	out, err := tool.Execute(context.Background(),
		`{"input_path":"in.mp4","codec":"aac","output_path":"out.m4a"}`,
		&tooling.Env{CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out != "done" {
		t.Fatalf("Execute() = %q", out)
	}
	calls, err := fake.Calls()
	if err != nil || len(calls) != 1 {
		t.Fatalf("calls = %#v, err %v", calls, err)
	}
	if strings.Join(calls[0].Args, " ") != "-i in.mp4 -vn -acodec aac out.m4a" {
		t.Fatalf("argv = %#v", calls[0].Args)
	}
}

// A profile whose binary is absent still registers, and calling it answers
// with the exact install command — the actionable path for both chat and the
// distillation error surface.
func TestMissingBinaryAnswersWithInstallGuidance(t *testing.T) {
	profile := testProfile("definitely-not-installed-xyz")
	cfg := &config.Config{Commands: []cmdprofile.ProfileSpec{profile}}
	tool := collectTools(cfg)["cmd_ffmpeg_extract_audio"]
	if tool == nil {
		t.Fatal("a profile with a missing binary must still register")
	}
	_, err := tool.Execute(context.Background(),
		`{"input_path":"in.mp4","codec":"aac","output_path":"out.m4a"}`,
		&tooling.Env{CWD: t.TempDir()})
	if err == nil {
		t.Fatal("a missing binary did not error")
	}
	message := err.Error()
	if !strings.Contains(message, "not installed") {
		t.Fatalf("error = %q, want a not-installed diagnosis", message)
	}
	// The hint names either a detected manager command or the config key.
	if !strings.Contains(message, "install") {
		t.Fatalf("error = %q, want install guidance", message)
	}
}

func TestExecuteRejectsAPathEscapeViaDash(t *testing.T) {
	fake, err := cmdtest.Build(t.TempDir(), "ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	cfg := &config.Config{Commands: []cmdprofile.ProfileSpec{testProfile(fake.Binary)}}
	tool := collectTools(cfg)["cmd_ffmpeg_extract_audio"]
	if _, err := tool.Execute(context.Background(),
		`{"input_path":"-loglevel","codec":"aac","output_path":"out.m4a"}`,
		&tooling.Env{CWD: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "dash") {
		t.Fatalf("dashed value error = %v", err)
	}
	if calls, _ := fake.Calls(); len(calls) != 0 {
		t.Fatalf("the binary ran despite the rejection: %#v", calls)
	}
}
