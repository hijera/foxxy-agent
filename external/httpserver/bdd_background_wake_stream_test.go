//go:build http

package httpserver

// Godog harness for features/background_wake_stream.feature: drives a real wake turn
// against a real httptest server and subscribes with GET .../composer-stream, the same
// request the SPA makes when it re-attaches to a session that is already working.

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// wakeStreamAnswer is what the simulated runner streams for the woken turn, distinctive
// enough that finding it in the SSE bytes cannot be an accident.
const wakeStreamAnswer = "the build finished, here is what changed"

type wakeStreamState struct {
	root      string
	ts        *httptest.Server
	mgr       *session.Manager
	srv       *Server
	sessionID string

	// hold parks the runner so a turn can be kept in flight for as long as a scenario
	// needs. Only the turn that set parkNext parks: a wake turn that parked would never
	// end its own stream, which is exactly what the subscriber is waiting for.
	hold     chan struct{}
	released atomic.Bool
	entered  chan struct{}
	parkNext atomic.Bool

	wakeDone  chan error
	userDone  chan struct{}
	userRelay *composerStreamRelay

	// subscribed closes once a composer-stream request has been answered, so the woken
	// turn does not finish and deregister its relay before anyone could attach. Without
	// it the scenario would pass or fail on scheduling luck rather than on behaviour.
	subscribed     chan struct{}
	subscribedOnce atomic.Bool

	streamBody string
}

// waitForSubscriber bounds the gate: a scenario that never subscribes must not stall
// teardown for longer than it takes to notice.
const waitForSubscriber = 2 * time.Second

func (s *wakeStreamState) markSubscribed() {
	if s.subscribed != nil && s.subscribedOnce.CompareAndSwap(false, true) {
		close(s.subscribed)
	}
}

func (s *wakeStreamState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-wake-stream-*")
	if err != nil {
		return err
	}
	s.root = root
	s.sessionID = ""
	s.streamBody = ""
	s.wakeDone = nil
	s.userDone = nil
	s.userRelay = nil
	s.hold = make(chan struct{})
	s.entered = make(chan struct{})
	s.subscribed = make(chan struct{})
	s.released = atomic.Bool{}
	s.parkNext = atomic.Bool{}
	s.subscribedOnce = atomic.Bool{}
	return nil
}

func (s *wakeStreamState) release() {
	if s.hold != nil && s.released.CompareAndSwap(false, true) {
		close(s.hold)
	}
}

func (s *wakeStreamState) close() {
	s.release()
	if s.userDone != nil {
		<-s.userDone
		s.userDone = nil
	}
	if s.wakeDone != nil {
		<-s.wakeDone
		s.wakeDone = nil
	}
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.srv != nil {
		s.srv.Drain()
		s.srv = nil
	}
	bgtask.Default().SetDraining(false)
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

func (s *wakeStreamState) startServerWithSession() error {
	home := filepath.Join(s.root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	sessRoot := filepath.Join(s.root, "sessions")
	if err := os.MkdirAll(sessRoot, 0o755); err != nil {
		return err
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: s.root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}

	// The runner stands in for the agent loop: it answers through whatever sender the
	// caller wired, which for a wake turn is the relay-backed bridge under test. The
	// first turn parks so a scenario can hold the session busy on purpose.
	runner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, sender acp.UpdateSender) (string, error) {
		if s.parkNext.CompareAndSwap(true, false) {
			close(s.entered)
			<-s.hold
		} else {
			// Hold the answer until someone is listening, so the relay is still
			// registered when the subscriber arrives.
			select {
			case <-s.subscribed:
			case <-time.After(waitForSubscriber):
			}
		}
		if sender != nil {
			_ = sender.SendSessionUpdate(st.ID, acp.MessageChunkUpdate{
				SessionUpdate: acp.UpdateTypeAgentMessageChunk,
				Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: wakeStreamAnswer},
			})
		}
		return string(acp.StopReasonEndTurn), nil
	}

	s.mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, &session.FileStore{Root: sessRoot})
	s.srv = New(cfg, s.mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(s.srv.Handler())

	res, err := s.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.root})
	if err != nil {
		return err
	}
	s.sessionID = res.SessionID
	return nil
}

