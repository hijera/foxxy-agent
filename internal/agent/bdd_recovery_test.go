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

type recoveryProvider struct {
	calls   int
	repeat  bool
	prompts []string
}

func (p *recoveryProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "title"}, nil
}

func (p *recoveryProvider) Stream(_ context.Context, msgs []llm.Message, _ []llm.ToolDefinition, send func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	p.prompts = append(p.prompts, msgs[0].Content)
	if p.repeat {
		tc := llm.ToolCall{ID: fmt.Sprintf("read_%d", p.calls), Name: "glob", InputJSON: `{"pattern":"*"}`}
		return &llm.Response{Content: "I will start fixing this now.", ToolCalls: []llm.ToolCall{tc}}, nil
	}
	send(llm.StreamChunk{TextDelta: "continued"})
	return &llm.Response{Content: "continued"}, nil
}

type recoveryFeature struct {
	t     *testing.T
	st    *session.State
	p     *recoveryProvider
	err   error
	store *session.FileStore
}

func (s *recoveryFeature) setup(crash bool) error {
	s.store = &session.FileStore{Root: s.t.TempDir()}
	dir, err := s.store.EnsureLayout("sess_recovery")
	if err != nil {
		return err
	}
	s.st = &session.State{ID: "sess_recovery", CWD: s.t.TempDir(), SessionDir: dir, Mode: session.ModeAgent}
	s.st.SetPersistHook(func() {
		if err := s.store.Save(s.st); err != nil {
			s.t.Error(err)
		}
	})
	s.st.SetTitlePinned("Recovery test")
	s.p = &recoveryProvider{repeat: !crash}
	if crash {
		return os.WriteFile(filepath.Join(s.st.SessionDir, "execution.json"), []byte(`{"status":"running","lastTool":"read"}`), 0o600)
	}
	return nil
}

func (s *recoveryFeature) run() error {
	cfg := &config.Config{Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}}, Models: []config.ModelEntry{{Model: "fake/model"}}, Agent: config.Agent{Model: "fake/model", MaxTurns: 3}}
	ag := NewAgent(cfg, s.st, resumePermissionSender{}, nil)
	ag.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return s.p, nil })
	_, s.err = ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "continue"}})
	return nil
}

func TestAgentRecoveryFeature(t *testing.T) {
	s := &recoveryFeature{t: t}
	suite := godog.TestSuite{Name: "agent-recovery", Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/agent_recovery.feature"}, TestingT: t, Strict: true}, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^a persisted agent execution interrupted during a model call$`, func() error { return s.setup(true) })
		sc.Step(`^the agent continues the saved conversation$`, s.run)
		sc.Step(`^the model receives an interruption recovery instruction$`, func() error {
			if s.err != nil {
				return s.err
			}
			if len(s.p.prompts) == 0 || !strings.Contains(s.p.prompts[0], "Previous execution was interrupted") {
				return fmt.Errorf("missing recovery instruction")
			}
			return nil
		})
		sc.Step(`^the recovery instruction is not a user message in the transcript$`, func() error {
			for _, m := range s.st.GetMessages() {
				if strings.Contains(m.Content, "Previous execution was interrupted") {
					return fmt.Errorf("recovery instruction leaked into transcript")
				}
			}
			return nil
		})
		sc.Step(`^an agent repeatedly reading unchanged information$`, func() error { return s.setup(false) })
		sc.Step(`^the agent reaches its turn limit and starts again$`, func() error {
			_ = s.run()
			snap, err := s.store.ReadSnapshot(s.st.ID)
			if err != nil {
				return err
			}
			s.st = &session.State{ID: snap.Meta.ID, CWD: snap.Meta.CWD, SessionDir: snap.Dir, Mode: session.ModeAgent}
			s.st.ReplaceMessagesWithoutPersist(snap.Messages)
			s.st.SetTitlePinned("Recovery test")
			s.st.SetPersistHook(func() {
				if err := s.store.Save(s.st); err != nil {
					s.t.Error(err)
				}
			})
			return s.run()
		})
		sc.Step(`^the model receives a loop correction before execution stops$`, func() error {
			if s.err == nil || !strings.Contains(s.err.Error(), "no progress") {
				return fmt.Errorf("expected no-progress stop, got %v after %d calls", s.err, s.p.calls)
			}
			if !strings.Contains(strings.Join(s.p.prompts, "\n"), "Repeated actions produced no new information") {
				return fmt.Errorf("missing loop correction")
			}
			return nil
		})
	}}
	if suite.Run() != 0 {
		t.Fatal("agent recovery feature failed")
	}
}
