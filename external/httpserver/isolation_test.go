//go:build http

package httpserver

import (
	"fmt"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testHomeEnv hands the home TestMain made to the helper processes that
// re-execute this test binary (the fake MCP servers), so they neither make
// nor remove one of their own.
const testHomeEnv = "FOXXYCODE_TEST_HTTPSERVER_HOME"

// TestMain points FOXXYCODE_HOME at an empty directory of the test run's own
// for the whole package (see TestTestsDoNotResolveTheOperatorHome). A test
// that needs a home of its own still sets FOXXYCODE_HOME itself. HOME stays the
// operator's on purpose: tests run git in temp repositories and need its
// identity, so ~-paths such as the default ~/.agents/skills are not isolated.
// CODEX_HOME and the Codex backend are isolated as well (see
// TestTestsDoNotReachTheOperatorsCodexLogin).
func TestMain(m *testing.M) {
	if home := os.Getenv(testHomeEnv); home != "" {
		// A helper process, or a run nested in one: the home is the parent's,
		// and so is the Codex backend, inherited as it is. CODEX_HOME is set
		// again in any case, and a backend that is not a local one is replaced,
		// so a marker left in the environment by hand does not open the way to
		// the operator's Codex login either.
		_ = os.Setenv("FOXXYCODE_HOME", home)
		_ = os.Setenv("CODEX_HOME", filepath.Join(home, "codex-home"))
		if base, err := url.Parse(os.Getenv(llm.EnvCodexBaseURL)); err != nil || !isLoopbackIP(base.Hostname()) {
			_ = os.Setenv(llm.EnvCodexBaseURL, "http://127.0.0.1:1")
		}
		os.Exit(m.Run())
	}
	home, err := os.MkdirTemp("", "foxxycode-httpserver-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "test home:", err)
		os.Exit(1)
	}
	codexHome, err := os.MkdirTemp("", "foxxycode-httpserver-codex-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "test codex home:", err)
		os.Exit(1)
	}
	codexBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no Codex backend in this test: point "+llm.EnvCodexBaseURL+" at a stand-in", http.StatusServiceUnavailable)
	}))
	_ = os.Setenv(testHomeEnv, home)
	_ = os.Setenv("FOXXYCODE_HOME", home)
	_ = os.Setenv("CODEX_HOME", codexHome)
	_ = os.Setenv(llm.EnvCodexBaseURL, codexBackend.URL)
	code := m.Run()
	codexBackend.Close()
	for _, dir := range []string{home, codexHome} {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "test home:", err)
		}
	}
	os.Exit(code)
}

// No test of this package may read the Codex CLI login of whoever runs the
// tests, or send it to the real Codex backend: a codex model has its catalog
// read for its context window whenever GET /v1/models is served or a turn
// starts, with the CLI login standing in for the only codex row. TestMain
// points CODEX_HOME at an empty directory under the temp dir and
// FOXXYCODE_CODEX_BASE_URL at a local server that refuses every request; a test
// with a Codex stand-in of its own still sets both itself.
func TestTestsDoNotReachTheOperatorsCodexLogin(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	codexHome, err := filepath.EvalSymlinks(os.Getenv("CODEX_HOME"))
	if err != nil || !strings.HasPrefix(codexHome, tmp+string(filepath.Separator)) {
		t.Fatalf("CODEX_HOME = %q (%v), want a directory of the run's own under %q", os.Getenv("CODEX_HOME"), err, tmp)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "auth.json")); err == nil {
		t.Fatalf("CODEX_HOME %s holds a Codex CLI login", codexHome)
	}
	base, err := url.Parse(os.Getenv(llm.EnvCodexBaseURL))
	if err != nil || !isLoopbackIP(base.Hostname()) {
		t.Fatalf("%s = %q, want the local server TestMain started", llm.EnvCodexBaseURL, os.Getenv(llm.EnvCodexBaseURL))
	}
}

// isLoopbackIP reports whether host is a loopback IP address; httptest binds
// 127.0.0.1, or [::1] on a host without IPv4 loopback. Unlike isLoopbackHost,
// which reads a listen address, an empty host is not one.
func isLoopbackIP(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// A test of this package that loads a config without naming a home (the
// config.Load(path) most of them use) must not read the home of whoever runs
// the tests: its .env would land in this process, its mcp.json servers would
// join every session, its hooks and skills would run. The home such a load
// resolves is the directory TestMain made for the run, under the temp dir.
func TestTestsDoNotResolveTheOperatorHome(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("agent:\n  model: fake/model\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	runHome := os.Getenv(testHomeEnv)
	if runHome == "" {
		t.Fatalf("no home of the test run's own; config home = %q", cfg.Paths.Home)
	}
	want, err := filepath.EvalSymlinks(runHome)
	if err != nil {
		t.Fatal(err)
	}
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(cfg.Paths.Home)
	if err != nil || home != want || !strings.HasPrefix(home, tmp+string(filepath.Separator)) {
		t.Fatalf("config home = %q (%v), want the run's own %q under %q", cfg.Paths.Home, err, want, tmp)
	}
}
