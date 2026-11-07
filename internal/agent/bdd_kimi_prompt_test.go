package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestKimiPromptFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "kimi-prompt",
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/kimi_prompt.feature"},
			TestingT: t, Strict: true,
		},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			var cfg *config.Config
			var prompt string
			sc.Step(`^an OpenAI-compatible Kimi model "([^"]+)"$`, func(model string) {
				model = "local/" + model
				cfg = &config.Config{
					Providers: []config.ProviderConfig{{Name: "local", Type: "openai", APIKey: "test"}},
					Models:    []config.ModelEntry{{Model: model}},
					Agent:     config.Agent{Model: model},
				}
				cfg.Agent.ApplyDefaults()
				cfg.Prompts.ApplyDefaults()
			})
			sc.Step(`^the Kimi agent system prompt is built$`, func() {
				st := &session.State{ID: "kimi", CWD: t.TempDir(), Mode: session.ModeAgent}
				prompt = NewAgent(cfg, st, nil, nil).buildSystemPrompt("agent", nil, nil)
			})
			sc.Step(`^the prompt includes Kimi guidance and shared file-search guidance$`, func() error {
				for _, marker := range []string{"Model-family notes (Kimi)", "Reading and searching (context is limited)"} {
					if !strings.Contains(prompt, marker) {
						return fmt.Errorf("prompt is missing %q", marker)
					}
				}
				return nil
			})
			sc.Step(`^the model offers reasoning levels "([^"]+)"$`, func(want string) error {
				got := strings.Join(cfg.ReasoningLevelsFor(&cfg.Models[0]), ",")
				if want == "none" {
					want = ""
				}
				if got != want {
					return fmt.Errorf("reasoning levels = %q, want %q", got, want)
				}
				return nil
			})
		},
	}
	if suite.Run() != 0 {
		t.Fatal("Kimi prompt feature failed")
	}
}
