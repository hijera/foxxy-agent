package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestPromptCacheDeepSeekPromptFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name: "deepseek-prompt",
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/deepseek_prompt.feature"},
			TestingT: t, Strict: true,
		},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			var cfg *config.Config
			var prompt string
			sc.Step(`^a DeepSeek model served by "([^"]+)"$`, func(provider string) {
				const model = "local/deepseek-chat"
				cfg = &config.Config{
					Providers: []config.ProviderConfig{{Name: "local", Type: provider, APIKey: "test"}},
					Models:    []config.ModelEntry{{Model: model}},
					Agent:     config.Agent{Model: model},
				}
				cfg.Agent.ApplyDefaults()
				cfg.Prompts.ApplyDefaults()
			})
			sc.Step(`^the custom DeepSeek prompt directory has "([^"]+)" with "([^"]+)"$`, func(name, body string) error {
				if cfg.Prompts.Dir == "" {
					cfg.Prompts.Dir = t.TempDir()
				}
				return os.WriteFile(filepath.Join(cfg.Prompts.Dir, name), []byte(body), 0o644)
			})
			sc.Step(`^the "([^"]+)" system prompt is built for DeepSeek$`, func(mode string) {
				st := &session.State{ID: "deepseek", CWD: t.TempDir(), Mode: session.Mode(mode)}
				prompt = NewAgent(cfg, st, nil, nil).buildSystemPrompt(mode, nil, nil)
			})
			sc.Step(`^the DeepSeek system prompt includes "([^"]+)"$`, func(marker string) error {
				if !strings.Contains(prompt, marker) {
					return fmt.Errorf("prompt is missing %q", marker)
				}
				return nil
			})
		},
	}
	if suite.Run() != 0 {
		t.Fatal("DeepSeek prompt feature failed")
	}
}
