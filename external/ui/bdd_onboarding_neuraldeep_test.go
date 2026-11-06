//go:build http && ui

package ui

import (
	"regexp"
	"testing"

	"github.com/cucumber/godog"
)

func TestOnboardingNeuralDeepDomainFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "onboarding_neuraldeep_domain",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, name := range []string{
				"offers the NeuralDeep domain before credentials and uses it for models and save",
				"uses the selected NeuralDeep domain for hub sign in",
			} {
				sc.Step("^onboarding "+regexp.QuoteMeta(name)+"$", func() error {
					return runVitestScenario("src/ui/onboarding/ProviderPickerDialog.test.tsx", "ProviderPickerDialog "+name)
				})
			}
		},
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/onboarding_neuraldeep_domain.feature"},
			TestingT: t, Strict: true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("NeuralDeep onboarding domain feature failed")
	}
}
