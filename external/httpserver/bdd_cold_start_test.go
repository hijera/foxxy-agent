//go:build http

package httpserver

// Godog harness for features/session_cold_start.feature: a freshly started backend being
// asked about a session it has never loaded, by the whole fan-out of per-session routes at
// once, followed by the first prompt.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// How long the first prompt is given to reach the model. Generous: the point is that it
// arrives at all, not how fast.
const coldStartPromptWait = 15 * time.Second

type coldStartState struct {
	root      string
	ts        *httptest.Server
	srv       *Server
	sessionID string
	loads     *countingSender

	statuses   []int
	reachedLLM chan string
}

func (s *coldStartState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-coldstart-*")
	if err != nil {
		return err
	}
	*s = coldStartState{root: root, reachedLLM: make(chan string, 1)}
	return nil
}

func (s *coldStartState) close() {
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.srv != nil {
		s.srv.Drain()
		s.srv = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

func (s *coldStartState) startServer() error {
	home := filepath.Join(s.root, "home")
	sessRoot := filepath.Join(s.root, "sessions")
	for _, d := range []string{home, sessRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	runner := func(_ context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		var sb strings.Builder
		for _, b := range prompt {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		text := strings.TrimSpace(sb.String())
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: text})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "ok"})
		select {
		case s.reachedLLM <- text:
		default:
		}
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: s.root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	s.loads = &countingSender{}
	mgr := session.NewManager(cfg, s.loads, runner, slog.Default(), s.root, &session.FileStore{Root: sessRoot})
	s.srv = New(cfg, mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(s.srv.Handler())

	sn, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.root})
	if err != nil {
		return err
	}
	s.sessionID = sn.SessionID
	return nil
}

// chatLeftOnDisk puts the backend in the state it is in right after a restart: the bundle
// exists, nothing is in memory.
func (s *coldStartState) chatLeftOnDisk() error {
	s.srv.mgr.ForgetLiveSession(s.sessionID)
	s.loads.loads.Store(0)
	return nil
}

func (s *coldStartState) panelOpensEveryView() error {
	paths := []string{"messages", "tool-calls", "stats", "branches", "activity", "plans"}
	s.statuses = make([]int, len(paths))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			<-start
			res, err := s.ts.Client().Get(fmt.Sprintf("%s/foxxycode/sessions/%s/%s", s.ts.URL, s.sessionID, p))
			if err != nil {
				s.statuses[i] = -1
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			s.statuses[i] = res.StatusCode
		}(i, p)
	}
	close(start)
	wg.Wait()
	return nil
}

func (s *coldStartState) everyViewAnswers() error {
	for i, code := range s.statuses {
		if code != http.StatusOK {
			return fmt.Errorf("view %d answered %d", i, code)
		}
	}
	return nil
}

func (s *coldStartState) readFromDiskOnce() error {
	if got := s.loads.loads.Load(); got != 1 {
		return fmt.Errorf("the chat was read from disk %d times, want 1", got)
	}
	return nil
}

func (s *coldStartState) sendFirstPrompt() error {
	body := `{"model":"agent","input":"hello","stream":true}`
	req, err := http.NewRequest(http.MethodPost, s.ts.URL+"/v1/responses", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the prompt was answered with %d", res.StatusCode)
	}
	return nil
}

func (s *coldStartState) promptReachedTheModel() error {
	select {
	case got := <-s.reachedLLM:
		if got != "hello" {
			return fmt.Errorf("the model was asked %q, want %q", got, "hello")
		}
		return nil
	case <-time.After(coldStartPromptWait):
		return fmt.Errorf("the prompt never reached the model")
	}
}

func initializeColdStartScenario(sc *godog.ScenarioContext) {
	s := &coldStartState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server$`, s.startServer)
	sc.Step(`^a chat that was left on disk by an earlier run$`, s.chatLeftOnDisk)
	sc.Step(`^the panel opens every view of the chat at once$`, s.panelOpensEveryView)
	sc.Step(`^every view answers$`, s.everyViewAnswers)
	sc.Step(`^the chat was read from disk once$`, s.readFromDiskOnce)
	sc.Step(`^I send my first prompt$`, s.sendFirstPrompt)
	sc.Step(`^the prompt reaches the model$`, s.promptReachedTheModel)
}

func TestSessionColdStartFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "session_cold_start",
		ScenarioInitializer: initializeColdStartScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/session_cold_start.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("session_cold_start feature failed")
	}
}
