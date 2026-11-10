//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

// features/swarm_web_ui.feature: the swarm screen's layout rules, checked by
// Vitest against the stylesheet the binary serves.
func TestSwarmWebUIFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "swarm_web_ui",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^the swarm dock sits above the shell's backdrop and below the top bar on a narrow shell$`, func() error {
				return runVitestScenario("src/ui/swarm/SwarmView.test.tsx",
					"SwarmView takes taps on a phone: the dock sits above the backdrop and below the top bar")
			})
			sc.Step(`^the History origin filter applies to the node currently open through the relay without leaving it$`, func() error {
				return runVitestScenario("src/ui/App.swarmNode.test.tsx",
					"History origin filters the active swarm node without switching environments")
			})
			sc.Step(`^the swarm canvas starts as a tree and remembers the graph layout in this browser$`, func() error {
				return runVitestScenario("src/ui/swarm/SwarmView.test.tsx",
					"SwarmView starts as a tree and remembers the graph layout in this browser")
			})
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/swarm_web_ui.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("swarm web UI feature failed")
	}
}
