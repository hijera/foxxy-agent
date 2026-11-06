package config_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

const idleTimeoutYAML = `
providers:
  - name: openai
    type: openai
    api_key: "k"
models:
  - model: "openai/gpt-4o"
agent:
  model: "openai/gpt-4o"
mcp:
  project_trust: allow
  idle_timeout_seconds: 42
`

// mcp.idle_timeout_seconds is read from the file, survives the settings
// screen's JSON round trip, and defaults to five minutes when absent.
func TestMCPIdleTimeoutLoadsAndSurvivesTheJSONRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvFOXXYCODEHome, home)
	p := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(p, []byte(idleTimeoutYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.MCP.EffectiveIdleTimeout(); got != 42*time.Second {
		t.Fatalf("EffectiveIdleTimeout = %s, want 42s", got)
	}
	raw, err := json.Marshal(config.ConfigToJSONDTO(cfg))
	if err != nil {
		t.Fatal(err)
	}
	back, err := config.ParseAndValidateConfigJSON(raw, cfg.Paths)
	if err != nil {
		t.Fatal(err)
	}
	if back.MCP.IdleTimeoutSeconds == nil || *back.MCP.IdleTimeoutSeconds != 42 {
		t.Fatalf("idle_timeout_seconds after the round trip = %v, want 42", back.MCP.IdleTimeoutSeconds)
	}
	if got := (config.MCP{}).EffectiveIdleTimeout(); got != 5*time.Minute {
		t.Fatalf("default EffectiveIdleTimeout = %s, want 5m", got)
	}
	zero := 0
	if got := (config.MCP{IdleTimeoutSeconds: &zero}).EffectiveIdleTimeout(); got != 0 {
		t.Fatalf("explicit 0 = %s, want 0", got)
	}
}

// A negative idle timeout is refused when the file is loaded.
func TestMCPIdleTimeoutRejectsANegativeValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.EnvFOXXYCODEHome, home)
	p := filepath.Join(home, "config.yaml")
	body := strings.Replace(idleTimeoutYAML, "idle_timeout_seconds: 42", "idle_timeout_seconds: -1", 1)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(p); err == nil || !strings.Contains(err.Error(), "idle_timeout_seconds") {
		t.Fatalf("Load with a negative idle timeout: err = %v", err)
	}
}

// The -mcp-project-trust flag replaces the policy only; the idle timeout the
// file set stays.
func TestProjectTrustFlagKeepsTheIdleTimeout(t *testing.T) {
	seconds := 42
	cfg := &config.Config{MCP: config.MCP{ProjectTrust: config.ProjectTrustAsk, IdleTimeoutSeconds: &seconds}}
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	val := fs.String(config.ProjectTrustFlagName, "", "")
	if err := fs.Parse([]string{"-" + config.ProjectTrustFlagName + "=deny"}); err != nil {
		t.Fatal(err)
	}
	if err := config.ApplyProjectTrustFlag(fs, cfg, val); err != nil {
		t.Fatal(err)
	}
	if cfg.MCP.ProjectTrust != config.ProjectTrustDeny {
		t.Fatalf("project_trust = %q, want deny", cfg.MCP.ProjectTrust)
	}
	if cfg.MCP.IdleTimeoutSeconds == nil || *cfg.MCP.IdleTimeoutSeconds != 42 {
		t.Fatalf("the flag dropped idle_timeout_seconds: %v", cfg.MCP.IdleTimeoutSeconds)
	}
}
