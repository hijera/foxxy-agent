//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestSharedFileMenuFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "shared_file_menu",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^file actions escape card clipping and close with Escape$`, func() error {
				return runVitestScenario("src/ui/messages/ToolArtifacts.test.tsx", "artifact actions escape the clipped card and close on Escape")
			})
			sc.Step(`^clicking file actions a second time closes the menu$`, func() error {
				return runVitestScenario("src/ui/messages/ToolArtifacts.test.tsx", "a second pointer click on artifact actions closes the menu")
			})
			sc.Step(`^file actions send an authenticated POST to reveal the source on its server$`, func() error {
				for _, mode := range []string{"local", "remote"} {
					if err := runVitestScenario("src/ui/messages/ToolArtifacts.test.tsx", "reveals an artifact with POST in the "+mode+" environment"); err != nil {
						return err
					}
				}
				return nil
			})
			sc.Step(`^file names and paths still copy when Clipboard API writes are denied$`, func() error {
				for _, label := range []string{"Copy name", "Copy relative path", "Copy absolute path"} {
					if err := runVitestScenario("src/ui/messages/ToolArtifacts.test.tsx", label+" falls back when the embedded browser denies Clipboard API writes"); err != nil {
						return err
					}
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/shared_file_menu.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("shared file menu feature failed")
	}
}
