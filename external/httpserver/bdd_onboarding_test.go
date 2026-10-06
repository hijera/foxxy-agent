//go:build http

package httpserver

import (
	"context"
	"encoding/json"
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

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// onboardingBDDState drives features/onboarding_status.feature: a real
// config.yaml in a temp home (PUT /foxxycode/config writes and reloads it), the
// Settings save flow over REST, and the onboarding probe the SPA gates the
// provider picker on.
type onboardingBDDState struct {
	home   string
	server *Server
	ts     *httptest.Server

	// agentOnOpenAI makes agent.model name the keyless openai provider instead
	// of the keyed neuraldeep one.
	agentOnOpenAI bool

	prevOpenAIKey     string
	prevNeuralDeepKey string
	lastStatus        map[string]any

	lastAuthStatusCode int
	lastAuthStatus     map[string]any
}

const onboardingBDDNeuralDeepModel = "neuraldeep/qwen3.6-35b-a3b"

func (s *onboardingBDDState) reset() error {
	s.home, _ = os.MkdirTemp("", "foxxycode-onboarding-bdd-*")
	s.agentOnOpenAI = false
	s.lastStatus = nil
	// The host machine's own keys must not leak into the credential probe.
	s.prevOpenAIKey = os.Getenv("OPENAI_API_KEY")
	s.prevNeuralDeepKey = os.Getenv("NEURALDEEP_API_KEY")
	if err := os.Setenv("OPENAI_API_KEY", ""); err != nil {
		return err
	}
	return os.Setenv("NEURALDEEP_API_KEY", "")
}

func (s *onboardingBDDState) close() {
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	_ = os.Setenv("OPENAI_API_KEY", s.prevOpenAIKey)
	_ = os.Setenv("NEURALDEEP_API_KEY", s.prevNeuralDeepKey)
	if s.home != "" {
		_ = os.RemoveAll(s.home)
	}
}

func (s *onboardingBDDState) openAIKeyInEnv() error {
	return os.Setenv("OPENAI_API_KEY", "sk-from-env")
}

func (s *onboardingBDDState) agentWillPointAtOpenAI() error {
	s.agentOnOpenAI = true
	return nil
}

func (s *onboardingBDDState) startServer() error {
	agentModel := onboardingBDDNeuralDeepModel
	if s.agentOnOpenAI {
		agentModel = "openai/gpt-4o"
	}
	yml := fmt.Sprintf(`
providers:
  - name: neuraldeep
    type: neuraldeep
    api_key: "nd-bdd-key"
  - name: openai
    type: openai

models:
  - model: %q
    max_tokens: 4096
  - model: "openai/gpt-4o"
    max_tokens: 4096

agent:
  model: %q
`, onboardingBDDNeuralDeepModel, agentModel)
	cfgPath := filepath.Join(s.home, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o600); err != nil {
		return err
	}
	cfg, err := config.LoadFromCLI(config.CLIPaths{Home: s.home, CWD: s.home})
	if err != nil {
		return err
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	sessionsDir := filepath.Join(s.home, "sessions")
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), sessionsDir, nil)
	s.server = New(cfg, mgr, slog.Default(), s.home)
	s.ts = httptest.NewServer(s.server.Handler())
	return nil
}

