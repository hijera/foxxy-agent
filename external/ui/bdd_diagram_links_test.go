//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestMermaidLinkedNodesFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "mermaid-linked-nodes",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a linked Mermaid node renders with a valid xlink namespace$`, func() error {
				return runVitestScenario("src/ui/markdown/DiagramsAndMath.test.tsx",
					"standaloneSvg keeps Mermaid linked nodes when its SVG omits the xlink namespace")
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/mermaid_linked_nodes.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("linked Mermaid nodes feature failed")
	}
}
