//go:build http

package httpserver

// Godog harness for features/session_busy_recovery.feature: drives a real agent turn
// that outlives the HTTP request that started it (webview reload), then the two client
// paths back to it - GET /foxxycode/sessions/{id}/composer-stream and the 409 a second
// POST /v1/responses receives while the turn holds the session lock.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const (
	busyBDDPreReloadText  = "before-the-reload"
	busyBDDPostReloadText = "after-the-reload"
	busyBDDWait           = 10 * time.Second
	// How long the cancelled turn keeps the session while it unwinds.
	busyBDDCancelUnwind = 700 * time.Millisecond
)

type sessionBusyFeatureState struct {
	root      string
	ts        *httptest.Server
	srv       *Server
	sessionID string

	// turn control
	turnStarted chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	turnDone    chan struct{}
	turns       atomic.Int32

	// first (abandoned) composer POST
	promptCancel context.CancelFunc
	promptDone   chan struct{}

	// re-attached relay reader
	relayBody io.ReadCloser
	relayMu   sync.Mutex
	relayBuf  strings.Builder
	relayErr  error

	turnActive  bool
	busyStatus  int
	busyPayload struct {
		Error struct {
			Message    string `json:"message"`
			Code       string `json:"code"`
			SessionID  string `json:"sessionId"`
			TurnActive bool   `json:"turnActive"`
		} `json:"error"`
	}
	reattachElapsed time.Duration
}

func (s *sessionBusyFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-busy-*")
	if err != nil {
		return err
	}
	*s = sessionBusyFeatureState{root: root}
	s.turnStarted = make(chan struct{})
	s.release = make(chan struct{})
	s.turnDone = make(chan struct{})
	return nil
}

func (s *sessionBusyFeatureState) close() {
	if s.release != nil {
		s.releaseOnce.Do(func() { close(s.release) })
	}
	if s.promptCancel != nil {
		s.promptCancel()
		s.promptCancel = nil
	}
	if s.relayBody != nil {
		_ = s.relayBody.Close()
		s.relayBody = nil
	}
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

// startServer boots a server whose agent turn emits one chunk, then waits to be
// released, then emits a second chunk - a stand-in for a slow model call.
func (s *sessionBusyFeatureState) startServer() error {
	home := filepath.Join(s.root, "home")
	sessRoot := filepath.Join(s.root, "sessions")
	for _, d := range []string{filepath.Join(home, "memory"), sessRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	runner := func(ctx context.Context, st *session.State, _ []acp.ContentBlock, sender acp.UpdateSender) (string, error) {
		// Only the first turn is the slow one. A follow-up question (after Stop, often
		// on a different model) has to be able to run on its own.
		if s.turns.Add(1) > 1 {
			st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "second answer"})
			return string(acp.StopReasonEndTurn), nil
		}
		defer close(s.turnDone)
		_ = sender.SendSessionUpdate(st.ID, acp.MessageChunkUpdate{
			SessionUpdate: acp.UpdateTypeAgentMessageChunk,
			Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: busyBDDPreReloadText},
		})
		close(s.turnStarted)
		select {
		case <-s.release:
		case <-ctx.Done():
			// A cancelled turn does not free the session instantly: it still persists
			// and diffs the workspace on the way out.
			time.Sleep(busyBDDCancelUnwind)
			return string(acp.StopReasonCancelled), ctx.Err()
		}
		_ = sender.SendSessionUpdate(st.ID, acp.MessageChunkUpdate{
			SessionUpdate: acp.UpdateTypeAgentMessageChunk,
			Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: busyBDDPostReloadText},
		})
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "long one"})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: busyBDDPreReloadText + busyBDDPostReloadText})
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: s.root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, &session.FileStore{Root: sessRoot})
	s.srv = New(cfg, mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(s.srv.Handler())

	sn, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.root})
	if err != nil {
		return err
	}
	s.sessionID = sn.SessionID
	return nil
}

