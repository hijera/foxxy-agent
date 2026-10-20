//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestWorkspaceToolCardsFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "workspace_tool_cards",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^workspace tool cards show documents, checklist operations, file results and SSH destinations$`, func() error {
				return runVitestScenario("src/ui/messages/WorkspaceToolCard.test.tsx",
					"workspace cards show documentation, checklist operations, file results and SSH destinations")
			})
		},
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/workspace_tool_cards.feature"},
			TestingT: t, Strict: true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("workspace tool cards feature failed")
	}
}
