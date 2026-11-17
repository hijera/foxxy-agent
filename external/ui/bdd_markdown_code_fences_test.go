//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestMarkdownCodeFencesFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "markdown_code_fences",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a code fence in a list item displays and copies its literal LaTeX delimiters$`, func() error {
				return runVitestScenario("src/ui/markdown/DiagramsAndMath.test.tsx",
					"formulas code fences in list items display and copy their literal LaTeX delimiters")
			})
			sc.Step(`^an inline formula copies its LaTeX source even when the editor denies the Clipboard API$`, func() error {
				return runVitestScenario("src/ui/markdown/DiagramsAndMath.test.tsx",
					"formulas copies inline LaTeX when the editor denies the Clipboard API")
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/markdown_code_fences.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("markdown code fences feature failed")
	}
}
