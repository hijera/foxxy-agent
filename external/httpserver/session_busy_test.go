//go:build http

package httpserver

import (
	"context"
	"encoding/json"
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

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// blockingTurnServer builds an HTTP server whose agent turn blocks until release is
// closed. unwind is how long the runner keeps working after its context is cancelled,
// standing in for the persist / workspace-diff work a real turn does on the way out.
func blockingTurnServer(t *testing.T, unwind ...time.Duration) (srv *Server, started <-chan struct{}, release chan struct{}) {
	t.Helper()
	unwindAfterCancel := time.Duration(0)
	if len(unwind) > 0 {
		unwindAfterCancel = unwind[0]
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sessRoot := filepath.Join(root, "sessions")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	startedCh := make(chan struct{})
	release = make(chan struct{})
	var once sync.Once
	var turns atomic.Int32
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		// Only the first turn is the slow one; a follow-up send (Stop, switch model,
		// ask again) must be free to complete on its own.
		if turns.Add(1) == 1 {
			once.Do(func() { close(startedCh) })
			select {
			case <-release:
			case <-ctx.Done():
				time.Sleep(unwindAfterCancel)
			}
		}
		var sb strings.Builder
		for _, b := range prompt {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: strings.TrimSpace(sb.String())})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "stub"})
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: "/tmp"},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	store := &session.FileStore{Root: sessRoot}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), "/tmp", store)
	srv = New(cfg, mgr, slog.Default(), "/tmp")
	t.Cleanup(srv.Drain)
	return srv, startedCh, release
}

// A second prompt while a turn is in flight must be machine-readable so the SPA can
// re-attach to the running turn instead of dead-ending on a plain error string.
func TestResponsesSessionBusyReturnsStructuredConflict(t *testing.T) {
	srv, started, release := blockingTurnServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	sn, err := srv.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	sid := sn.SessionID

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses", strings.NewReader(`{"model":"agent","input":"long one","stream":true}`))
		if err != nil {
			t.Error(err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-FoxxyCode-Session-ID", sid)
		res, err := ts.Client().Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()
	<-started

	req2, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses", strings.NewReader(`{"model":"agent","input":"second","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-FoxxyCode-Session-ID", sid)
	res2, err := ts.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioReadAllClose(res2.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("busy status %d body %s", res2.StatusCode, b)
	}
	var out struct {
		Error struct {
			Message    string `json:"message"`
			Code       string `json:"code"`
			SessionID  string `json:"sessionId"`
			TurnActive bool   `json:"turnActive"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("busy body %s: %v", b, err)
	}
	if out.Error.Code != "session_busy" {
		t.Errorf("code %q want session_busy (body %s)", out.Error.Code, b)
	}
	if out.Error.SessionID != sid {
		t.Errorf("sessionId %q want %q", out.Error.SessionID, sid)
	}
	if !out.Error.TurnActive {
		t.Errorf("turnActive false, want true (body %s)", b)
	}
	if !strings.Contains(out.Error.Message, "session busy") {
		t.Errorf("message %q lost the human-readable text", out.Error.Message)
	}

	close(release)
	wg.Wait()
}

// Stop, pick another model, ask again: the classic move when a model turns out to be
// too slow. The cancelled turn still has to persist and diff its workspace before it
// lets go of the session, so the follow-up send must wait that out, not fail on it.
func TestResponsesAfterCancelIsAcceptedWhileTurnUnwinds(t *testing.T) {
	srv, started, release := blockingTurnServer(t, 700*time.Millisecond)
	defer close(release)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	sn, err := srv.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	sid := sn.SessionID

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses", strings.NewReader(`{"model":"agent","input":"slow model please","stream":true}`))
		if err != nil {
			t.Error(err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-FoxxyCode-Session-ID", sid)
		res, err := ts.Client().Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()
	<-started

	cancelReq, err := http.NewRequest(http.MethodPost, ts.URL+"/foxxycode/sessions/"+url.PathEscape(sid)+"/cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelReq.Header.Set("X-FoxxyCode-Session-ID", sid)
	cancelRes, err := ts.Client().Do(cancelReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = cancelRes.Body.Close()
	if cancelRes.StatusCode != http.StatusOK {
		t.Fatalf("cancel status %d", cancelRes.StatusCode)
	}

	// No pause here on purpose: this is the user retyping the moment Stop lands.
	req2, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses",
		strings.NewReader(`{"model":"agent","input":"same question, faster model","stream":true,"metadata":{"model":"openai/gpt-4o"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-FoxxyCode-Session-ID", sid)
	res2, err := ts.Client().Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioReadAllClose(res2.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res2.StatusCode == http.StatusConflict {
		t.Fatalf("send right after Stop was refused as busy: %s", b)
	}
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", res2.StatusCode, b)
	}
	wg.Wait()
}

// Behind nginx or a corporate HTTP proxy an SSE body is buffered by default, so the
// answer only reaches the browser once the turn ends - it looks frozen, then complete
// after a reload. Every stream must opt out of that buffering.
func TestStreamingResponsesDisableProxyBuffering(t *testing.T) {
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	sn, err := srv.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	sid := sn.SessionID

	cases := []struct {
		name string
		do   func() (*http.Response, error)
	}{
		{"responses", func() (*http.Response, error) {
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses",
				strings.NewReader(`{"model":"agent","input":"hi","stream":true}`))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-FoxxyCode-Session-ID", sid)
			return ts.Client().Do(req)
		}},
		{"chat completions", func() (*http.Response, error) {
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions",
				strings.NewReader(`{"model":"agent","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-FoxxyCode-Session-ID", sid)
			return ts.Client().Do(req)
		}},
		{"composer relay", func() (*http.Response, error) {
			req, err := http.NewRequest(http.MethodGet,
				ts.URL+"/foxxycode/sessions/"+url.PathEscape(sid)+"/composer-stream", nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("X-FoxxyCode-Session-ID", sid)
			return ts.Client().Do(req)
		}},
		{"ide events", func() (*http.Response, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			t.Cleanup(cancel)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/foxxycode/ide/events", nil)
			if err != nil {
				return nil, err
			}
			return ts.Client().Do(req)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.do()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
				t.Fatalf("content type %q, want an SSE stream", ct)
			}
			if got := res.Header.Get("X-Accel-Buffering"); got != "no" {
				t.Errorf("X-Accel-Buffering = %q, want \"no\": a buffering proxy would hide the whole turn", got)
			}
		})
	}
}

// The composer relay is the SPA's re-attach path after a webview reload. With no turn
// running it must say so immediately instead of holding the request open for 30s.
func TestComposerStreamWithoutActiveTurnFailsFast(t *testing.T) {
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	sn, err := srv.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	sid := sn.SessionID

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/foxxycode/sessions/"+url.PathEscape(sid)+"/composer-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-FoxxyCode-Session-ID", sid)
	start := time.Now()
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioReadAllClose(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %s for an idle session, want an immediate error", elapsed)
	}
	body := string(b)
	if !strings.Contains(body, "event: error") {
		t.Fatalf("missing error event: %s", body)
	}
	// OpenAI-shaped so the SPA's openAIStreamErrorMessage treats it as an error
	// rather than a silently dropped stream.
	if !strings.Contains(body, `"error"`) || !strings.Contains(body, "no active composer stream") {
		t.Fatalf("error payload not OpenAI-shaped: %s", body)
	}
}
