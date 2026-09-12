//go:build http

package httpserver

// Godog harness for features/session_mode_model_sync.feature: the session mode
// and the YAML backend as the SPA sees them. Turns run through the REAL agent
// runner and the REAL openai provider pointed at a streaming stub, so the model
// id that reaches the provider is observable on the wire and the SSE the client
// reads is the production frame sequence. The gateway is rebuilt over the same
// FileStore between steps, which is what proves the mode survives a restart
// instead of only living in the manager's memory.

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/cucumber/godog"
	"github.com/tidwall/gjson"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const modeSyncPlanSlug = "mode-sync-plan"

// modeSyncStubBackend is an OpenAI-compatible chat completions server that
// answers every turn, optionally calling plan_exit first. It records each
// request so the model id stays observable on the wire.
type modeSyncStubBackend struct {
	leavePlan bool

	mu       sync.Mutex
	requests []string
}

func (b *modeSyncStubBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	b.requests = append(b.requests, string(raw))
	turn := len(b.requests)
	b.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	send := func(delta map[string]any, finish any) {
		payload := map[string]any{
			"id": "chatcmpl-mode-sync", "object": "chat.completion.chunk", "model": "stub",
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		data, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	if b.leavePlan && turn == 1 {
		send(map[string]any{
			"role": "assistant",
			"tool_calls": []map[string]any{{
				"index": 0, "id": "call_mode_sync_exit", "type": "function",
				"function": map[string]string{"name": "plan_exit", "arguments": "{}"},
			}},
		}, nil)
		send(map[string]any{}, "tool_calls")
	} else {
		send(map[string]any{"role": "assistant", "content": "Done."}, nil)
		send(map[string]any{}, "stop")
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func (b *modeSyncStubBackend) snapshot() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.requests...)
}

type modeSyncState struct {
	root      string
	cwd       string
	cfg       *config.Config
	backend   *modeSyncStubBackend
	backendTS *httptest.Server
	mgr       *session.Manager
	srv       *Server
	ts        *httptest.Server
	sid       string
	sseBody   string
	patchBody string
}

func (s *modeSyncState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-mode-sync-*")
	if err != nil {
		return err
	}
	s.root = root
	s.sid = ""
	s.sseBody = ""
	s.patchBody = ""
	return nil
}

func (s *modeSyncState) stopGateway() {
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.srv != nil {
		s.srv.Drain()
		s.srv = nil
	}
	s.mgr = nil
}

func (s *modeSyncState) close() {
	s.stopGateway()
	if s.backendTS != nil {
		s.backendTS.Close()
		s.backendTS = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

// bootGateway wires a manager and a server over the session store at s.root.
// The restart step calls it again, which is why it never touches the store.
func (s *modeSyncState) bootGateway() {
	log := slog.Default()
	cfg := s.cfg
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, snd acp.UpdateSender) (string, error) {
		return agent.NewAgent(cfg, st, snd, log).Run(ctx, prompt)
	}
	store := &session.FileStore{Root: filepath.Join(s.root, "sessions")}
	s.mgr = session.NewManager(cfg, noopSender{}, runner, log, s.cwd, store)
	s.srv = New(cfg, s.mgr, log, s.cwd)
	s.ts = httptest.NewServer(s.srv.Handler())
}

// startServer boots the gateway with the REAL agent runner and the REAL provider
// factory, so the transport and the tool sets come from the production path.
func (s *modeSyncState) startServer(leavePlan bool) error {
	home := filepath.Join(s.root, "home")
	s.cwd = filepath.Join(s.root, "workspace")
	for _, dir := range []string{home, s.cwd} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	s.backend = &modeSyncStubBackend{leavePlan: leavePlan}
	s.backendTS = httptest.NewServer(s.backend)

	cfg := &config.Config{
		Paths:     config.Paths{Home: home, CWD: s.cwd},
		Providers: []config.ProviderConfig{{Name: "local", Type: "openai", APIBase: s.backendTS.URL, APIKey: "test-key"}},
		Models:    []config.ModelEntry{{Model: "local/stub"}, {Model: "local/alt"}},
		Agent:     config.Agent{Model: "local/stub"},
	}
	cfg.Tools.PermissionMode = config.PermModeBypass
	s.cfg = cfg
	s.bootGateway()
	return nil
}

func (s *modeSyncState) startPlainServer() error { return s.startServer(false) }

func (s *modeSyncState) startPlanExitingServer() error { return s.startServer(true) }

// restartGateway drops the manager holding the session in memory and builds a
// new one over the same store, so the next read comes off disk.
func (s *modeSyncState) restartGateway() error {
	if s.cfg == nil {
		return fmt.Errorf("the gateway was never started")
	}
	s.stopGateway()
	s.bootGateway()
	return nil
}

func (s *modeSyncState) createSession() error {
	res, err := s.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.cwd})
	if err != nil {
		return err
	}
	s.sid = res.SessionID
	if s.mgr.SessionByID(s.sid) == nil {
		return fmt.Errorf("session %q not registered", s.sid)
	}
	return nil
}

func (s *modeSyncState) writeRunnablePlan() error {
	st := s.mgr.SessionByID(s.sid)
	if st == nil {
		return fmt.Errorf("session %q not registered", s.sid)
	}
	sd := strings.TrimSpace(st.GetPersistedSessionDir())
	if sd == "" {
		return fmt.Errorf("session %q has no persisted bundle", s.sid)
	}
	_, err := plans.Write(sd, modeSyncPlanSlug, plans.DefaultContent(modeSyncPlanSlug, "Mode sync plan"))
	return err
}

// postResponses sends one streamed turn and keeps the raw SSE body.
func (s *modeSyncState) postResponses(body string) error {
	if s.sid == "" {
		s.sid = "sess_mode_sync"
	}
	req, err := http.NewRequest(http.MethodPost, s.ts.URL+"/v1/responses", bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", s.sid)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /v1/responses status %d: %s", res.StatusCode, raw)
	}
	s.sseBody = string(raw)
	if !strings.Contains(s.sseBody, "data: [DONE]") {
		return fmt.Errorf("SSE body was not terminated: %s", s.sseBody)
	}
	return nil
}

