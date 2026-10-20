package main

// Godog harness for features/provider_diagnostics.feature: builds a real
// provider against a stand-in endpoint and reads the notice a failed call
// produces, then runs the provider list over a config with nothing to
// authenticate with.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type providerDiagState struct {
	t        *testing.T
	server   *httptest.Server
	provider llm.Provider
	endpoint string
	name     string
	err      error
	cfg      *config.Config
	lines    []string
}

func (s *providerDiagState) reset(t *testing.T) {
	s.t = t
	s.server = nil
	s.provider = nil
	s.endpoint = ""
	s.name = ""
	s.err = nil
	s.cfg = nil
	s.lines = nil
}

func (s *providerDiagState) rejectingProvider(name string) error {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided"}}`))
	}))
	s.t.Cleanup(s.server.Close)
	s.name = name
	s.endpoint = s.server.URL
	p, err := llm.NewProvider(llm.ProviderInput{
		Name:    name,
		Type:    "openai",
		Model:   "some-model",
		APIKey:  "nope",
		BaseURL: s.server.URL,
	})
	if err != nil {
		return err
	}
	s.provider = p
	return nil
}

func (s *providerDiagState) providerWithNoAPIBase(name string) error {
	s.name = name
	p, err := llm.NewProvider(llm.ProviderInput{Name: name, Type: "openai", Model: "some-model", APIKey: "k"})
	if err != nil {
		return err
	}
	s.provider = p
	return nil
}

func (s *providerDiagState) streamThroughProvider() error {
	_, s.err = s.provider.Stream(context.Background(),
		[]llm.Message{{Role: llm.RoleUser, Content: "hi"}}, nil, func(llm.StreamChunk) {})
	if s.err == nil {
		return fmt.Errorf("the rejecting endpoint produced no error")
	}
	return nil
}

func (s *providerDiagState) buildProvider() error {
	s.endpoint = llm.ProviderEndpoint("openai", "")
	return nil
}

func (s *providerDiagState) noticeNamesProvider(name string) error {
	if !strings.Contains(s.err.Error(), `provider "`+name+`"`) {
		return fmt.Errorf("notice %q does not name provider %q", s.err, name)
	}
	return nil
}

func (s *providerDiagState) noticeNamesTheAddress() error {
	if !strings.Contains(s.err.Error(), s.endpoint) {
		return fmt.Errorf("notice %q does not name the address %q", s.err, s.endpoint)
	}
	return nil
}

func (s *providerDiagState) endpointMessageSurvives() error {
	if !strings.Contains(s.err.Error(), "Incorrect API key provided") {
		return fmt.Errorf("notice %q lost what the endpoint said", s.err)
	}
	return nil
}

func (s *providerDiagState) endpointIsTheOfficialOpenAIAddress() error {
	if s.endpoint != llm.OpenAIDefaultAPIBase() {
		return fmt.Errorf("endpoint = %q, want %q", s.endpoint, llm.OpenAIDefaultAPIBase())
	}
	return nil
}

func (s *providerDiagState) configWithBareOpenAIProvider(name string, withKey bool) error {
	prov := config.ProviderConfig{Name: name, Type: "openai"}
	if withKey {
		prov.APIKey = "sk-something"
	}
	s.name = name
	s.cfg = &config.Config{Providers: []config.ProviderConfig{prov}}
	s.t.Setenv(config.ProviderAPIKeyEnvVarName(name), "")
	return nil
}

func (s *providerDiagState) runProviderList() error {
	s.lines = providersListLines(s.cfg)
	return nil
}

func (s *providerDiagState) warningLine() string {
	for _, l := range s.lines {
		if strings.HasPrefix(strings.TrimSpace(l), "!") {
			return l
		}
	}
	return ""
}

func (s *providerDiagState) listWarnsAboutTheOfficialEndpoint() error {
	if w := s.warningLine(); !strings.Contains(w, llm.OpenAIDefaultAPIBase()) {
		return fmt.Errorf("warning %q does not name the official endpoint", w)
	}
	return nil
}

func (s *providerDiagState) warningSaysNoCredential() error {
	if w := s.warningLine(); !strings.Contains(w, "no credential is configured") {
		return fmt.Errorf("warning %q does not say the provider has no credential", w)
	}
	return nil
}

func (s *providerDiagState) warningNamesProvider(name string) error {
	if w := s.warningLine(); !strings.Contains(w, name+":") {
		return fmt.Errorf("warning %q does not name provider %q", w, name)
	}
	return nil
}

func (s *providerDiagState) listCarriesNoWarning() error {
	if w := s.warningLine(); w != "" {
		return fmt.Errorf("the list warned about a provider that can authenticate: %q", w)
	}
	return nil
}

func initializeProviderDiagnosticsScenario(t *testing.T, sc *godog.ScenarioContext) {
	s := &providerDiagState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		s.reset(t)
		return ctx, nil
	})

	sc.Step(`^a provider named "([^"]*)" of type openai whose endpoint rejects every request$`, s.rejectingProvider)
	sc.Step(`^a provider named "([^"]*)" of type openai with no api_base$`, s.providerWithNoAPIBase)
	sc.Step(`^a config whose only provider is "([^"]*)" of type openai with no api_base and no key$`,
		func(name string) error { return s.configWithBareOpenAIProvider(name, false) })
	sc.Step(`^a config whose only provider is "([^"]*)" of type openai with no api_base but a key$`,
		func(name string) error { return s.configWithBareOpenAIProvider(name, true) })
	sc.Step(`^a completion is streamed through that provider$`, s.streamThroughProvider)
	sc.Step(`^the provider is built$`, s.buildProvider)
	sc.Step(`^I run the provider list$`, s.runProviderList)
	sc.Step(`^the notice names the provider "([^"]*)"$`, s.noticeNamesProvider)
	sc.Step(`^the notice names the address of that endpoint$`, s.noticeNamesTheAddress)
	sc.Step(`^the message from the endpoint survives in the notice$`, s.endpointMessageSurvives)
	sc.Step(`^the endpoint reported for it is the official OpenAI address$`, s.endpointIsTheOfficialOpenAIAddress)
	sc.Step(`^the list warns that the provider talks to the official OpenAI endpoint$`, s.listWarnsAboutTheOfficialEndpoint)
	sc.Step(`^the warning says no credential is configured$`, s.warningSaysNoCredential)
	sc.Step(`^the warning names the provider "([^"]*)"$`, s.warningNamesProvider)
	sc.Step(`^the list carries no warning$`, s.listCarriesNoWarning)
}

func TestProviderDiagnosticsFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "provider-diagnostics",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			initializeProviderDiagnosticsScenario(t, sc)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/provider_diagnostics.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("provider_diagnostics.feature failed")
	}
}
