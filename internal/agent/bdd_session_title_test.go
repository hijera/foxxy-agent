package agent

// Godog harness for features/session_title.feature: drives real Agent turns and
// stops some of them mid-stream, so the title guarantee is checked on the path a
// user actually takes rather than by calling the generator directly.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const bddTitleText = "Connecting Postgres to a Go API"

// bddTitleProvider serves both halves of the exchange: Stream answers the turn,
// Complete answers the title pass that follows. cancelAfter, when set, is called
// once the first delta has been streamed so a scenario can stop the turn from the
// inside, exactly as pressing Stop does mid-answer.
type bddTitleProvider struct {
	mu           sync.Mutex
	titleCalls   int
	titlePrompts []string

	answer      string
	cancelAfter func()
	cancelFirst bool
}

func (p *bddTitleProvider) Complete(_ context.Context, msgs []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	p.mu.Lock()
	p.titleCalls++
	for _, m := range msgs {
		if m.Role == llm.RoleUser {
			p.titlePrompts = append(p.titlePrompts, m.Content)
		}
	}
	p.mu.Unlock()
	return &llm.Response{Content: bddTitleText, StopReason: "end_turn"}, nil
}

func (p *bddTitleProvider) Stream(ctx context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	if p.cancelFirst {
		// Stopped before a single delta: the turn carries no assistant output.
		if p.cancelAfter != nil {
			p.cancelAfter()
		}
		return nil, context.Canceled
	}
	if p.cancelAfter == nil {
		onChunk(llm.StreamChunk{TextDelta: p.answer})
		return &llm.Response{Content: p.answer, StopReason: "end_turn"}, nil
	}
	// Stream the beginning of the answer, then stop: the caller keeps whatever
	// already reached it, which is what the transcript must show.
	partial := p.answer
	onChunk(llm.StreamChunk{TextDelta: partial})
	p.cancelAfter()
	return &llm.Response{Content: partial}, context.Canceled
}

func (p *bddTitleProvider) titleCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.titleCalls
}

func (p *bddTitleProvider) lastTitlePrompt() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.titlePrompts) == 0 {
		return ""
	}
	return p.titlePrompts[len(p.titlePrompts)-1]
}

// bddTitleSender records the title broadcasts a client would receive.
type bddTitleSender struct {
	resumePermissionSender
	mu     sync.Mutex
	titles []string
}

func (s *bddTitleSender) SendSessionUpdate(_ string, update interface{}) error {
	if u, ok := update.(acp.SessionTitleUpdate); ok {
		s.mu.Lock()
		s.titles = append(s.titles, u.Title)
		s.mu.Unlock()
	}
	return nil
}

func (s *bddTitleSender) broadcasts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.titles...)
}

type titleFeatureState struct {
	pendingPrompt string

	st       *session.State
	sender   *bddTitleSender
	provider *bddTitleProvider
	agent    *Agent
	stop     string
	runErr   error
}

func (s *titleFeatureState) reset() error {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "p", Type: "openai", APIKey: "k"}},
		Models:    []config.ModelEntry{{Model: "p/m", MaxTokens: 100, MaxContextTokens: 1000}},
	}
	cfg.Agent.Model = "p/m"
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Title.ApplyDefaults()

	s.st = &session.State{ID: "sess_title_bdd", CWD: ".", Mode: session.ModeAgent}
	s.sender = &bddTitleSender{}
	s.provider = &bddTitleProvider{answer: "You can use a connection pool"}
	s.agent = NewAgent(cfg, s.st, s.sender, nil)
	s.agent.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return s.provider, nil }
	s.stop, s.runErr = "", nil
	return nil
}

func (s *titleFeatureState) freshSession() error { return nil }

func (s *titleFeatureState) freshSessionPinned(title string) error {
	s.st.SetTitlePinnedWithoutPersist(title)
	return nil
}

func (s *titleFeatureState) userAsks(text string) error {
	s.pendingPrompt = text
	return nil
}

func (s *titleFeatureState) run() {
	s.stop, s.runErr = s.agent.Run(context.Background(), []acp.ContentBlock{
		{Type: "text", Text: s.pendingPrompt},
	})
}