func (s *modeSyncState) sendPrompt(mode string) error {
	return s.postResponses(fmt.Sprintf(`{"model":%q,"input":"say something","stream":true}`, mode))
}

func (s *modeSyncState) sendPromptSelectingBackend(mode, backend string) error {
	return s.postResponses(fmt.Sprintf(
		`{"model":%q,"input":"say something","stream":true,"metadata":{"model":%q}}`, mode, backend))
}

func (s *modeSyncState) runPlanFromMode(mode string) error {
	return s.postResponses(fmt.Sprintf(
		`{"model":%q,"input":"Implement the plan.","stream":true,"metadata":{"runPlanSlug":%q}}`,
		mode, modeSyncPlanSlug))
}

func (s *modeSyncState) patchSessionMode(mode string) error {
	body := fmt.Sprintf(`{"mode":%q}`, mode)
	req, err := http.NewRequest(http.MethodPatch, s.ts.URL+"/foxxycode/sessions/"+s.sid, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("PATCH session status %d: %s", res.StatusCode, raw)
	}
	s.patchBody = string(raw)
	return nil
}

func (s *modeSyncState) patchEchoesMode(want string) error {
	if got := gjson.Get(s.patchBody, "mode").String(); got != want {
		return fmt.Errorf("patch response mode = %q, want %q in %s", got, want, s.patchBody)
	}
	return nil
}

// modeFrameNames reports whether the stream carried the named mode frame. The
// SPA reads the frame, not the transcript, which is what keeps the composer
// from posting a profile the session has already left.
func (s *modeSyncState) modeFrameNames(want string) error {
	if !strings.Contains(s.sseBody, "event: mode") {
		return fmt.Errorf("no mode frame on the stream: %s", s.sseBody)
	}
	for _, block := range strings.Split(s.sseBody, "\n\n") {
		if !strings.Contains(block, "event: mode") {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			payload, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}
			if gjson.Get(payload, "currentModeId").String() == want {
				return nil
			}
		}
	}
	return fmt.Errorf("no mode frame named %q on the stream: %s", want, s.sseBody)
}

func (s *modeSyncState) messagesPayload() ([]byte, error) {
	res, err := http.Get(s.ts.URL + "/foxxycode/sessions/" + s.sid + "/messages")
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("messages status %d: %s", res.StatusCode, raw)
	}
	return raw, nil
}

func (s *modeSyncState) transcriptReportsMode(want string) error {
	raw, err := s.messagesPayload()
	if err != nil {
		return err
	}
	if got := gjson.GetBytes(raw, "mode").String(); got != want {
		return fmt.Errorf("transcript mode = %q, want %q", got, want)
	}
	return nil
}

func (s *modeSyncState) transcriptReportsModelOverride(want string) error {
	raw, err := s.messagesPayload()
	if err != nil {
		return err
	}
	if got := gjson.GetBytes(raw, "selectedModelId").String(); got != want {
		return fmt.Errorf("transcript selectedModelId = %q, want %q", got, want)
	}
	return nil
}

// providerReceivedModel checks the id on the wire, which is the model reference
// minus its provider prefix.
func (s *modeSyncState) providerReceivedModel(ref string) error {
	_, api, _ := strings.Cut(ref, "/")
	requests := s.backend.snapshot()
	if len(requests) == 0 {
		return fmt.Errorf("the stub model was never called")
	}
	for _, req := range requests {
		if got := gjson.Get(req, "model").String(); got != api {
			return fmt.Errorf("provider was called with model %q, want %q", got, api)
		}
	}
	return nil
}

func initializeModeSyncScenario(sc *godog.ScenarioContext) {
	s := &modeSyncState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a foxxycode gateway backed by a stub model$`, s.startPlainServer)
	sc.Step(`^a foxxycode gateway backed by a stub model that leaves plan mode$`, s.startPlanExitingServer)
	sc.Step(`^a session created for the client$`, s.createSession)
	sc.Step(`^a design plan the client can run$`, s.writeRunnablePlan)
	sc.Step(`^a client sends an? "([^"]+)" prompt over POST /v1/responses$`, s.sendPrompt)
	sc.Step(`^a client sends an? "([^"]+)" prompt selecting the "([^"]+)" backend$`, s.sendPromptSelectingBackend)
	sc.Step(`^the client runs that plan from "([^"]+)" mode$`, s.runPlanFromMode)
	sc.Step(`^the client patches the session mode to "([^"]+)"$`, s.patchSessionMode)
	sc.Step(`^the patch response echoes the "([^"]+)" mode$`, s.patchEchoesMode)
	sc.Step(`^the gateway restarts$`, s.restartGateway)
	sc.Step(`^the stream carries a mode frame naming "([^"]+)"$`, s.modeFrameNames)
	sc.Step(`^the transcript reports the session in "([^"]+)" mode$`, s.transcriptReportsMode)
	sc.Step(`^the transcript reports "([^"]+)" as the session model override$`, s.transcriptReportsModelOverride)
	sc.Step(`^the provider received the request as "([^"]+)"$`, s.providerReceivedModel)
}

func TestSessionModeModelSyncFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "session-mode-model-sync",
		ScenarioInitializer: initializeModeSyncScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/session_mode_model_sync.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("session mode/model sync feature suite failed")
	}
}
