//go:build http

package httpserver

// Godog harness for features/config_portable_paths.feature: drives the settings
// screen's read-modify-write cycle (GET /foxxycode/config, then PUT of the same
// document) against a server whose agent home is a directory of the test, and
// inspects the file that lands on disk.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// portablePathsBaseYAML sets none of the process-scoped locations. It pins
// skills.dirs, subagents.dirs and hooks.files to ${CWD} entries, which a load and
// a save carry through as written, so the scenarios see only the locations this
// feature is about.
const portablePathsBaseYAML = `providers:
  - name: valera
    type: openai
    api_key: "k"
models:
  - model: valera/qwen3.8-27b
    max_tokens: 4096
agent:
  model: valera/qwen3.8-27b
skills:
  dirs: ["${CWD}/.foxxycode/skills"]
subagents:
  dirs: ["${CWD}/.foxxycode/agents"]
hooks:
  files: ["${CWD}/.foxxycode/hooks.json"]
`

// portablePathsPlaceholderYAML spells every process-scoped location with a
// placeholder the loader expands.
const portablePathsPlaceholderYAML = portablePathsBaseYAML + `sessions:
  dir: "${FOXXYCODE_HOME}/team-sessions"
memory:
  dir: "~/foxxy-memory"
logger:
  outputs: [stderr]
  file: "${FOXXYCODE_HOME}/logs/agent.log"
scheduler:
  dir: "${CWD}/jobs"
`

type portablePathsWorld struct {
	ts    *httptest.Server
	paths config.Paths
	// saves holds config.yaml as each save left it.
	saves [][]byte
}

func (w *portablePathsWorld) startServer(t *testing.T, yml string) error {
	dir := t.TempDir()
	w.paths = config.Paths{
		Home:       filepath.Join(dir, "home"),
		CWD:        filepath.Join(dir, "launch"),
		ConfigPath: filepath.Join(dir, "config.yaml"),
	}
	if err := os.MkdirAll(w.paths.Home, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(w.paths.ConfigPath, []byte(yml), 0o644); err != nil {
		return err
	}
	cfg, err := config.LoadWithPaths(w.paths)
	if err != nil {
		return err
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), w.paths.Home, nil)
	srv := New(cfg, mgr, slog.Default(), w.paths.Home)
	w.ts = httptest.NewServer(srv.Handler())
	t.Cleanup(w.ts.Close)
	return nil
}

// saveUnchanged is the settings screen saving a config nobody edited: the
// document GET returned goes back as the PUT body.
func (w *portablePathsWorld) saveUnchanged() error {
	if w.ts == nil {
		return fmt.Errorf("server not started")
	}
	res, err := http.Get(w.ts.URL + "/foxxycode/config")
	if err != nil {
		return err
	}
	doc, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /foxxycode/config: status %d, want 200", res.StatusCode)
	}
	req, err := http.NewRequest(http.MethodPut, w.ts.URL+"/foxxycode/config", bytes.NewReader(doc))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	put, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = put.Body.Close() }()
	if put.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(put.Body)
		return fmt.Errorf("PUT /foxxycode/config: status %d, want 200 (%s)", put.StatusCode, raw)
	}
	saved, err := os.ReadFile(w.paths.ConfigPath)
	if err != nil {
		return err
	}
	w.saves = append(w.saves, saved)
	return nil
}

func (w *portablePathsWorld) lastSave() ([]byte, error) {
	if len(w.saves) == 0 {
		return nil, fmt.Errorf("nothing was saved")
	}
	return w.saves[len(w.saves)-1], nil
}

// savedValue reads one dotted key of the last saved file as a YAML parser does.
func (w *portablePathsWorld) savedValue(key string) (string, error) {
	saved, err := w.lastSave()
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(saved, &doc); err != nil {
		return "", fmt.Errorf("parse saved config: %w", err)
	}
	var cur any = doc
	for _, part := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%s: no mapping at %q in the saved config:\n%s", key, part, saved)
		}
		cur = m[part]
	}
	if cur == nil {
		return "", nil
	}
	s, ok := cur.(string)
	if !ok {
		return "", fmt.Errorf("%s = %v (%T) in the saved config, want a string", key, cur, cur)
	}
	return s, nil
}

func (w *portablePathsWorld) wantValue(key, want string) error {
	got, err := w.savedValue(key)
	if err != nil {
		return err
	}
	if got != want {
		saved, _ := w.lastSave()
		return fmt.Errorf("saved %s = %q, want %q:\n%s", key, got, want, saved)
	}
	return nil
}

func (w *portablePathsWorld) wantEmpty(key string) error {
	return w.wantValue(key, "")
}

func (w *portablePathsWorld) wantNoAgentHome() error {
	saved, err := w.lastSave()
	if err != nil {
		return err
	}
	for _, home := range []string{w.paths.Home, filepath.ToSlash(w.paths.Home)} {
		if bytes.Contains(saved, []byte(home)) {
			return fmt.Errorf("the saved config names the agent home %s:\n%s", home, saved)
		}
	}
	return nil
}

func (w *portablePathsWorld) wantSameSaves() error {
	if len(w.saves) < 2 {
		return fmt.Errorf("%d saves, want two", len(w.saves))
	}
	first, second := w.saves[len(w.saves)-2], w.saves[len(w.saves)-1]
	if !bytes.Equal(first, second) {
		return fmt.Errorf("the second save changed the file\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	return nil
}

// wantUnderAnotherHome reads the saved file the way a process started with
// another --home would.
func (w *portablePathsWorld) wantUnderAnotherHome(key string) error {
	if key != "scheduler.dir" {
		return fmt.Errorf("no reader for %q", key)
	}
	moved := w.paths
	moved.Home = filepath.Join(filepath.Dir(w.paths.Home), "another-home")
	cfg, err := config.LoadWithPaths(moved)
	if err != nil {
		return err
	}
	if got, want := cfg.Scheduler.Dir, filepath.Join(moved.Home, "scheduler"); got != want {
		return fmt.Errorf("scheduler.dir read with another home = %q, want %q", got, want)
	}
	return nil
}

func TestConfigPortablePathsFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "config_portable_paths",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			w := &portablePathsWorld{}
			sc.Step(`^a foxxycode server whose config\.yaml leaves scheduler\.dir unset$`, func() error {
				return w.startServer(t, portablePathsBaseYAML)
			})
			sc.Step(`^a foxxycode server whose config\.yaml spells its locations with placeholders$`, func() error {
				return w.startServer(t, portablePathsPlaceholderYAML)
			})
			sc.Step(`^the settings screen saves the config unchanged$`, w.saveUnchanged)
			sc.Step(`^the settings screen saves the config unchanged again$`, w.saveUnchanged)
			sc.Step(`^the saved config\.yaml leaves "([^"]*)" empty$`, w.wantEmpty)
			sc.Step(`^the saved config\.yaml sets "([^"]*)" to "([^"]*)"$`, w.wantValue)
			sc.Step(`^the saved config\.yaml names no path under the agent home$`, w.wantNoAgentHome)
			sc.Step(`^both saves wrote the same config\.yaml$`, w.wantSameSaves)
			sc.Step(`^the saved config\.yaml, read with another agent home, keeps "([^"]*)" under that home$`, w.wantUnderAnotherHome)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/config_portable_paths.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("config_portable_paths feature failed")
	}
}
