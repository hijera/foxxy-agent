package agent

// Godog harness for features/ide_tool_delegate.feature: drives the real
// Agent.Run with a fake LLM provider and a sender that implements
// acp.ToolExecutor, standing in for an IDE plugin that executes tool calls
// through the editor. One scenario has the client take the call (the tool must
// not touch the disk and the client's output must reach the model); the other
// has it decline (the call must run locally as before).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const (
	bddIdeCall      = "call_ide"
	bddIdeFileBody  = "written through the IDE"
	bddIdeClientOut = "applied through the IDE document"
)

// ideDelegateSender stands in for an IDE plugin: it records every tool call it
// is offered and answers handled, declined or a failure as the scenario asks.
type ideDelegateSender struct {
	resumePermissionSender
	mu       sync.Mutex
	declines bool
	failWith string
	got      []acp.ToolExecutionRequest
}

func (s *ideDelegateSender) ExecuteToolCall(_ context.Context, req acp.ToolExecutionRequest) (acp.ToolExecutionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, req)
	switch {
	case s.failWith != "":
		return acp.ToolExecutionResult{Status: acp.ToolExecHandled, Err: s.failWith}, nil
	case s.declines:
		return acp.ToolExecutionResult{Status: acp.ToolExecNotHandled}, nil
	default:
		return acp.ToolExecutionResult{Status: acp.ToolExecHandled, Output: bddIdeClientOut}, nil
	}
}

func (s *ideDelegateSender) snapshotRequests() []acp.ToolExecutionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]acp.ToolExecutionRequest(nil), s.got...)
}

type ideDelegateFeatureState struct {
	tmpDirs  []string
	st       *session.State
	ag       *Agent
	cfg      *config.Config
	provider *bddAskProvider
	sender   *ideDelegateSender
	target   string

	stop   string
	runErr error
}

func (s *ideDelegateFeatureState) reset() error {
	s.close()
	s.sender = &ideDelegateSender{}
	s.provider = nil
	s.target = ""
	s.stop = ""
	s.runErr = nil
	return nil
}

func (s *ideDelegateFeatureState) close() {
	for _, d := range s.tmpDirs {
		_ = os.RemoveAll(d)
	}
	s.tmpDirs = nil
	s.st = nil
	s.ag = nil
	s.cfg = nil
}

func (s *ideDelegateFeatureState) tempDir() (string, error) {
	d, err := os.MkdirTemp("", "foxxycode-bdd-ide-delegate-*")
	if err != nil {
		return "", err
	}
	s.tmpDirs = append(s.tmpDirs, d)
	return d, nil
}

func (s *ideDelegateFeatureState) sessionInMode(mode string) error {
	if !session.IsValidMode(mode) {
		return fmt.Errorf("unknown session mode %q", mode)
	}
	cwd, err := s.tempDir()
	if err != nil {
		return err
	}
	sessionDir, err := s.tempDir()
	if err != nil {
		return err
	}
	s.target = filepath.Join(cwd, "notes.md")
	s.st = &session.State{
		ID:         "sess_bdd_ide_delegate",
		CWD:        cwd,
		Mode:       session.Mode(mode),
		SessionDir: sessionDir,
	}
	s.cfg = &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 6},
	}
	return nil
}

func (s *ideDelegateFeatureState) buildAgent() error {
	if s.st == nil {
		return fmt.Errorf("no session prepared")
	}
	s.ag = NewAgent(s.cfg, s.st, s.sender, nil)
	s.ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return s.provider, nil
	}
	return nil
}

func (s *ideDelegateFeatureState) clientHandles(_ string) error {
	s.sender.declines = false
	return nil
}

func (s *ideDelegateFeatureState) clientDeclines(_ string) error {
	s.sender.declines = true
	return nil
}

