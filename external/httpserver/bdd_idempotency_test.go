//go:build http

package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
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

type idempotencyFeature struct {
	root          string
	srv           *Server
	calls         int
	first, second *httptest.ResponseRecorder
}

func (s *idempotencyFeature) boot() {
	cfg := &config.Config{Paths: config.Paths{Home: s.root, CWD: s.root}, Agent: config.Agent{Model: "fake/model"}, Models: []config.ModelEntry{{Model: "fake/model"}}}
	runner := func(_ context.Context, st *session.State, prompt []acp.ContentBlock, snd acp.UpdateSender) (string, error) {
		s.calls++
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: prompt[0].Text})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "original answer"})
		_ = snd.SendSessionUpdate(st.GetID(), acp.MessageChunkUpdate{SessionUpdate: acp.UpdateTypeAgentMessageChunk, Content: acp.ContentBlock{Type: acp.ContentTypeText, Text: "original answer"}})
		return string(acp.StopReasonEndTurn), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, &session.FileStore{Root: filepath.Join(s.root, "sessions")})
	s.srv = New(cfg, mgr, slog.Default(), s.root)
}

func (s *idempotencyFeature) request(key, input string, stream bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":"agent","input":%q,"stream":%t}`, input, stream)))
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(w, req)
	return w
}

func TestResponseIdempotencyFeature(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	suite := godog.TestSuite{Name: "response-idempotency", Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/response_idempotency.feature"}, TestingT: t, Strict: true}, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^a persisted response session$`, func() error { s.boot(); return nil })
		sc.Step(`^a response request with an idempotency key completes$`, func() error {
			s.first = s.request("request-1", "fix it", true)
			if s.first.Code != 200 {
				return fmt.Errorf("status %d: %s", s.first.Code, s.first.Body.String())
			}
			return nil
		})
		sc.Step(`^the server is restarted and the same request is sent again$`, func() error { s.srv.Drain(); s.boot(); s.second = s.request("request-1", "fix it", true); return nil })
		sc.Step(`^the original response is replayed without another agent run$`, func() error {
			if s.calls != 1 || s.second.Code != s.first.Code || s.second.Body.String() != s.first.Body.String() || s.second.Header().Get("X-FoxxyCode-Session-ID") != s.first.Header().Get("X-FoxxyCode-Session-ID") {
				return fmt.Errorf("calls=%d, replay status=%d, body=%s", s.calls, s.second.Code, s.second.Body.String())
			}
			return nil
		})
	}}
	if suite.Run() != 0 {
		t.Fatal("response idempotency feature failed")
	}
	s.srv.Drain()
}

func TestResponseIdempotencyRejectsChangedPayload(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	s.request("same-key", "first", false)
	w := s.request("same-key", "changed", false)
	if w.Code != 409 || s.calls != 1 {
		t.Fatalf("status=%d calls=%d", w.Code, s.calls)
	}
}

func TestResponseIdempotencyAllowsIntentionalNewSubmissions(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	for _, key := range []string{"first", "second", "", ""} {
		w := s.request(key, "same text", false)
		if w.Code != 200 {
			t.Fatalf("key=%q status=%d", key, w.Code)
		}
	}
	if s.calls != 4 {
		t.Fatalf("intentional requests suppressed: calls=%d", s.calls)
	}
}

func TestResponseIdempotencyKeepsOpaqueKeysDistinct(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	for _, key := range []string{`"request-1"`, `"request-\u0031"`, "[1,2]", "[1, 2]"} {
		w := s.request(key, "same text", false)
		if w.Code != 200 || w.Header().Get("Idempotency-Replayed") != "" {
			t.Fatalf("distinct key %q was replayed: status=%d headers=%v", key, w.Code, w.Header())
		}
	}
	if s.calls != 4 {
		t.Fatalf("different keys started %d turns, want 4", s.calls)
	}
}

func TestResponseIdempotencyUnpublishedLegacyClaimIsInterrupted(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	dir := filepath.Join(s.srv.mgr.FileStore().Root, ".response_requests", session.ObservationHash("claim-crash"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		w := s.request("claim-crash", "fix it", false)
		if w.Code != 202 || s.calls != 0 || !strings.Contains(w.Body.String(), "interrupted") {
			t.Fatalf("retry %d: status=%d calls=%d body=%s", i, w.Code, s.calls, w.Body.String())
		}
	}
}

func TestResponseIdempotencyInterruptedClaimDoesNotRun(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	_, _, err := s.srv.mgr.FileStore().ClaimResponseRequest("crashed", session.ObservationHash("/v1/responses", "", `{"model":"agent","input":"fix it","stream":false}`), "")
	if err != nil {
		t.Fatal(err)
	}
	w := s.request("crashed", "fix it", false)
	if w.Code != 202 || s.calls != 0 || !strings.Contains(w.Body.String(), "interrupted") {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body.String())
	}
}

func TestResponseIdempotencyActiveClaimDoesNotRun(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	_, _, err := s.srv.mgr.FileStore().ClaimResponseRequest("active", session.ObservationHash("/v1/responses", "", `{"model":"agent","input":"fix it","stream":false}`), "")
	if err != nil {
		t.Fatal(err)
	}
	s.srv.responseRequests.Store("active", true)
	w := s.request("active", "fix it", false)
	if w.Code != 202 || s.calls != 0 || !strings.Contains(w.Body.String(), "in_progress") {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body.String())
	}
}

func TestResponseIdempotencyCORSHeaders(t *testing.T) {
	s := &idempotencyFeature{root: t.TempDir()}
	s.boot()
	defer s.srv.Drain()
	s.srv.activeCfg().HTTPServer.CORS = config.HTTPCORSConfig{Enabled: true, AllowedOrigins: []string{"https://first.example", "https://second.example"}}
	preflight := httptest.NewRequest("OPTIONS", "/v1/responses", nil)
	preflight.Header.Set("Origin", "https://first.example")
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(w, preflight)
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatal("idempotency header not allowed")
	}
	for _, origin := range []string{"https://first.example", "https://second.example"} {
		r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"agent","input":"hello"}`))
		r.Header.Set("Idempotency-Key", "cors")
		r.Header.Set("Origin", origin)
		w = httptest.NewRecorder()
		s.srv.Handler().ServeHTTP(w, r)
		if w.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatalf("replayed stale CORS policy: %v", w.Header())
		}
	}
}
