package config_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// placeholderBaseYAML sets none of skills.dirs, subagents.dirs and hooks.files, the
// state of almost every config on disk.
const placeholderBaseYAML = `providers:
  - name: local
    type: openai
    api_key: "k"
models:
  - model: local/gpt-4o
    max_tokens: 4096
agent:
  model: local/gpt-4o
`

// placeholderOperatorYAML lists its own locations, spelled with every placeholder the
// loaders understand plus one absolute path.
const placeholderOperatorYAML = placeholderBaseYAML + `skills:
  dirs:
    - "~/team-skills"
    - "${FOXXYCODE_HOME}/skills"
    - "${CWD}/.agents/skills"
    - "/opt/foxxycode/skills"
subagents:
  dirs:
    - "${FOXXYCODE_HOME}/team-agents"
    - "${CWD}/.claude/agents"
hooks:
  files:
    - "~/.config/foxxycode/hooks.json"
    - "${FOXXYCODE_HOME}/hooks.json"
`

func placeholderPaths(t *testing.T, body string) config.Paths {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return config.Paths{Home: filepath.Join(dir, "home"), CWD: filepath.Join(dir, "launch"), ConfigPath: path}
}

// resaveLoaded does what a save of an unchanged config does: load the file, render it
// over itself, and write the result back.
func resaveLoaded(t *testing.T, paths config.Paths) []byte {
	t.Helper()
	cfg, err := config.LoadWithPaths(paths)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return writeRendered(t, cfg, paths)
}

// resaveFromSettings does what the Settings screen does: GET the config as JSON and
// PUT the same document back.
func resaveFromSettings(t *testing.T, paths config.Paths) []byte {
	t.Helper()
	live, err := config.LoadWithPaths(paths)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	body, err := json.Marshal(config.ConfigToJSONDTO(live))
	if err != nil {
		t.Fatalf("encode GET body: %v", err)
	}
	next, err := config.ParseConfigJSONPreservingSecrets(body, paths, live)
	if err != nil {
		t.Fatalf("parse PUT body: %v", err)
	}
	return writeRendered(t, next, paths)
}

func writeRendered(t *testing.T, cfg *config.Config, paths config.Paths) []byte {
	t.Helper()
	out, err := config.MarshalConfigYAMLForFile(cfg, paths.ConfigPath)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := config.AtomicWriteConfigYAML(paths.ConfigPath, out); err != nil {
		t.Fatalf("write: %v", err)
	}
	return out
}

// savedPathLists reads the three per-session path lists back out of a saved file.
func savedPathLists(t *testing.T, saved []byte) map[string][]string {
	t.Helper()
	var doc struct {
		Skills struct {
			Dirs []string `yaml:"dirs"`
		} `yaml:"skills"`
		Subagents struct {
			Dirs []string `yaml:"dirs"`
		} `yaml:"subagents"`
		Hooks struct {
			Files []string `yaml:"files"`
		} `yaml:"hooks"`
	}
	if err := yaml.Unmarshal(saved, &doc); err != nil {
		t.Fatalf("parse saved config: %v\n%s", err, saved)
	}
	return map[string][]string{
		"skills.dirs":    doc.Skills.Dirs,
		"subagents.dirs": doc.Subagents.Dirs,
		"hooks.files":    doc.Hooks.Files,
	}
}

func wantPathLists(t *testing.T, saved []byte, want map[string][]string) {
	t.Helper()
	got := savedPathLists(t, saved)
	for key, list := range want {
		if strings.Join(got[key], "|") != strings.Join(list, "|") {
			t.Errorf("%s saved as %q, want %q", key, got[key], list)
		}
	}
}

