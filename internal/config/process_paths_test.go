package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// perSessionListsYAML pins skills.dirs, subagents.dirs and hooks.files to ${CWD}
// entries, which a load and a save carry through as written, so the tests in this
// file see only the process-scoped locations.
const perSessionListsYAML = `skills:
  dirs: ["${CWD}/.foxxycode/skills"]
subagents:
  dirs: ["${CWD}/.foxxycode/agents"]
hooks:
  files: ["${CWD}/.foxxycode/hooks.json"]
`

// processPathsBaseYAML is the smallest config the loader accepts. It sets none of
// the process-scoped locations, which is the state of almost every config on disk.
const processPathsBaseYAML = `providers:
  - name: local
    type: openai
    api_key: "k"
models:
  - model: local/gpt-4o
    max_tokens: 4096
agent:
  model: local/gpt-4o
` + perSessionListsYAML

// processPathCase is one process-scoped location spelled one way in the file.
type processPathCase struct {
	name string
	// yaml is appended to processPathsBaseYAML.
	yaml string
	// key is the dotted path of the field in the saved file.
	key string
	// written is what a save must leave in the file.
	written string
	// resolved is what the loaded config hands its consumers.
	resolved func(p Paths) string
	// read returns the loaded value of the field.
	read func(c *Config) string
}

func userHomeOrSkip(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home directory: %v", err)
	}
	return home
}

func processPathCases(t *testing.T) []processPathCase {
	t.Helper()
	// Forward slashes: the reference is expanded in the text of the file, where a
	// backslash inside a quoted scalar would start an escape sequence.
	t.Setenv("FOXXYCODE_PP_LOGS", filepath.ToSlash(filepath.Join(t.TempDir(), "logs")))
	userHome := userHomeOrSkip(t)
	scheduler := func(c *Config) string { return c.Scheduler.Dir }
	sessions := func(c *Config) string { return c.Sessions.Dir }
	memory := func(c *Config) string { return c.Memory.Dir }
	logFile := func(c *Config) string { return c.Logger.File }
	return []processPathCase{
		{
			name:     "scheduler.dir left to its default",
			key:      "scheduler.dir",
			written:  "",
			resolved: func(p Paths) string { return filepath.Join(p.Home, "scheduler") },
			read:     scheduler,
		},
		{
			name:     "scheduler.dir under the home placeholder",
			yaml:     "scheduler:\n  dir: \"${FOXXYCODE_HOME}/jobs\"\n",
			key:      "scheduler.dir",
			written:  "${FOXXYCODE_HOME}/jobs",
			resolved: func(p Paths) string { return filepath.Join(p.Home, "jobs") },
			read:     scheduler,
		},
		{
			name:     "scheduler.dir under the user home",
			yaml:     "scheduler:\n  dir: ~/foxxy-jobs\n",
			key:      "scheduler.dir",
			written:  "~/foxxy-jobs",
			resolved: func(Paths) string { return filepath.Join(userHome, "foxxy-jobs") },
			read:     scheduler,
		},
		{
			name:     "sessions.dir under the home placeholder",
			yaml:     "sessions:\n  dir: \"${FOXXYCODE_HOME}/team-sessions\"\n",
			key:      "sessions.dir",
			written:  "${FOXXYCODE_HOME}/team-sessions",
			resolved: func(p Paths) string { return filepath.Join(p.Home, "team-sessions") },
			read:     sessions,
		},
		{
			name:     "sessions.dir with an escaped dollar",
			yaml:     "sessions:\n  dir: \"${FOXXYCODE_HOME}/$$shared\"\n",
			key:      "sessions.dir",
			written:  "${FOXXYCODE_HOME}/$$shared",
			resolved: func(p Paths) string { return filepath.Join(p.Home, "$shared") },
			read:     sessions,
		},
		{
			name:     "memory.dir under the user home",
			yaml:     "memory:\n  dir: \"~/foxxy-memory\"\n",
			key:      "memory.dir",
			written:  "~/foxxy-memory",
			resolved: func(Paths) string { return filepath.Join(userHome, "foxxy-memory") },
			read:     memory,
		},
		{
			name:     "memory.dir under the launch directory",
			yaml:     "memory:\n  dir: \"${CWD}/.foxxycode/memory\"\n",
			key:      "memory.dir",
			written:  "${CWD}/.foxxycode/memory",
			resolved: func(p Paths) string { return filepath.Join(p.CWD, ".foxxycode", "memory") },
			read:     memory,
		},
		{
			name:     "logger.file under the home placeholder",
			yaml:     "logger:\n  outputs: [file]\n  file: \"${FOXXYCODE_HOME}/logs/agent.log\"\n",
			key:      "logger.file",
			written:  "${FOXXYCODE_HOME}/logs/agent.log",
			resolved: func(p Paths) string { return filepath.Join(p.Home, "logs", "agent.log") },
			read:     logFile,
		},
		{
			name:    "logger.file under an environment reference",
			yaml:    "logger:\n  outputs: [file]\n  file: \"${FOXXYCODE_PP_LOGS}/agent.log\"\n",
			key:     "logger.file",
			written: "${FOXXYCODE_PP_LOGS}/agent.log",
			resolved: func(Paths) string {
				return filepath.Join(filepath.FromSlash(os.Getenv("FOXXYCODE_PP_LOGS")), "agent.log")
			},
			read: logFile,
		},
	}
}

