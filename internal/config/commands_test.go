package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

func commandsTestProfileYAML() string {
	return `
commands:
  - name: ffmpeg_extract_audio
    binary: ffmpeg
    permission: allow
    template: ["-i", "{input_path}", "-vn", "-acodec", "{codec}", "{output_path}"]
    params:
      - name: input_path
        type: file
      - name: codec
        type: enum
        enum: [libmp3lame, aac]
      - name: output_path
        type: file
    install:
      winget: Gyan.FFmpeg
      scoop: ffmpeg
      apt: ffmpeg
`
}

func loadCommandsConfig(t *testing.T, body string) (*Config, error) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "config.yaml")
	content := "providers:\n  - name: fake\n    type: openai\n    api_key: test\n" +
		"models:\n  - model: fake/model\n" +
		"agent:\n  model: fake/model\n" + body
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadParsesCommandProfiles(t *testing.T) {
	cfg, err := loadCommandsConfig(t, commandsTestProfileYAML())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Commands) != 1 {
		t.Fatalf("commands = %#v", cfg.Commands)
	}
	profile := cfg.Commands[0]
	if profile.Name != "ffmpeg_extract_audio" || profile.Binary != "ffmpeg" {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.ResolvedPermission() != cmdprofile.PermissionAllow {
		t.Fatalf("permission = %q", profile.Permission)
	}
	if len(profile.Template) != 6 || profile.Template[1] != "{input_path}" {
		t.Fatalf("template = %#v", profile.Template)
	}
	if len(profile.Params) != 3 || profile.Params[1].Type != cmdprofile.ParamEnum {
		t.Fatalf("params = %#v", profile.Params)
	}
	if profile.Install.Winget != "Gyan.FFmpeg" {
		t.Fatalf("install = %#v", profile.Install)
	}
}

func TestLoadRejectsAnInvalidCommandProfile(t *testing.T) {
	_, err := loadCommandsConfig(t, `
commands:
  - name: bad name
    binary: ffmpeg
    template: ["-version"]
`)
	if err == nil || !strings.Contains(err.Error(), "commands") {
		t.Fatalf("Load() error = %v, want a commands validation error", err)
	}
}

func TestLoadRejectsDuplicateCommandProfileNames(t *testing.T) {
	_, err := loadCommandsConfig(t, `
commands:
  - name: probe
    binary: ffprobe
    template: ["-version"]
  - name: probe
    binary: ffprobe
    template: ["-version"]
`)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Load() error = %v, want a duplicate-name error", err)
	}
}

// The Settings PUT rewrites config.yaml from the JSON DTO; a section missing
// from the DTO is silently erased from disk on any save. This round-trip is
// the regression net for that hazard.
func TestConfigJSONRoundTripPreservesCommands(t *testing.T) {
	cfg, err := loadCommandsConfig(t, commandsTestProfileYAML())
	if err != nil {
		t.Fatal(err)
	}
	dto := ConfigToJSONDTO(cfg)
	if len(dto.Commands) != 1 {
		t.Fatalf("dto commands = %#v", dto.Commands)
	}
	back := JSONDTOToConfig(dto, cfg.Paths)
	if len(back.Commands) != 1 {
		t.Fatalf("round-tripped commands = %#v", back.Commands)
	}
	if back.Commands[0].Name != cfg.Commands[0].Name ||
		back.Commands[0].Install.Winget != cfg.Commands[0].Install.Winget ||
		len(back.Commands[0].Params) != len(cfg.Commands[0].Params) {
		t.Fatalf("round trip lost fields: %#v", back.Commands[0])
	}
	// The DTO must not share backing arrays with the source config.
	dto.Commands[0].Template[0] = "mutated"
	if cfg.Commands[0].Template[0] == "mutated" {
		t.Fatal("DTO shares the template slice with the config")
	}
}
