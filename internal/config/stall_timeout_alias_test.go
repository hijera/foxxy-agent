package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fork(stall-timeout-alias) guard: configs written before 0.3.x name the stall
// guard llm_stall_timeout_ms. The key upstream named it,
// llm_stream_idle_timeout_ms, is the one FoxxyCode writes now; the old one is
// still read wherever the file is.
func TestLegacyStallTimeoutKeyIsRead(t *testing.T) {
	cfg, err := parseValidateYAMLBytes("agent:\n  llm_stall_timeout_ms: 1500\n", Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("load a config with the old key: %v", err)
	}
	if got := cfg.Agent.EffectiveLLMStreamIdleTimeout(); got != 1500*time.Millisecond {
		t.Fatalf("llm_stall_timeout_ms: 1500 read as %v", got)
	}

	// Both names in one section: the current one counts, wherever it stands.
	for name, body := range map[string]string{
		"new first": "agent:\n  llm_stream_idle_timeout_ms: 2000\n  llm_stall_timeout_ms: 1500\n",
		"old first": "agent:\n  llm_stall_timeout_ms: 1500\n  llm_stream_idle_timeout_ms: 2000\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseValidateYAMLBytes(body, Paths{Home: t.TempDir()})
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if got := cfg.Agent.EffectiveLLMStreamIdleTimeout(); got != 2*time.Second {
				t.Errorf("llm_stream_idle_timeout_ms: 2000 lost to the old key: %v", got)
			}
		})
	}
}

// The JSON config API reads the old key too and answers with the new one.
func TestConfigJSONReadsLegacyStallTimeout(t *testing.T) {
	cfg, err := ParseAndValidateConfigJSON([]byte(`{"agent":{"llm_stall_timeout_ms":1500}}`), Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("parse config JSON: %v", err)
	}
	if got := cfg.Agent.EffectiveLLMStreamIdleTimeout(); got != 1500*time.Millisecond {
		t.Fatalf("llm_stall_timeout_ms: 1500 read as %v", got)
	}
	out, err := json.Marshal(ConfigToJSONDTO(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "llm_stall_timeout_ms") || !strings.Contains(string(out), `"llm_stream_idle_timeout_ms":1500`) {
		t.Errorf("the config API does not answer with the new key:\n%s", out)
	}
}

// A settings save writes the new name and carries the operator's note along.
func TestSettingsSaveRenamesTheLegacyStallTimeout(t *testing.T) {
	existing := "agent:\n  # the hub stalls a lot\n  llm_stall_timeout_ms: 1500\n"
	cfg := loadForRewrite(t, existing)
	saved := rewrite(t, cfg, existing)
	if strings.Contains(saved, "llm_stall_timeout_ms") {
		t.Errorf("the save kept the old key:\n%s", saved)
	}
	if !strings.Contains(saved, "# the hub stalls a lot\n  llm_stream_idle_timeout_ms: 1500") {
		t.Errorf("the save lost the value or the comment of the key:\n%s", saved)
	}
}

// -t reads the old key without failing the file and names the new one.
func TestCheckLegacyStallTimeoutIsAWarning(t *testing.T) {
	rep := checkYAML(t, withModeline("agent:\n  llm_stall_timeout_ms: 1500\n"))
	if !rep.Valid() {
		t.Fatalf("the old name of the stall timeout must not fail the check: %+v", rep.Findings)
	}
	var found bool
	for _, f := range rep.Findings {
		if f.Path == "agent.llm_stall_timeout_ms" && f.Severity == SeverityWarning &&
			strings.Contains(f.Message, "llm_stream_idle_timeout_ms") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no warning names the new key: %+v", rep.Findings)
	}
}