// processPathsFixture writes body as config.yaml in a fresh directory whose
// agent home and launch directory are subdirectories of it.
func processPathsFixture(t *testing.T, body string) Paths {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return Paths{Home: filepath.Join(dir, "home"), CWD: filepath.Join(dir, "launch"), ConfigPath: path}
}

// saveOver renders cfg over the file at paths and writes the result, as every
// save path does.
func saveOver(t *testing.T, cfg *Config, paths Paths) []byte {
	t.Helper()
	out, err := MarshalConfigYAMLForFile(cfg, paths.ConfigPath)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := AtomicWriteConfigYAML(paths.ConfigPath, out); err != nil {
		t.Fatalf("write: %v", err)
	}
	return out
}

func loadOrFail(t *testing.T, paths Paths) *Config {
	t.Helper()
	cfg, err := LoadWithPaths(paths)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

// resaveUnchanged loads the file and saves it back untouched.
func resaveUnchanged(t *testing.T, paths Paths) []byte {
	t.Helper()
	return saveOver(t, loadOrFail(t, paths), paths)
}

// resaveThroughSettings does what the Settings screen does with a config nobody
// edited: GET /foxxycode/config, then PUT the same document back.
func resaveThroughSettings(t *testing.T, paths Paths) []byte {
	t.Helper()
	return saveOver(t, settingsRoundTrip(t, loadOrFail(t, paths), paths, nil), paths)
}

// settingsRoundTrip renders live the way GET /foxxycode/config does, lets edit
// change the document, and parses it back the way PUT /foxxycode/config does.
func settingsRoundTrip(t *testing.T, live *Config, paths Paths, edit func(*ConfigJSON)) *Config {
	t.Helper()
	dto := ConfigToJSONDTO(live)
	if edit != nil {
		edit(dto)
	}
	body, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("encode GET body: %v", err)
	}
	next, err := ParseConfigJSONPreservingSecrets(body, paths, live)
	if err != nil {
		t.Fatalf("parse PUT body: %v", err)
	}
	return next
}

// savedValue reads one dotted key of a saved file as the YAML parser sees it.
func savedValue(t *testing.T, saved []byte, key string) string {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal(saved, &doc); err != nil {
		t.Fatalf("parse saved file: %v\n%s", err, saved)
	}
	var cur any = doc
	for _, part := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("%s: %v is not a mapping in the saved file:\n%s", key, cur, saved)
		}
		cur = m[part]
	}
	if cur == nil {
		return ""
	}
	s, ok := cur.(string)
	if !ok {
		t.Fatalf("%s = %v (%T) in the saved file, want a string", key, cur, cur)
	}
	return s
}