// retypeOpenAIToNeuralDeep does what Settings does on Save: GET the config,
// flip the type of the "openai" row (its name, empty api_key and api_base stay
// as the form leaves them), validate, then PUT.
func (s *onboardingBDDState) retypeOpenAIToNeuralDeep() error {
	res, err := http.Get(s.ts.URL + "/foxxycode/config")
	if err != nil {
		return err
	}
	var doc map[string]any
	err = json.NewDecoder(res.Body).Decode(&doc)
	_ = res.Body.Close()
	if err != nil {
		return err
	}
	provs, _ := doc["providers"].([]any)
	var row map[string]any
	for _, p := range provs {
		m, _ := p.(map[string]any)
		if m != nil && m["name"] == "openai" {
			row = m
		}
	}
	if row == nil {
		return fmt.Errorf("providers = %+v, want an openai row", doc["providers"])
	}
	if row["type"] != "openai" {
		return fmt.Errorf("openai row type = %v before the change, want openai", row["type"])
	}
	row["type"] = "neuraldeep"
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	val, err := http.Post(s.ts.URL+"/foxxycode/config/validate", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	var verdict struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	err = json.NewDecoder(val.Body).Decode(&verdict)
	_ = val.Body.Close()
	if err != nil {
		return err
	}
	if !verdict.OK {
		return fmt.Errorf("validate rejected the retyped provider: %s", verdict.Error)
	}

	req, err := http.NewRequest(http.MethodPut, s.ts.URL+"/foxxycode/config", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	put, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	snippet, _ := io.ReadAll(put.Body)
	_ = put.Body.Close()
	if put.StatusCode != http.StatusOK {
		return fmt.Errorf("save status %d: %s", put.StatusCode, strings.TrimSpace(string(snippet)))
	}
	err = json.Unmarshal(snippet, &verdict)
	if err != nil {
		return err
	}
	if !verdict.OK {
		return fmt.Errorf("save answered ok=false: %s", verdict.Error)
	}
	return nil
}

// askSignInStatusBeforeSave is what NeuralDeepAuthField does the moment the
// type combobox flips to neuraldeep: the saved row still says openai.
func (s *onboardingBDDState) askSignInStatusBeforeSave() error {
	res, err := http.Get(s.ts.URL + "/foxxycode/providers/openai/neuraldeep-auth")
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	s.lastAuthStatusCode = res.StatusCode
	s.lastAuthStatus = map[string]any{}
	if err := json.Unmarshal(body, &s.lastAuthStatus); err != nil {
		return fmt.Errorf("sign-in status body %q: %w", strings.TrimSpace(string(body)), err)
	}
	return nil
}

func (s *onboardingBDDState) signInStatusDisconnectedNotConflict() error {
	if s.lastAuthStatusCode == http.StatusConflict {
		return fmt.Errorf("sign-in status answered 409 %v: the retyped row must be served before Save", s.lastAuthStatus["error"])
	}
	if s.lastAuthStatusCode != http.StatusOK {
		return fmt.Errorf("sign-in status answered %d %v, want 200", s.lastAuthStatusCode, s.lastAuthStatus)
	}
	if s.lastAuthStatus["connected"] != false || s.lastAuthStatus["source"] != "none" {
		return fmt.Errorf("sign-in status = %v, want disconnected with source none", s.lastAuthStatus)
	}
	return nil
}

func (s *onboardingBDDState) savedConfigHoldsOpenAIAsNeuralDeep() error {
	// On disk, so a restarted server sees the new type.
	cfg, err := config.LoadFromCLI(config.CLIPaths{Home: s.home, CWD: s.home})
	if err != nil {
		return err
	}
	prov := cfg.FindProvider("openai")
	if prov == nil || prov.Type != "neuraldeep" {
		return fmt.Errorf("config.yaml openai provider = %+v, want type neuraldeep", prov)
	}
	if strings.TrimSpace(prov.APIKey) != "" {
		return fmt.Errorf("config.yaml openai provider gained an api_key %q, want none", prov.APIKey)
	}
	// And in the running server, which is what the status probe reads.
	live := s.server.activeCfg().FindProvider("openai")
	if live == nil || live.Type != "neuraldeep" {
		return fmt.Errorf("live openai provider = %+v, want type neuraldeep", live)
	}
	return nil
}

func (s *onboardingBDDState) fetchStatus() (map[string]any, error) {
	res, err := http.Get(s.ts.URL + "/foxxycode/onboarding/status")
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("onboarding status answered %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	s.lastStatus = body
	return body, nil
}

// pickerWouldOpen mirrors shouldShowOnboarding in the SPA
// (external/ui/src/ui/onboarding/onboardingStatus.ts).
func pickerWouldOpen(st map[string]any) bool {
	if st["first_run"] == true {
		return true
	}
	for _, k := range []string{"has_providers", "has_models", "has_agent_model", "has_agent_credentials"} {
		if st[k] != true {
			return true
		}
	}
	return false
}

func (s *onboardingBDDState) statusKeepsPickerClosed() error {
	st, err := s.fetchStatus()
	if err != nil {
		return err
	}
	if pickerWouldOpen(st) {
		return fmt.Errorf("onboarding status would open the picker: %+v", st)
	}
	return nil
}

func (s *onboardingBDDState) statusAsksForPicker() error {
	st, err := s.fetchStatus()
	if err != nil {
		return err
	}
	if st["has_agent_credentials"] != false {
		return fmt.Errorf("has_agent_credentials = %v, want false: %+v", st["has_agent_credentials"], st)
	}
	if !pickerWouldOpen(st) {
		return fmt.Errorf("onboarding status would keep the picker closed: %+v", st)
	}
	return nil
}

func (s *onboardingBDDState) missingKeys() ([]string, error) {
	st := s.lastStatus
	if st == nil {
		var err error
		if st, err = s.fetchStatus(); err != nil {
			return nil, err
		}
	}
	raw, _ := st["missing_api_keys"].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		name, _ := v.(string)
		out = append(out, name)
	}
	return out, nil
}

func (s *onboardingBDDState) statusListsProviderWithoutKey(name string) error {
	got, err := s.missingKeys()
	if err != nil {
		return err
	}
	for _, n := range got {
		if n == name {
			return nil
		}
	}
	return fmt.Errorf("missing_api_keys = %v, want it to list %q", got, name)
}

func (s *onboardingBDDState) statusListsNoProviderWithoutKey() error {
	got, err := s.missingKeys()
	if err != nil {
		return err
	}
	if len(got) != 0 {
		return fmt.Errorf("missing_api_keys = %v, want none", got)
	}
	return nil
}

func initializeOnboardingScenario(sc *godog.ScenarioContext) {
	s := &onboardingBDDState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^OPENAI_API_KEY is set in the environment$`, s.openAIKeyInEnv)
	sc.Step(`^the agent model is going to point at the openai provider$`, s.agentWillPointAtOpenAI)
	sc.Step(`^a foxxycode HTTP server whose config has a keyed neuraldeep agent provider and a keyless openai provider$`, s.startServer)
	sc.Step(`^I retype the openai provider to neuraldeep through the settings REST flow$`, s.retypeOpenAIToNeuralDeep)
	sc.Step(`^the save succeeds and the saved config holds the openai provider as neuraldeep$`, s.savedConfigHoldsOpenAIAsNeuralDeep)
	sc.Step(`^the onboarding status keeps the provider picker closed$`, s.statusKeepsPickerClosed)
	sc.Step(`^the onboarding status asks for the provider picker$`, s.statusAsksForPicker)
	sc.Step(`^the onboarding status lists "([^"]+)" among the providers without a key$`, s.statusListsProviderWithoutKey)
	sc.Step(`^the onboarding status lists no providers without a key$`, s.statusListsNoProviderWithoutKey)
	sc.Step(`^I ask the NeuralDeep sign-in status for the openai provider before saving$`, s.askSignInStatusBeforeSave)
	sc.Step(`^the sign-in status reports the openai provider disconnected instead of a conflict$`, s.signInStatusDisconnectedNotConflict)
}

func TestOnboardingStatusE2E(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "onboarding_status",
		ScenarioInitializer: initializeOnboardingScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/onboarding_status.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("onboarding_status feature failed")
	}
}