// aUserTurnIsStreaming holds a turn in the runner with a relay of its own registered, the
// state a chat watching a live answer is in.
func (s *wakeStreamState) aUserTurnIsStreaming() error {
	s.parkNext.Store(true)
	s.userRelay = s.srv.beginComposerRelay(s.sessionID)
	st := s.mgr.SessionByID(s.sessionID)
	if st == nil {
		return fmt.Errorf("session %s not found", s.sessionID)
	}
	unlock, err := s.mgr.AcquireComposerTurnLockWaiting(context.Background(), s.sessionID, st, 5*time.Second)
	if err != nil {
		return err
	}

	s.userDone = make(chan struct{})
	go func() {
		defer close(s.userDone)
		defer unlock()
		_, _ = s.mgr.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
			SessionID: s.sessionID,
			Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "user turn"}},
		}, planRunNoopSender{}, &session.PromptRunOpts{SkipTurnLock: true})
	}()

	select {
	case <-s.entered:
		return nil
	case <-time.After(15 * time.Second):
		return fmt.Errorf("the user turn never reached the runner")
	}
}

func (s *wakeStreamState) aFinishedTaskWakesTheSession() error {
	s.wakeDone = make(chan error, 1)
	go func() {
		s.wakeDone <- s.srv.runWakeTurn(context.Background(), s.sessionID,
			"A background task you asked to be notified about has finished.")
	}()
	return nil
}

// iSubscribeToTheComposerStream reads the SSE body to its end, which is what the SPA does
// when it re-attaches to a session that is already working.
func (s *wakeStreamState) iSubscribeToTheComposerStream() error {
	deadline := time.Now().Add(20 * time.Second)
	for {
		req, err := http.NewRequest(http.MethodGet,
			s.ts.URL+"/foxxycode/sessions/"+s.sessionID+"/composer-stream", nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		// Headers are written before the handler starts looking for the relay, so this
		// is the point where the turn may safely answer.
		s.markSubscribed()
		var body strings.Builder
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			body.WriteString(sc.Text())
			body.WriteString("\n")
		}
		_ = resp.Body.Close()
		text := body.String()
		// The wake is started from a goroutine, so the first subscribe can land before
		// the relay is registered; that answers "no active composer stream" at once.
		if !strings.Contains(text, "no active composer stream") || time.Now().After(deadline) {
			s.streamBody = text
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (s *wakeStreamState) streamCarriesTheAnswer() error {
	if !strings.Contains(s.streamBody, wakeStreamAnswer) {
		return fmt.Errorf("the woken turn's answer never reached the stream: %q", s.streamBody)
	}
	return nil
}

func (s *wakeStreamState) streamEndsWithDone() error {
	if !strings.Contains(s.streamBody, "[DONE]") {
		return fmt.Errorf("stream never terminated, so a client reads it as cut: %q", s.streamBody)
	}
	return nil
}

// userTurnKeepsItsStream is the guarantee that makes waiting harmless: beginComposerRelay
// closes whatever relay a session already has, so a wake that grabbed one before winning
// the turn lock would cut the chat off from the answer it is watching.
func (s *wakeStreamState) userTurnKeepsItsStream() error {
	// Give the wake time to be wrong.
	time.Sleep(750 * time.Millisecond)
	if got := s.srv.peekComposerRelay(s.sessionID); got != s.userRelay {
		return fmt.Errorf("the waiting wake replaced the running turn's relay")
	}
	select {
	case err := <-s.wakeDone:
		return fmt.Errorf("the wake ran while the session was busy: %v", err)
	default:
	}
	return nil
}

func initializeWakeStreamScenario(sc *godog.ScenarioContext) {
	s := &wakeStreamState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode http server with a session$`, s.startServerWithSession)
	sc.Step(`^a user turn is streaming on that session$`, s.aUserTurnIsStreaming)
	sc.Step(`^a finished background task wakes that session$`, s.aFinishedTaskWakesTheSession)
	sc.Step(`^I subscribe to the composer stream of that session$`, s.iSubscribeToTheComposerStream)
	sc.Step(`^the stream carries the woken turn's answer$`, s.streamCarriesTheAnswer)
	sc.Step(`^the stream ends with \[DONE\]$`, s.streamEndsWithDone)
	sc.Step(`^the user turn keeps its own composer stream$`, s.userTurnKeepsItsStream)
}

func TestBackgroundWakeStreamFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "background-wake-stream",
		ScenarioInitializer: initializeWakeStreamScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/background_wake_stream.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("background wake stream feature suite failed")
	}
}