// assertNoHomeIn fails when the saved file spells out the agent home anywhere.
func assertNoHomeIn(t *testing.T, saved []byte, home string) {
	t.Helper()
	for _, spelling := range []string{home, filepath.ToSlash(home)} {
		if bytes.Contains(saved, []byte(spelling)) {
			t.Fatalf("the saved file pins the agent home %s:\n%s", spelling, saved)
		}
	}
}

// TestSaveKeepsProcessPathsAsWritten is the regression for the first save from
// the Settings screen pinning the process-scoped locations to this machine:
// a defaulted scheduler.dir came back as <home>/scheduler, and sessions.dir,
// memory.dir, logger.file and scheduler.dir spelled with ${FOXXYCODE_HOME}, ~,
// ${CWD} or an environment reference came back expanded. The file stopped
// following FOXXYCODE_HOME (--home) and a copy on another machine pointed at
// the old home.
func TestSaveKeepsProcessPathsAsWritten(t *testing.T) {
	saves := []struct {
		name string
		save func(*testing.T, Paths) []byte
	}{
		{"load and save", resaveUnchanged},
		{"settings screen", resaveThroughSettings},
	}
	for _, tc := range processPathCases(t) {
		for _, save := range saves {
			t.Run(tc.name+"/"+save.name, func(t *testing.T) {
				paths := processPathsFixture(t, processPathsBaseYAML+tc.yaml)

				// Consumers keep reading a resolved, absolute location.
				loaded := loadOrFail(t, paths)
				if got, want := tc.read(loaded), tc.resolved(paths); got != want {
					t.Fatalf("loaded %s = %q, want %q", tc.key, got, want)
				}

				first := save.save(t, paths)
				if got := savedValue(t, first, tc.key); got != tc.written {
					t.Fatalf("saved %s = %q, want %q as written\n%s", tc.key, got, tc.written, first)
				}
				assertNoHomeIn(t, first, paths.Home)

				second := save.save(t, paths)
				if !bytes.Equal(first, second) {
					t.Fatalf("a second save of an unchanged config changed the file\nfirst:\n%s\nsecond:\n%s", first, second)
				}
				if got, want := tc.read(loadOrFail(t, paths)), tc.resolved(paths); got != want {
					t.Fatalf("reloaded %s = %q, want %q", tc.key, got, want)
				}
			})
		}
	}
}

// TestSavedProcessPathsFollowTheHome is what keeping the spelling is for: the
// saved file, read with another agent home (--home, another user, a Docker
// volume), resolves under that home rather than the one it was saved from.
func TestSavedProcessPathsFollowTheHome(t *testing.T) {
	paths := processPathsFixture(t, processPathsBaseYAML+"sessions:\n  dir: \"${FOXXYCODE_HOME}/team-sessions\"\n")
	resaveThroughSettings(t, paths)

	moved := paths
	moved.Home = filepath.Join(filepath.Dir(paths.Home), "other-home")
	cfg := loadOrFail(t, moved)
	if got, want := cfg.Scheduler.Dir, filepath.Join(moved.Home, "scheduler"); got != want {
		t.Fatalf("scheduler.dir under another home = %q, want %q", got, want)
	}
	if got, want := cfg.Sessions.Dir, filepath.Join(moved.Home, "team-sessions"); got != want {
		t.Fatalf("sessions.dir under another home = %q, want %q", got, want)
	}
}