// startTurn fires the streaming composer POST the SPA sends and leaves it running.
func (s *sessionBusyFeatureState) startTurn() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.promptCancel = cancel
	s.promptDone = make(chan struct{})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ts.URL+"/v1/responses",
		strings.NewReader(`{"model":"agent","input":"long one","stream":true}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	go func() {
		defer close(s.promptDone)
		res, err := s.ts.Client().Do(req)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()
	return nil
}

func (s *sessionBusyFeatureState) turnProducedOutput() error {
	select {
	case <-s.turnStarted:
		return nil
	case <-time.After(busyBDDWait):
		return fmt.Errorf("the agent turn never started")
	}
}

// reloadPanel drops the streaming POST the way a webview reload drops its fetch.
func (s *sessionBusyFeatureState) reloadPanel() error {
	if s.promptCancel == nil {
		return fmt.Errorf("no turn to reload away from")
	}
	s.promptCancel()
	s.promptCancel = nil
	select {
	case <-s.promptDone:
	case <-time.After(busyBDDWait):
		return fmt.Errorf("the abandoned request never returned")
	}
	return nil
}

func (s *sessionBusyFeatureState) askIfWorking() error {
	res, err := http.Get(s.ts.URL + "/foxxycode/sessions/" + url.PathEscape(s.sessionID) + "/activity")
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET activity returned %d", res.StatusCode)
	}
	var body struct {
		TurnActive bool `json:"turnActive"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	s.turnActive = body.TurnActive
	return nil
}

func (s *sessionBusyFeatureState) chatReportedWorking() error {
	if err := s.askIfWorking(); err != nil {
		return err
	}
	if !s.turnActive {
		return fmt.Errorf("the chat reports no turn in flight")
	}
	return nil
}

// reattach subscribes to the composer relay and keeps reading it in the background,
// which is what the reloaded SPA does.
func (s *sessionBusyFeatureState) reattach() error {
	req, err := http.NewRequest(http.MethodGet,
		s.ts.URL+"/foxxycode/sessions/"+url.PathEscape(s.sessionID)+"/composer-stream", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	start := time.Now()
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return fmt.Errorf("composer-stream returned %d", res.StatusCode)
	}
	s.relayBody = res.Body
	go func() {
		buf := make([]byte, 512)
		for {
			n, err := res.Body.Read(buf)
			if n > 0 {
				s.relayMu.Lock()
				s.relayBuf.Write(buf[:n])
				s.relayMu.Unlock()
			}
			if err != nil {
				s.relayMu.Lock()
				s.relayErr = err
				s.relayMu.Unlock()
				return
			}
		}
	}()
	// The immediate-error path is what "without waiting" asserts on later.
	if err := s.waitForRelay(func(seen string) bool { return seen != "" }); err != nil {
		return err
	}
	s.reattachElapsed = time.Since(start)
	return nil
}

