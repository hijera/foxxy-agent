package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const (
	bddIdeaModuleToken = "INTELLIJ_BDD_MODULE_TOKEN"
	bddIdeaPluginToken = "INTELLIJ_BDD_PLUGIN_TOKEN"
)

type bddIdeaContextProvider struct {
	seen []llm.Message
}

func (p *bddIdeaContextProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Complete must not be used by the IntelliJ context suite")
}

func (p *bddIdeaContextProvider) Stream(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.seen = append([]llm.Message(nil), messages...)
	const answer = "Project metadata inspected."
	onChunk(llm.StreamChunk{TextDelta: answer})
	return &llm.Response{Content: answer, StopReason: "end_turn"}, nil
}

type intellijContextFeatureState struct {
	tmpDirs  []string
	cwd      string
	provider *bddIdeaContextProvider
	agent    *Agent
}

func (s *intellijContextFeatureState) reset() error {
	s.close()
	return nil
}

func (s *intellijContextFeatureState) close() {
	for _, dir := range s.tmpDirs {
		_ = os.RemoveAll(dir)
	}
	s.tmpDirs = nil
	s.cwd = ""
	s.provider = nil
	s.agent = nil
}

func (s *intellijContextFeatureState) tempDir() (string, error) {
	dir, err := os.MkdirTemp("", "foxxycode-bdd-intellij-context-*")
	if err != nil {
		return "", err
	}
	s.tmpDirs = append(s.tmpDirs, dir)
	return dir, nil
}

func (s *intellijContextFeatureState) projectWithIntelliJMetadata() error {
	cwd, err := s.tempDir()
	if err != nil {
		return err
	}
	s.cwd = cwd
	modulesDir := filepath.Join(cwd, ".idea", "modules")
	if err := os.MkdirAll(modulesDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cwd, ".idea", "externalDependencies.xml"), []byte(`<plugin id="`+bddIdeaPluginToken+`" />`), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(modulesDir, "backend.iml"), []byte(`<module name="`+bddIdeaModuleToken+`" />`), 0o644)
}

func (s *intellijContextFeatureState) agentSession() error {
	if s.cwd == "" {
		return fmt.Errorf("no IntelliJ project prepared")
	}
	sessionDir, err := s.tempDir()
	if err != nil {
		return err
	}
	state := &session.State{
		ID:         "sess_bdd_intellij_context",
		CWD:        s.cwd,
		Mode:       session.ModeAgent,
		SessionDir: sessionDir,
	}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 2},
	}
	cfg.Prompts.ApplyDefaults()
	s.provider = &bddIdeaContextProvider{}
	s.agent = NewAgent(cfg, state, resumePermissionSender{}, nil)
	s.agent.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return s.provider, nil }
	return nil
}

func (s *intellijContextFeatureState) userAsksAboutProjectSetup() error {
	if s.agent == nil {
		return fmt.Errorf("no agent prepared")
	}
	stop, err := s.agent.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "Which IDE modules and plugins does this project use?"}})
	if err != nil {
		return err
	}
	if stop != string(acp.StopReasonEndTurn) {
		return fmt.Errorf("stop reason = %q, want end_turn", stop)
	}
	return nil
}

func (s *intellijContextFeatureState) firstRequestContainsMetadata() error {
	if len(s.provider.seen) == 0 || s.provider.seen[0].Role != llm.RoleSystem {
		return fmt.Errorf("first request has no system message")
	}
	prompt := s.provider.seen[0].Content
	for _, token := range []string{bddIdeaModuleToken, bddIdeaPluginToken, `.idea/modules/backend.iml`, `.idea/externalDependencies.xml`} {
		if !strings.Contains(prompt, token) {
			return fmt.Errorf("system prompt is missing %q", token)
		}
	}
	return nil
}

func initializeIntelliJContextScenario(sc *godog.ScenarioContext) {
	s := &intellijContextFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a project with IntelliJ module and plugin metadata$`, s.projectWithIntelliJMetadata)
	sc.Step(`^a foxxycode agent session in that IntelliJ project$`, s.agentSession)
	sc.Step(`^the user asks about the project setup$`, s.userAsksAboutProjectSetup)
	sc.Step(`^the first model request contains the IntelliJ module and plugin metadata$`, s.firstRequestContainsMetadata)
}

func TestIntelliJProjectContextFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "intellij-project-context",
		ScenarioInitializer: initializeIntelliJContextScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/intellij_project_context.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("IntelliJ project context feature suite failed")
	}
}
