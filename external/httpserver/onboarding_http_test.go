//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// onboardingStatusFor serves yml from a temp home and returns the decoded
// GET /foxxycode/onboarding/status body.
func onboardingStatusFor(t *testing.T, yml string) map[string]interface{} {
	t.Helper()
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), home, nil)
	srv := New(cfg, mgr, slog.Default(), home)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/foxxycode/onboarding/status")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ioReadAllClose(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", res.StatusCode, string(b))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func missingKeysOf(body map[string]interface{}) []string {
	raw, _ := body["missing_api_keys"].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

// The agent's provider is keyed; a second provider saved without any key is
// listed for information but must not flip the picker gate. This is the
// "added openai in Settings, pressed Save, onboarding popped up" bug.
const onboardingKeyedAgentPlusKeylessYAML = `
providers:
  - name: neuraldeep
    type: neuraldeep
    api_key: "nd-test"
  - name: openai
    type: openai

models:
  - model: "neuraldeep/qwen3.6-35b-a3b"
    max_tokens: 4096

agent:
  model: "neuraldeep/qwen3.6-35b-a3b"
`

func TestOnboardingStatusKeylessSecondProviderKeepsAgentReady(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("NEURALDEEP_API_KEY", "")
	body := onboardingStatusFor(t, onboardingKeyedAgentPlusKeylessYAML)
	if body["has_agent_credentials"] != true {
		t.Fatalf("has_agent_credentials %v want true: %v", body["has_agent_credentials"], body)
	}
	if got := missingKeysOf(body); len(got) != 1 || got[0] != "openai" {
		t.Fatalf("missing_api_keys %v want [openai]", got)
	}
}

func TestOnboardingStatusEnvVarCountsAsCredential(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	t.Setenv("NEURALDEEP_API_KEY", "")
	body := onboardingStatusFor(t, onboardingKeyedAgentPlusKeylessYAML)
	if got := missingKeysOf(body); len(got) != 0 {
		t.Fatalf("missing_api_keys %v want none (OPENAI_API_KEY is set)", got)
	}
}

func TestOnboardingStatusAgentProviderWithoutCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	body := onboardingStatusFor(t, `
providers:
  - name: openai
    type: openai

models:
  - model: "openai/gpt-4o"
    max_tokens: 4096

agent:
  model: "openai/gpt-4o"
`)
	if body["has_agent_credentials"] != false {
		t.Fatalf("has_agent_credentials %v want false", body["has_agent_credentials"])
	}
	if body["has_agent_model"] != true {
		t.Fatalf("has_agent_model %v want true", body["has_agent_model"])
	}
}

func TestOnboardingStatusAgentModelNamesUnknownProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	body := onboardingStatusFor(t, `
providers:
  - name: openai
    type: openai
    api_key: "sk-test"

agent:
  model: "ghost/gpt-4o"
`)
	if body["has_agent_credentials"] != false {
		t.Fatalf("has_agent_credentials %v want false for a provider that is not configured", body["has_agent_credentials"])
	}
}

// api_key_command counts without being executed: a command that would fail
// loudly (and slowly) must never run on a status read.
func TestProviderHasCredentialsDoesNotRunKeyCommand(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("NEURALDEEP_API_KEY", "")
	home := t.TempDir()
	cases := []struct {
		name string
		p    config.ProviderConfig
		want bool
	}{
		{"literal key", config.ProviderConfig{Name: "openai", Type: "openai", APIKey: "sk-x"}, true},
		{"tilde placeholder", config.ProviderConfig{Name: "openai", Type: "openai", APIKey: "~"}, true},
		{"unexpanded reference", config.ProviderConfig{Name: "openai", Type: "openai", APIKey: "${MISSING}"}, true},
		{"key command", config.ProviderConfig{Name: "openai", Type: "openai", APIKeyCommand: "exit 1"}, true},
		{"nothing", config.ProviderConfig{Name: "openai", Type: "openai"}, false},
		{"neuraldeep without login", config.ProviderConfig{Name: "neuraldeep", Type: "neuraldeep"}, false},
	}
	for _, tc := range cases {
		if got := providerHasCredentials(home, tc.p); got != tc.want {
			t.Errorf("%s: providerHasCredentials = %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestProviderHasCredentialsStoredNeuralDeepLogin(t *testing.T) {
	t.Setenv("NEURALDEEP_API_KEY", "")
	home := t.TempDir()
	path := config.NeuralDeepAuthPath(home, "neuraldeep")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"api_key":"nd-hub-key","hub":"https://hub.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !providerHasCredentials(home, config.ProviderConfig{Name: "neuraldeep", Type: "neuraldeep"}) {
		t.Fatal("a stored hub login must count as a credential")
	}
}