func (s *ideDelegateFeatureState) modelRequestsToolOnceThenAnswers(tool string) error {
	if tool != "write" {
		return fmt.Errorf("the suite only simulates the write tool, got %q", tool)
	}
	args, err := json.Marshal(map[string]string{"path": s.target, "content": bddIdeFileBody})
	if err != nil {
		return err
	}
	s.provider = &bddAskProvider{
		toolCall: &llm.ToolCall{ID: bddIdeCall, Name: tool, InputJSON: string(args)},
	}
	return s.buildAgent()
}

func (s *ideDelegateFeatureState) userAsksForAChange() error {
	if s.ag == nil {
		return fmt.Errorf("no agent prepared")
	}
	s.stop, s.runErr = s.ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "update the notes"}})
	return nil
}

func (s *ideDelegateFeatureState) clientReceivedCall(tool string) error {
	for _, req := range s.sender.snapshotRequests() {
		if req.ToolName == tool && req.ToolCallID == bddIdeCall && strings.Contains(req.ArgsJSON, bddIdeFileBody) {
			return nil
		}
	}
	return fmt.Errorf("the IDE client was never offered the %s call: %+v", tool, s.sender.snapshotRequests())
}

func (s *ideDelegateFeatureState) fileIsNotWritten() error {
	if _, err := os.Stat(s.target); err == nil {
		return fmt.Errorf("%s exists: the delegated write still ran locally", s.target)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *ideDelegateFeatureState) fileIsWritten() error {
	body, err := os.ReadFile(s.target)
	if err != nil {
		return fmt.Errorf("the write did not happen: %v", err)
	}
	if string(body) != bddIdeFileBody {
		return fmt.Errorf("unexpected file body %q", body)
	}
	return nil
}

func (s *ideDelegateFeatureState) toolResultCarriesClientOutput() error {
	for _, m := range s.st.GetMessages() {
		if m.Role == llm.RoleTool && m.ToolCallID == bddIdeCall {
			if strings.Contains(m.Content, bddIdeClientOut) {
				return nil
			}
			return fmt.Errorf("tool result does not carry the client output: %q", m.Content)
		}
	}
	return fmt.Errorf("no tool result recorded for %s", bddIdeCall)
}

func (s *ideDelegateFeatureState) turnEndsWithAnswer() error {
	if s.runErr != nil {
		return fmt.Errorf("turn failed: %v", s.runErr)
	}
	if s.stop != string(acp.StopReasonEndTurn) {
		return fmt.Errorf("stop reason = %q, want end_turn", s.stop)
	}
	msgs := s.st.GetMessages()
	if len(msgs) == 0 {
		return fmt.Errorf("empty transcript")
	}
	last := msgs[len(msgs)-1]
	// The suite reuses the ask-mode provider double, which answers with
	// bddAskAnswer; the scenario only cares that the turn ends with the
	// model's own answer.
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, bddAskAnswer) {
		return fmt.Errorf("final answer missing: %+v", last)
	}
	return nil
}

func initializeIdeDelegateScenario(sc *godog.ScenarioContext) {
	s := &ideDelegateFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a foxxycode session in "([^"]+)" mode$`, s.sessionInMode)
	sc.Step(`^an IDE client that handles the "([^"]+)" tool$`, s.clientHandles)
	sc.Step(`^an IDE client that declines the "([^"]+)" tool$`, s.clientDeclines)
	sc.Step(`^a model that requests the "([^"]+)" tool once, then answers$`, s.modelRequestsToolOnceThenAnswers)
	sc.Step(`^the user asks for a change$`, s.userAsksForAChange)
	sc.Step(`^the IDE client received the "([^"]+)" call$`, s.clientReceivedCall)
	sc.Step(`^the file is not written$`, s.fileIsNotWritten)
	sc.Step(`^the file is written$`, s.fileIsWritten)
	sc.Step(`^the tool result carries the IDE client's output$`, s.toolResultCarriesClientOutput)
	sc.Step(`^the turn ends with the model's answer$`, s.turnEndsWithAnswer)
}

func TestIdeDelegateFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "ide-tool-delegate",
		ScenarioInitializer: initializeIdeDelegateScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/ide_tool_delegate.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("ide tool delegation feature suite failed")
	}
}
