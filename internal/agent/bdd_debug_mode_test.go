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

type debugFeatureProvider struct {
	messages []llm.Message
	tools    []llm.ToolDefinition
}

func (p *debugFeatureProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Debug feature must use streaming")
}

func (p *debugFeatureProvider) Stream(
	_ context.Context,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	onChunk func(llm.StreamChunk),
) (*llm.Response, error) {
	p.messages = append([]llm.Message(nil), messages...)
	p.tools = append([]llm.ToolDefinition(nil), tools...)
	onChunk(llm.StreamChunk{TextDelta: "Confirmed diagnosis: off-by-one in the loop bound."})
	return &llm.Response{
		Content:    "Confirmed diagnosis: off-by-one in the loop bound.",
		StopReason: "end_turn",
	}, nil
}

type debugFeatureState struct {
	state    *session.State
	agent    *Agent
	provider *debugFeatureProvider
}

func (s *debugFeatureState) reset() error {
	s.provider = &debugFeatureProvider{}
	s.state = &session.State{
		ID:   "sess_debug_feature",
		CWD:  ".",
		Mode: session.ModeDebug,
	}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "openai", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "openai/gpt-5", MaxTokens: 256}},
		Agent:     config.Agent{Model: "openai/gpt-5"},
	}
	s.agent = NewAgent(cfg, s.state, resumePermissionSender{}, nil)
	s.agent.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return s.provider, nil
	}
	return nil
}

func (s *debugFeatureState) reportFailingTest() error {
	_, err := s.agent.Run(context.Background(), []acp.ContentBlock{{
		Type: "text",
		Text: "The pagination test fails on the last page, help me debug it.",
	}})
	return err
}

func (s *debugFeatureState) promptEnforcesMethodology() error {
	if len(s.provider.messages) == 0 || s.provider.messages[0].Role != llm.RoleSystem {
		return fmt.Errorf("provider received no system prompt")
	}
	prompt := s.provider.messages[0].Content
	for _, want := range []string{"Mode: Debug", "5-7", "confirm the diagnosis", "verify the fix"} {
		if !strings.Contains(prompt, want) {
			return fmt.Errorf("Debug prompt missing %q", want)
		}
	}
	return nil
}

func (s *debugFeatureState) modelReceivesFullTools() error {
	got := make(map[string]bool, len(s.provider.tools))
	for _, tool := range s.provider.tools {
		got[tool.Name] = true
	}
	// Debug has full access: research tools AND mutation tools, unlike Ask.
	for _, want := range []string{"read", "grep", "run_command"} {
		if !got[want] {
			return fmt.Errorf("Debug toolset missing research tool %q: %+v", want, got)
		}
	}
	for _, want := range []string{"write", "edit", "apply_patch"} {
		if !got[want] {
			return fmt.Errorf("Debug toolset missing mutation tool %q (should be full access): %+v", want, got)
		}
	}
	return nil
}

func (s *debugFeatureState) answerSaved() error {
	messages := s.state.GetMessages()
	if len(messages) < 2 {
		return fmt.Errorf("Debug transcript has %d messages", len(messages))
	}
	last := messages[len(messages)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, "off-by-one") {
		return fmt.Errorf("unexpected final transcript message: %+v", last)
	}
	return nil
}

func initializeDebugScenario(sc *godog.ScenarioContext) {
	s := &debugFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.Step(`^a Debug-mode session with a responding model$`, func() error { return nil })
	sc.Step(`^the user reports a failing test$`, s.reportFailingTest)
	sc.Step(`^the Debug prompt enforces the diagnosis methodology$`, s.promptEnforcesMethodology)
	sc.Step(`^the model receives full tool access including file mutation tools$`, s.modelReceivesFullTools)
	sc.Step(`^the Debug answer is saved in the transcript$`, s.answerSaved)
}

func TestDebugModeFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "debug-mode",
		ScenarioInitializer: initializeDebugScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/debug_mode.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("Debug mode feature suite failed")
	}
}
