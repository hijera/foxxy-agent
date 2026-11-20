//go:build http && ui

package ui

import (
	"github.com/cucumber/godog"
	"testing"
)

func TestNotificationStopFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "web_ui_notification_stop",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a stopped task stays silent and the next completed task notifies$`, func() error {
				return runVitestScenario("src/ui/App.notifications.test.tsx", "a completed task notifies once: 'acknowledged Stop'")
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/web_ui_notification_stop.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("notification Stop feature failed")
	}
}
