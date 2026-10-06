//go:build http

package httpserver

// Godog harness for features/chat_loads_during_turn.feature: reads a chat over the live
// HTTP surface (GET /foxxycode/sessions/{id}/messages and GET /foxxycode/sessions) while
// an agent turn is writing to that same session.

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
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const (
	chatLoadQuestion = "why is this taking so long"
	chatLoadWait     = 10 * time.Second
	// A list call may not turn into a transcript scan: the panel blocks its first
	// render on it, so it has to stay quick no matter how much history is on disk.
	chatLoadListBudget = 1500 * time.Millisecond
)

type chatLoadFeatureState struct {
	root      string
	sessRoot  string
	ts        *httptest.Server
	srv       *Server
	sessionID string

	turnStarted chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	promptDone  chan struct{}

	loadedMessages []map[string]interface{}
	listElapsed    time.Duration
	listed         []map[string]interface{}
}

func (s *chatLoadFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-chatload-*")
	if err != nil {
		return err
	}
	*s = chatLoadFeatureState{root: root}
	s.turnStarted = make(chan struct{})
	s.release = make(chan struct{})
	return nil
}

func (s *chatLoadFeatureState) close() {
	if s.release != nil {
		s.releaseOnce.Do(func() { close(s.release) })
	}
	if s.promptDone != nil {
		select {
		case <-s.promptDone:
		case <-time.After(chatLoadWait):
		}
		s.promptDone = nil
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

func (s *chatLoadFeatureState) startServer() error {
	home := filepath.Join(s.root, "home")
	s.sessRoot = filepath.Join(s.root, "sessions")
	for _, d := range []string{filepath.Join(home, "memory"), s.sessRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		// The question is persisted up front, exactly like a real turn; the answer
		// only lands when the model finally replies.
		var sb strings.Builder
		for _, b := range prompt {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: strings.TrimSpace(sb.String())})
		close(s.turnStarted)
		select {
		case <-s.release:
		case <-ctx.Done():
		}
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "finally"})
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: s.root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, &session.FileStore{Root: s.sessRoot})
	s.srv = New(cfg, mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(s.srv.Handler())

	sn, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.root})
	if err != nil {
		return err
	}
	s.sessionID = sn.SessionID
	return nil
}

func (s *chatLoadFeatureState) startTurn() error {
	s.promptDone = make(chan struct{})
	body := fmt.Sprintf(`{"model":"agent","input":%q,"stream":true}`, chatLoadQuestion)
	req, err := http.NewRequest(http.MethodPost, s.ts.URL+"/v1/responses", strings.NewReader(body))
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
	select {
	case <-s.turnStarted:
		return nil
	case <-time.After(chatLoadWait):
		return fmt.Errorf("the agent turn never started")
	}
}

// manyLongChats fills the store with other sessions carrying sizeable transcripts -
// the everyday state of a workspace that has been used for a while.
func (s *chatLoadFeatureState) manyLongChats() error {
	filler := strings.Repeat("lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 400)
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("sess_bulk_%02d", i)
		dir := filepath.Join(s.sessRoot, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		meta := map[string]interface{}{
			"version":   1,
			"id":        id,
			"cwd":       s.root,
			"title":     fmt.Sprintf("bulk chat %02d", i),
			"updatedAt": "2026-07-01T10:00:00Z",
		}
		b, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "session.json"), b, 0o644); err != nil {
			return err
		}
		msgs := make([]map[string]string, 0, 200)
		for j := 0; j < 200; j++ {
			role := "assistant"
			if j%2 == 0 {
				role = "user"
			}
			msgs = append(msgs, map[string]string{"role": role, "content": filler})
		}
		payload, err := json.Marshal(map[string]interface{}{"version": 1, "messages": msgs})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "messages.json"), payload, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *chatLoadFeatureState) openChat() error {
	req, err := http.NewRequest(http.MethodGet,
		s.ts.URL+"/foxxycode/sessions/"+url.PathEscape(s.sessionID)+"/messages", nil)
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
		return fmt.Errorf("GET messages returned %d", res.StatusCode)
	}
	var body struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	s.loadedMessages = body.Messages
	return nil
}

func (s *chatLoadFeatureState) chatLoadedWithQuestion() error {
	if len(s.loadedMessages) == 0 {
		return fmt.Errorf("the chat came back empty while its turn was running")
	}
	for _, m := range s.loadedMessages {
		if c, _ := m["content"].(string); strings.Contains(c, chatLoadQuestion) {
			return nil
		}
	}
	return fmt.Errorf("the question is missing from the loaded chat: %+v", s.loadedMessages)
}

func (s *chatLoadFeatureState) listChats() error {
	start := time.Now()
	res, err := http.Get(s.ts.URL + "/foxxycode/sessions?limit=30&include_activity=true")
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET sessions returned %d", res.StatusCode)
	}
	var body struct {
		Sessions []map[string]interface{} `json:"sessions"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	s.listElapsed = time.Since(start)
	s.listed = body.Sessions
	return nil
}

func (s *chatLoadFeatureState) listedAsWorking() error {
	if err := s.listChats(); err != nil {
		return err
	}
	for _, row := range s.listed {
		if id, _ := row["id"].(string); id != s.sessionID {
			continue
		}
		if active, _ := row["turnActive"].(bool); !active {
			return fmt.Errorf("the chat is listed as idle while its turn runs")
		}
		return nil
	}
	return fmt.Errorf("the chat is missing from history: %+v", s.listed)
}

func (s *chatLoadFeatureState) listStaysResponsive() error {
	if err := s.listChats(); err != nil {
		return err
	}
	if len(s.listed) == 0 {
		return fmt.Errorf("history came back empty")
	}
	if s.listElapsed > chatLoadListBudget {
		return fmt.Errorf("listing chats took %s, over the %s budget - the panel would sit on a spinner",
			s.listElapsed, chatLoadListBudget)
	}
	return nil
}

func initializeChatLoadScenario(sc *godog.ScenarioContext) {
	s := &chatLoadFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server$`, s.startServer)
	sc.Step(`^a long agent turn in flight for my chat$`, s.startTurn)
	sc.Step(`^a workspace with many long chats$`, s.manyLongChats)

	sc.Step(`^the panel opens the chat$`, s.openChat)

	sc.Step(`^the chat loads with the question it started from$`, s.chatLoadedWithQuestion)
	sc.Step(`^the chat is listed in history as still working$`, s.listedAsWorking)
	sc.Step(`^listing the chats stays responsive$`, s.listStaysResponsive)
}

func TestChatLoadsDuringTurnFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "chat-loads-during-turn",
		ScenarioInitializer: initializeChatLoadScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/chat_loads_during_turn.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("chat loads during turn feature suite failed")
	}
}