func (s *sessionBusyFeatureState) waitForRelay(ok func(seen string) bool) error {
	deadline := time.Now().Add(busyBDDWait)
	for {
		s.relayMu.Lock()
		seen := s.relayBuf.String()
		s.relayMu.Unlock()
		if ok(seen) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("relay never matched; saw %q", seen)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *sessionBusyFeatureState) replayedEarlierOutput() error {
	return s.waitForRelay(func(seen string) bool { return strings.Contains(seen, busyBDDPreReloadText) })
}

func (s *sessionBusyFeatureState) receivedRestOfAnswer() error {
	return s.waitForRelay(func(seen string) bool {
		return strings.Contains(seen, busyBDDPostReloadText) && strings.Contains(seen, "[DONE]")
	})
}

func (s *sessionBusyFeatureState) noLiveTurnReported() error {
	if err := s.waitForRelay(func(seen string) bool {
		return strings.Contains(seen, "event: error") && strings.Contains(seen, "no active composer stream")
	}); err != nil {
		return err
	}
	// Anything near composerStreamWaitDeadline means the client sat blind on a
	// finished turn instead of falling back to the persisted transcript.
	if s.reattachElapsed > 5*time.Second {
		return fmt.Errorf("relay took %s to report an idle chat", s.reattachElapsed)
	}
	s.relayMu.Lock()
	seen := s.relayBuf.String()
	s.relayMu.Unlock()
	// OpenAI-shaped so SPA stream readers classify it as an error, not a dropped stream.
	if !strings.Contains(seen, `{"error":`) {
		return fmt.Errorf("error payload is not OpenAI-shaped: %q", seen)
	}
	return nil
}

func (s *sessionBusyFeatureState) finishTurn() error {
	s.releaseOnce.Do(func() { close(s.release) })
	select {
	case <-s.turnDone:
	case <-time.After(busyBDDWait):
		return fmt.Errorf("the agent turn never finished")
	}
	// The lock is dropped by the handler a moment after the runner returns.
	deadline := time.Now().Add(busyBDDWait)
	for {
		if err := s.askIfWorking(); err != nil {
			return err
		}
		if !s.turnActive {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the chat still reports a turn in flight")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// stopGeneration is the composer's Stop button: POST /cancel, which returns as soon as
// cancellation is *requested* - the turn is still unwinding at that point.
func (s *sessionBusyFeatureState) stopGeneration() error {
	req, err := http.NewRequest(http.MethodPost,
		s.ts.URL+"/foxxycode/sessions/"+url.PathEscape(s.sessionID)+"/cancel", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("cancel returned %d", res.StatusCode)
	}
	return nil
}

// askAgainWithAnotherModel is the retry a user makes right after Stop, usually after
// switching the backing model because the previous one was too slow. No pause: the
// point is that it lands while the cancelled turn still owns the session.
func (s *sessionBusyFeatureState) askAgainWithAnotherModel() error {
	req, err := http.NewRequest(http.MethodPost, s.ts.URL+"/v1/responses",
		strings.NewReader(`{"model":"agent","input":"same question, faster model","stream":true,"metadata":{"model":"openai/gpt-4o"}}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	s.busyStatus = res.StatusCode
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

func (s *sessionBusyFeatureState) newRequestAccepted() error {
	if s.busyStatus == http.StatusConflict {
		return fmt.Errorf("the retry after Stop was refused as busy")
	}
	if s.busyStatus != http.StatusOK {
		return fmt.Errorf("retry status %d, want 200", s.busyStatus)
	}
	return nil
}

func (s *sessionBusyFeatureState) sendAnotherMessage() error {
	req, err := http.NewRequest(http.MethodPost, s.ts.URL+"/v1/responses",
		strings.NewReader(`{"model":"agent","input":"something else","stream":true}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	s.busyStatus = res.StatusCode
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	s.busyPayload.Error.Message = ""
	if err := json.Unmarshal(b, &s.busyPayload); err != nil {
		return fmt.Errorf("busy body %s: %w", b, err)
	}
	return nil
}

func (s *sessionBusyFeatureState) refusedAsBusy() error {
	if s.busyStatus != http.StatusConflict {
		return fmt.Errorf("status %d, want 409", s.busyStatus)
	}
	if s.busyPayload.Error.Code != "session_busy" {
		return fmt.Errorf("code %q, want session_busy", s.busyPayload.Error.Code)
	}
	if s.busyPayload.Error.SessionID != s.sessionID {
		return fmt.Errorf("sessionId %q, want %q", s.busyPayload.Error.SessionID, s.sessionID)
	}
	if !s.busyPayload.Error.TurnActive {
		return fmt.Errorf("turnActive false, want true")
	}
	if !strings.Contains(s.busyPayload.Error.Message, "session busy") {
		return fmt.Errorf("message %q lost the human-readable text", s.busyPayload.Error.Message)
	}
	return nil
}

func initializeSessionBusyScenario(sc *godog.ScenarioContext) {
	s := &sessionBusyFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server$`, s.startServer)
	sc.Step(`^a long agent turn in flight for my chat$`, s.startTurn)
	sc.Step(`^the turn has already produced some output$`, s.turnProducedOutput)

	sc.Step(`^the IDE panel reloads$`, s.reloadPanel)
	sc.Step(`^the panel re-attaches to the live turn$`, s.reattach)
	sc.Step(`^the turn finishes$`, s.finishTurn)
	sc.Step(`^I send another message to that chat$`, s.sendAnotherMessage)
	sc.Step(`^I stop the generation$`, s.stopGeneration)
	sc.Step(`^I ask again with another model$`, s.askAgainWithAnotherModel)

	sc.Step(`^my chat is reported as still working$`, s.chatReportedWorking)
	sc.Step(`^it replays the output produced before the reload$`, s.replayedEarlierOutput)
	sc.Step(`^the re-attached panel receives the rest of the answer$`, s.receivedRestOfAnswer)
	sc.Step(`^it reports that there is no live turn without waiting$`, s.noLiveTurnReported)
	sc.Step(`^the send is refused as busy and names the running chat$`, s.refusedAsBusy)
	sc.Step(`^the new request is accepted$`, s.newRequestAccepted)
}

func TestSessionBusyRecoveryFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "session-busy-recovery",
		ScenarioInitializer: initializeSessionBusyScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/session_busy_recovery.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("session busy recovery feature suite failed")
	}
}
