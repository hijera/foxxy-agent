//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestVSCodeDiagramDownloadFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "vscode-diagram-download",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a reader downloads a diagram in the VS Code panel$`, func() error {
				return runVitestScenario("src/ui/markdown/diagramDownload.test.ts",
					"routes diagram downloads through the VS Code host without creating a blob")
			})
			sc.Step(`^the source reaches the editor host unchanged$`, func() {})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/vscode_diagram_download.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("VS Code diagram download feature failed")
	}
}
