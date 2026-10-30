//go:build http && ui

package ui

import (
	"testing"

	"github.com/cucumber/godog"
)

// Whether a closed reader leaves a complete or a cut stream is decided by the
// pure reader in chat/consumeComposerSse.ts, so each step runs the Vitest test
// that feeds it a reader of crafted chunks.
func TestComposerStreamTailWebUIFeature(t *testing.T) {
	const feed = "src/ui/chat/consumeComposerSse.test.ts"
	suite := godog.TestSuite{
		Name: "composer_stream_tail_web_ui",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a \[DONE\] frame delivered without the closing blank line ends the stream as complete$`, func() error {
				return runVitestScenario(feed, "consumeComposerSseReader endedWithoutDone is false when [DONE] arrives in the unterminated tail")
			})
			sc.Step(`^a reader that closes before \[DONE\] reports the stream as cut mid-turn$`, func() error {
				return runVitestScenario(feed, "consumeComposerSseReader endedWithoutDone is true when the reader closes before [DONE] (cut mid-turn)")
			})
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/composer_stream_tail_web_ui.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("composer stream tail web UI feature failed")
	}
}
