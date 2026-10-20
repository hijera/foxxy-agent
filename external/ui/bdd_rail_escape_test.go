//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

// Escape and the screens of the rail are behaviour of the rendered app, so
// each step runs the Vitest test that renders it against a stubbed server.
func TestWebUIRailEscapeFeature(t *testing.T) {
	const app = "src/ui/App.railEscape.test.tsx"
	steps := []struct{ step, file, name string }{
		{`^Escape closes the documentation reader opened from the rail$`, app,
			"Escape closes the documentation reader opened from the rail"},
		{`^Escape closes Settings opened from the rail$`, app,
			"Escape closes Settings opened from the rail"},
		{`^every screen of the rail closes on Escape and gives the address back to the chat$`, app,
			"every screen of the rail closes on Escape and gives the address back to the chat"},
		{`^a screen added to the rail closes by the same rule$`,
			"src/ui/nav/railEscape.test.tsx",
			"useRailScreenEscape closes a screen added to the table like any other"},
		{`^the scheduler leaves an open job for its list first$`, app,
			"Escape still closes the scheduler, its job editor first"},
		{`^Settings leaves an open row for its list first$`, app,
			"in Settings Escape leaves an open row for its list first, then Settings"},
		{`^on the stacked shell Settings goes back to its tiles first$`, app,
			"on the stacked shell Escape takes Settings back to its tiles first, then closes it"},
		{`^a search with text in it is cleared before the reader closes$`, app,
			"an Escape the documentation search takes clears it and leaves the reader open"},
		{`^a row menu of History folds before History closes$`, app,
			"an Escape that folds a row menu of History leaves History open"},
	}
	suite := godog.TestSuite{
		Name: "web_ui_rail_escape",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, s := range steps {
				file, name := s.file, s.name
				sc.Step(s.step, func() error { return runVitestScenario(file, name) })
			}
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/web_ui_rail_escape.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("web UI rail Escape feature failed")
	}
}
