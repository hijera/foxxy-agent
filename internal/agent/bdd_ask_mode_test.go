package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type askFeatureProvider struct {
	messages []llm.Message
	tools    []llm.ToolDefinition
}

func (p *askFeatureProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Ask feature must use streaming")
}

func (p *askFeatureProvider) Stream(
	_ context.Context,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	onChunk func(llm.StreamChunk),
) (*llm.Response, error) {
	p.messages = append([]llm.Message(nil), messages...)
	p.tools = append([]llm.ToolDefinition(nil), tools...)
	onChunk(llm.StreamChunk{TextDelta: "The repository uses a layered Go architecture."})
	return &llm.Response{
		Content:    "The repository uses a layered Go architecture.",
		StopReason: "end_turn",
	}, nil
}

type askFeatureState struct {
	state    *session.State
	agent    *Agent
	provider *askFeatureProvider
}

func (s *askFeatureState) reset() error {
	s.provider = &askFeatureProvider{}
	s.state = &session.State{
		ID:   "sess_ask_feature",
		CWD:  ".",
		Mode: session.ModeAsk,
	}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "openai", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "openai/gpt-5", MaxTokens: 256}},
		Agent:     config.Agent{Model: "openai/gpt-5"},
	}
	disableTitlePass(cfg)
	s.agent = NewAgent(cfg, s.state, resumePermissionSender{}, nil)
	s.agent.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return s.provider, nil
	}
	return nil
}

func (s *askFeatureState) askRepositoryQuestion() error {
	_, err := s.agent.Run(context.Background(), []acp.ContentBlock{{
		Type: "text",
		Text: "How does this repository work?",
	}})
	return err
}

func (s *askFeatureState) promptEnforcesReadOnly() error {
	if len(s.provider.messages) == 0 || s.provider.messages[0].Role != llm.RoleSystem {
		return fmt.Errorf("provider received no system prompt")
	}
	prompt := s.provider.messages[0].Content
	for _, want := range []string{"Mode: Ask", "read-only", "never modify"} {
		if !strings.Contains(prompt, want) {
			return fmt.Errorf("Ask prompt missing %q", want)
		}
	}
	return nil
}

func (s *askFeatureState) modelReceivesResearchTools() error {
	got := make(map[string]bool, len(s.provider.tools))
	for _, tool := range s.provider.tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"read", "glob", "grep", "print_tree", "run_command", "websearch", "webfetch"} {
		if !got[want] {
			return fmt.Errorf("Ask toolset missing %q: %+v", want, got)
		}
	}
	return nil
}

func (s *askFeatureState) modelReceivesNoMutationTools() error {
	for _, tool := range s.provider.tools {
		switch tool.Name {
		case "write", "edit", "apply_patch", "mkdir", "rm", "docs_write", "docs_edit", "plan_write":
			return fmt.Errorf("Ask exposed mutating tool %q", tool.Name)
		}
	}
	return nil
}

func (s *askFeatureState) answerSaved() error {
	messages := s.state.GetMessages()
	if len(messages) < 2 {
		return fmt.Errorf("Ask transcript has %d messages", len(messages))
	}
	last := messages[len(messages)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, "layered Go architecture") {
		return fmt.Errorf("unexpected final transcript message: %+v", last)
	}
	return nil
}

func initializeAskScenario(sc *godog.ScenarioContext) {
	s := &askFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.Step(`^an Ask-mode session with a responding model$`, func() error { return nil })
	sc.Step(`^the user asks how the repository works$`, s.askRepositoryQuestion)
	sc.Step(`^the Ask prompt enforces the read-only boundary$`, s.promptEnforcesReadOnly)
	sc.Step(`^the model receives repository, shell, and web research tools$`, s.modelReceivesResearchTools)
	sc.Step(`^the model receives no file mutation tools$`, s.modelReceivesNoMutationTools)
	sc.Step(`^the Ask answer is saved in the transcript$`, s.answerSaved)
}

func TestAskModeFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "ask-mode",
		ScenarioInitializer: initializeAskScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/ask_mode.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("Ask mode feature suite failed")
	}
}