func wantSameSave(t *testing.T, first, second []byte) {
	t.Helper()
	if !bytes.Equal(first, second) {
		t.Fatalf("saving an unchanged config twice wrote different files\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

var defaultPathLists = map[string][]string{
	"skills.dirs":    config.DefaultSkillDirs(),
	"subagents.dirs": config.DefaultSubagentDirs(),
	"hooks.files":    config.DefaultHookFiles(),
}

// A config that leaves the three lists to their defaults used to be written with the
// placeholders on the first save and with this machine's home directories on the
// second, so the file stopped following FOXXYCODE_HOME and could not be copied to
// another machine or user.
func TestSavingAnUnchangedConfigTwiceWritesTheSameFile(t *testing.T) {
	paths := placeholderPaths(t, placeholderBaseYAML)
	first := resaveLoaded(t, paths)
	second := resaveLoaded(t, paths)
	wantPathLists(t, second, defaultPathLists)
	wantSameSave(t, first, second)
}

// The Settings screen reads the config as JSON and writes the same document back; the
// defaults it was shown must reach the file as placeholders, on every save.
func TestSettingsSaveKeepsTheDefaultPathPlaceholders(t *testing.T) {
	paths := placeholderPaths(t, placeholderBaseYAML)
	first := resaveFromSettings(t, paths)
	wantPathLists(t, first, defaultPathLists)
	second := resaveFromSettings(t, paths)
	wantSameSave(t, first, second)
}

// Entries the operator wrote keep their spelling through a load, the Settings round
// trip and a save: ${FOXXYCODE_HOME} and a leading ~ are resolved by whatever reads
// the directory, not written into the file as this machine's paths.
func TestSavesKeepTheOperatorsPathSpelling(t *testing.T) {
	want := map[string][]string{
		"skills.dirs": {
			"~/team-skills", "${FOXXYCODE_HOME}/skills", "${CWD}/.agents/skills", "/opt/foxxycode/skills",
		},
		"subagents.dirs": {"${FOXXYCODE_HOME}/team-agents", "${CWD}/.claude/agents"},
		"hooks.files":    {"~/.config/foxxycode/hooks.json", "${FOXXYCODE_HOME}/hooks.json"},
	}
	paths := placeholderPaths(t, placeholderOperatorYAML)

	cfg, err := config.LoadWithPaths(paths)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded := map[string][]string{
		"skills.dirs": cfg.Skills.Dirs, "subagents.dirs": cfg.Subagents.Dirs, "hooks.files": cfg.Hooks.Files,
	}
	dto := config.ConfigToJSONDTO(cfg)
	reported := map[string][]string{
		"skills.dirs": dto.Skills.Dirs, "subagents.dirs": dto.Subagents.Dirs, "hooks.files": dto.Hooks.Files,
	}
	for key, list := range want {
		if strings.Join(loaded[key], "|") != strings.Join(list, "|") {
			t.Errorf("%s loaded as %q, want %q", key, loaded[key], list)
		}
		if strings.Join(reported[key], "|") != strings.Join(list, "|") {
			t.Errorf("GET /foxxycode/config reports %s as %q, want %q", key, reported[key], list)
		}
	}

	first := resaveFromSettings(t, paths)
	wantPathLists(t, first, want)
	wantSameSave(t, first, resaveFromSettings(t, paths))
	wantSameSave(t, first, resaveLoaded(t, paths))
}

// Only the text ${FOXXYCODE_HOME} is kept as a placeholder. A path that spells the home
// directory out stays the path the operator pinned, and an environment reference next
// to the placeholder is still resolved when the file is read, as everywhere else.
func TestLoadKeepsOnlyTheHomePlaceholderItself(t *testing.T) {
	t.Setenv("FOXXYCODE_TEST_TEAM", "team-a")
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	pinned := filepath.ToSlash(filepath.Join(home, "pinned-skills"))
	body := placeholderBaseYAML + `skills:
  dirs:
    - "` + pinned + `"
    - "${FOXXYCODE_HOME}/${FOXXYCODE_TEST_TEAM}"
    - "$${FOXXYCODE_TEST_TEAM}/literal"
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithPaths(config.Paths{Home: home, CWD: dir, ConfigPath: path})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{pinned, "${FOXXYCODE_HOME}/team-a", "${FOXXYCODE_TEST_TEAM}/literal"}
	if strings.Join(cfg.Skills.Dirs, "|") != strings.Join(want, "|") {
		t.Fatalf("skills.dirs loaded as %q, want %q", cfg.Skills.Dirs, want)
	}
}