// TestSaveWritesAnEditedProcessPath keeps the spelling from outliving the value
// it spelled: a location changed in memory or on the Settings screen is saved
// as the new value, a placeholder typed into the Settings screen is saved as
// typed, and a field cleared there goes back to its default.
func TestSaveWritesAnEditedProcessPath(t *testing.T) {
	body := processPathsBaseYAML + "scheduler:\n  dir: \"${FOXXYCODE_HOME}/jobs\"\n"

	t.Run("changed in memory", func(t *testing.T) {
		paths := processPathsFixture(t, body)
		cfg := loadOrFail(t, paths)
		elsewhere := filepath.Join(filepath.Dir(paths.Home), "elsewhere")
		cfg.Scheduler.Dir = elsewhere
		if got := savedValue(t, saveOver(t, cfg, paths), "scheduler.dir"); got != elsewhere {
			t.Fatalf("saved scheduler.dir = %q, want the edited %q", got, elsewhere)
		}
	})

	settingsCases := []struct {
		name  string
		value func(Paths) string
		want  func(Paths) string
	}{
		{
			name:  "an absolute path",
			value: func(p Paths) string { return filepath.Join(filepath.Dir(p.Home), "elsewhere") },
			want:  func(p Paths) string { return filepath.Join(filepath.Dir(p.Home), "elsewhere") },
		},
		{
			name:  "a placeholder",
			value: func(Paths) string { return "${FOXXYCODE_HOME}/other-jobs" },
			want:  func(Paths) string { return "${FOXXYCODE_HOME}/other-jobs" },
		},
		{
			name:  "cleared",
			value: func(Paths) string { return "" },
			want:  func(Paths) string { return "" },
		},
	}
	for _, tc := range settingsCases {
		t.Run("settings screen sets "+tc.name, func(t *testing.T) {
			paths := processPathsFixture(t, body)
			next := settingsRoundTrip(t, loadOrFail(t, paths), paths, func(j *ConfigJSON) {
				j.Scheduler.Dir = tc.value(paths)
			})
			if got, want := savedValue(t, saveOver(t, next, paths), "scheduler.dir"), tc.want(paths); got != want {
				t.Fatalf("saved scheduler.dir = %q, want %q", got, want)
			}
		})
	}
}

// TestFileWatcherComparesProcessPathsAsWritten keeps the watcher's view in step
// with the save: the file a save renders from the live configuration is not a
// change, a new location is, and so is a new spelling of the same location -
// otherwise the live configuration would keep the old spelling and the next
// save from the Settings screen would put it back over the operator's edit.
func TestFileWatcherComparesProcessPathsAsWritten(t *testing.T) {
	w, _, live := watchFixture(t, watchBaseYAML+perSessionListsYAML+"scheduler:\n  dir: \"${FOXXYCODE_HOME}/jobs\"\n")
	w.Poll()
	saved, err := MarshalConfigYAMLForFile(live(), w.Paths.ConfigPath)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rewriteWatched(t, w, string(saved))
	if w.Poll() {
		t.Fatal("the file a save wrote came back as a configuration change")
	}

	moved := strings.ReplaceAll(string(saved), "${FOXXYCODE_HOME}/jobs", "${FOXXYCODE_HOME}/other-jobs")
	rewriteWatched(t, w, moved)
	if !w.Poll() {
		t.Fatal("a new scheduler.dir was not installed")
	}
	if got, want := live().Scheduler.Dir, filepath.Join(w.Paths.Home, "other-jobs"); got != want {
		t.Fatalf("installed scheduler.dir = %q, want %q", got, want)
	}

	absolute := strings.ReplaceAll(moved, "${FOXXYCODE_HOME}/other-jobs", filepath.ToSlash(filepath.Join(w.Paths.Home, "other-jobs")))
	rewriteWatched(t, w, absolute)
	if !w.Poll() {
		t.Fatal("a new spelling of scheduler.dir was not installed")
	}
	resaved, err := MarshalConfigYAMLForFile(live(), w.Paths.ConfigPath)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got, want := savedValue(t, resaved, "scheduler.dir"), filepath.ToSlash(filepath.Join(w.Paths.Home, "other-jobs")); got != want {
		t.Fatalf("a save after the edit wrote scheduler.dir = %q, want the operator's %q", got, want)
	}
}
