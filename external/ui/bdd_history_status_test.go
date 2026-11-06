//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestHistoryStatusVisibilityFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "history_status_visibility",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^the project-only choice survives a fresh localStorage origin$`, func() error {
				return runVitestScenario("src/ui/sessions/sessionsProjectFilter.test.ts", "project scope survives an IDE port change with a fresh localStorage origin")
			})
			sc.Step(`^collapsed groups survive a new IDE port without mixing environments$`, func() error {
				return runVitestScenario("src/ui/sessions/collapsedSessionGroups.test.ts", "collapsed groups survive a new IDE port without mixing remote environments")
			})
			sc.Step(`^an empty archived History names its selected status and offers active chats$`, func() error {
				return runVitestScenario("src/ui/sessions/SessionsSidebar.test.tsx",
					"an empty archived history exposes its saved status and returns to active chats")
			})
		},
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/history_status_visibility.feature"},
			TestingT: t, Strict: true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("history status visibility feature failed")
	}
}
