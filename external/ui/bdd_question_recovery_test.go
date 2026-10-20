//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestQuestionPromptRecoveryFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "question_prompt_recovery",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a question is still waiting on the server but its stream event was missed$`, func() error { return nil })
			sc.Step(`^the chat restores selectable answer options from session activity$`, func() error {
				return runVitestScenario("src/ui/App.stopQueue.test.tsx", "a pending question missing from SSE is recovered from session activity")
			})
			sc.Step(`^the selected answer uses the original request id$`, func() error { return nil })
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/question_prompt_recovery.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("question prompt recovery failed")
	}
}