func (s *titleFeatureState) modelAnswersNormally() error {
	s.provider.cancelAfter = nil
	s.run()
	return s.runErr
}

func (s *titleFeatureState) userStopsMidAnswer() error {
	s.provider.cancelAfter = func() { s.st.SetUserCancelledTurn() }
	s.run()
	return nil
}

func (s *titleFeatureState) userStopsBeforeAnyOutput() error {
	s.provider.cancelFirst = true
	s.provider.cancelAfter = func() { s.st.SetUserCancelledTurn() }
	s.run()
	return nil
}

func (s *titleFeatureState) turnEndsCancelled() error {
	if s.stop != string(acp.StopReasonCancelled) {
		return fmt.Errorf("stop reason = %q, want cancelled (err %v)", s.stop, s.runErr)
	}
	return nil
}

func (s *titleFeatureState) partialAnswerKept() error {
	for _, m := range s.st.GetMessages() {
		if m.Role == llm.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return nil
		}
	}
	return fmt.Errorf("no assistant message with content in the transcript")
}

// awaitTitle waits for the detached title goroutine, which by design outlives
// the turn it was started from.
func (s *titleFeatureState) awaitTitle() string {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := s.st.GetTitleAuto(); got != "" {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	return s.st.GetTitleAuto()
}

func (s *titleFeatureState) titleGeneratedFromUserMessage() error {
	got := s.awaitTitle()
	if got != bddTitleText {
		return fmt.Errorf("generated title = %q, want %q", got, bddTitleText)
	}
	if prompt := s.provider.lastTitlePrompt(); !strings.Contains(prompt, s.pendingPrompt) {
		return fmt.Errorf("title prompt %q does not carry the user's message", prompt)
	}
	return nil
}

func (s *titleFeatureState) titleBroadcastOnce() error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(s.sender.broadcasts()) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	got := s.sender.broadcasts()
	if len(got) != 1 || got[0] != bddTitleText {
		return fmt.Errorf("title broadcasts = %v, want exactly one %q", got, bddTitleText)
	}
	if n := s.provider.titleCallCount(); n != 1 {
		return fmt.Errorf("title provider called %d times, want 1", n)
	}
	return nil
}

func (s *titleFeatureState) titleStays(want string) error {
	// Give a stray generator time to misfire before declaring the pin intact.
	time.Sleep(300 * time.Millisecond)
	if got := s.st.GetTitlePinned(); got != want {
		return fmt.Errorf("pinned title = %q, want %q", got, want)
	}
	if got := s.st.GetTitleAuto(); got != "" {
		return fmt.Errorf("auto title = %q, want none next to a pinned title", got)
	}
	if n := s.provider.titleCallCount(); n != 0 {
		return fmt.Errorf("title provider called %d times, want 0 for a pinned title", n)
	}
	return nil
}

func initializeTitleScenario(sc *godog.ScenarioContext) {
	s := &titleFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.Step(`^a fresh session$`, s.freshSession)
	sc.Step(`^a fresh session with the title pinned to "([^"]*)"$`, s.freshSessionPinned)
	sc.Step(`^the user asks "([^"]*)"$`, s.userAsks)
	sc.Step(`^the model answers normally$`, s.modelAnswersNormally)
	sc.Step(`^the user stops the turn while the model is still writing$`, s.userStopsMidAnswer)
	sc.Step(`^the user stops the turn before the model writes anything$`, s.userStopsBeforeAnyOutput)
	sc.Step(`^the turn ends as cancelled$`, s.turnEndsCancelled)
	sc.Step(`^the partial answer is kept in the transcript$`, s.partialAnswerKept)
	sc.Step(`^the session title is generated from the user's message$`, s.titleGeneratedFromUserMessage)
	sc.Step(`^the title is broadcast to clients once$`, s.titleBroadcastOnce)
	sc.Step(`^the session title stays "([^"]*)"$`, s.titleStays)
}

func TestSessionTitleFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "session-title",
		ScenarioInitializer: initializeTitleScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/session_title.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("session title feature suite failed")
	}
}
