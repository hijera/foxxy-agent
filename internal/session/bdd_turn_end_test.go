package session_test

// Godog harness for features/turn_end_reasons.feature: a real ReAct turn
// through the session manager, whose LLM is the REAL openai provider pointed
// at a scripted OpenAI-compatible stream. The script decides per request what
// the model does - call the read tool, answer, or break off mid-answer with
// the error frame a LiteLLM proxy sends when its fallback fails - so the
// turn's end is asserted on the manager's result, the transcript and what the
// turn announced. Ported from upstream 1.2.9 (#351) and adapted to the fork:
// the stop notice lives in the transcript, the pause is announced as a park,
// the switch is agent.llm_continue and the budget agent.llm_continue_max.

import (
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
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const turnEndAnswer = "The notes hold forty numbered lines."

// scriptedOpenAI is an OpenAI-compatible chat completions stream whose answer
// to the n-th step of the agent (1-based) is written by script. A request
// that offers no tools is not a step of the turn - the session's title is
// generated that way - and gets a plain answer outside the count.
type scriptedOpenAI struct {
	mu       sync.Mutex
	requests [][]byte
	script   func(n int, w io.Writer)
}

func (o *scriptedOpenAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "text/event-stream")
	var probe struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(raw, &probe) != nil || len(probe.Tools) == 0 {
		sseAnswer(w, "Notes summary")
		return
	}
	o.mu.Lock()
	o.requests = append(o.requests, raw)
	n := len(o.requests)
	o.mu.Unlock()
	o.script(n, w)
}

func (o *scriptedOpenAI) calls() [][]byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([][]byte(nil), o.requests...)
}

func sseChunk(w io.Writer, delta map[string]any, finish any) {
	b, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-stand", "object": "chat.completion.chunk", "model": "stand",
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
	})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
}

func sseAnswer(w io.Writer, text string) {
	sseChunk(w, map[string]any{"role": "assistant", "content": text}, nil)
	sseChunk(w, map[string]any{}, "stop")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func sseReadCall(w io.Writer, n int) {
	args, _ := json.Marshal(map[string]any{"path": "notes.txt", "offset": n, "limit": 1})
	sseChunk(w, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
		"index": 0, "id": fmt.Sprintf("call_%d", n), "type": "function",
		"function": map[string]any{"name": "read", "arguments": string(args)},
	}}}, nil)
	sseChunk(w, map[string]any{}, "tool_calls")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

// sseFailMidAnswer streams part of an answer, then the error frame of a
// failed proxy with the given status.
func sseFailMidAnswer(w io.Writer, partial string, code int) {
	sseChunk(w, map[string]any{"role": "assistant", "content": partial}, nil)
	b, _ := json.Marshal(map[string]any{"error": map[string]any{
		"code":    code,
		"message": "litellm.MidStreamFallbackError: litellm.APIConnectionError: peer closed connection without sending complete message body (incomplete chunked read)",
	}})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
}

// parkRecorder is the client of the turn: it keeps the retry updates the turn
// announced.
type parkRecorder struct {
	noopSender
	mu    sync.Mutex
	parks []acp.LLMRetryUpdate
}

func (p *parkRecorder) SendSessionUpdate(_ string, u interface{}) error {
	if r, ok := u.(acp.LLMRetryUpdate); ok {
		p.mu.Lock()
		p.parks = append(p.parks, r)
		p.mu.Unlock()
	}
	return nil
}

type turnEndState struct {
	root   string
	cwd    string
	home   string
	stub   *scriptedOpenAI
	ts     *httptest.Server
	cfg    *config.Config
	state  *session.State
	client *parkRecorder
	res    *acp.SessionPromptResult
	runErr error
	// partial is the text the scripted model streamed before breaking off.
	partial string
}

func (s *turnEndState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-turn-end-*")
	if err != nil {
		return err
	}
	s.root = root
	s.home = filepath.Join(root, "home")
	s.cwd = filepath.Join(root, "workspace")
	for _, dir := range []string{s.home, s.cwd} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	return os.WriteFile(filepath.Join(s.cwd, "notes.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func (s *turnEndState) close() {
	if s.state != nil {
		s.state.CloseAll()
		s.state = nil
	}
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
	s.res, s.runErr, s.partial, s.cfg, s.client = nil, nil, "", nil, nil
}

func (s *turnEndState) serve(script func(n int, w io.Writer)) {
	s.stub = &scriptedOpenAI{script: script}
	s.ts = httptest.NewServer(s.stub)
}

func (s *turnEndState) modelReadsBeforeAnswering(reads int) error {
	s.serve(func(n int, w io.Writer) {
		if n <= reads {
			sseReadCall(w, n)
			return
		}
		sseAnswer(w, turnEndAnswer)
	})
	return nil
}

func (s *turnEndState) modelBreaksOffOnce(code int, partial string) error {
	s.partial = partial
	s.serve(func(n int, w io.Writer) {
		if n == 1 {
			sseFailMidAnswer(w, partial, code)
			return
		}
		sseAnswer(w, " "+turnEndAnswer)
	})
	return nil
}

func (s *turnEndState) agentWithMaxTurns(maxTurns int) error {
	s.cfg = &config.Config{
		Paths:     config.Paths{Home: s.home, CWD: s.cwd},
		Providers: []config.ProviderConfig{{Name: "stand", Type: "openai", APIBase: s.ts.URL, APIKey: "test-key"}},
		Models:    []config.ModelEntry{{Model: "stand/model"}},
		// Short pauses keep the provider-failure recovery fast.
		Agent: config.Agent{Model: "stand/model", MaxTurns: maxTurns, LLMRetryBaseMS: 1, LLMContinueErrorDelaysMS: []int{5}},
	}
	return nil
}

func (s *turnEndState) agentWithoutLimit() error { return s.agentWithMaxTurns(0) }

func (s *turnEndState) agentThatDoesNotCarryOn() error {
	if err := s.agentWithoutLimit(); err != nil {
		return err
	}
	off := false
	s.cfg.Agent.LLMContinue = &off
	return nil
}

func (s *turnEndState) userSendsPrompt() error {
	log := slog.Default()
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, snd acp.UpdateSender) (string, error) {
		s.state = st
		return agent.NewAgent(s.cfg, st, snd, log).Run(ctx, prompt)
	}
	mgr := session.NewManager(s.cfg, noopSender{}, runner, log, s.cwd, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	newRes, err := mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: s.cwd})
	if err != nil {
		return fmt.Errorf("session/new: %w", err)
	}
	s.client = &parkRecorder{}
	s.res, s.runErr = mgr.HandleSessionPromptWithSender(ctx, acp.SessionPromptParams{
		SessionID: newRes.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "read the notes and sum them up"}},
	}, s.client, nil)
	return nil
}

func (s *turnEndState) stop() acp.StopReason {
	if s.res == nil {
		return ""
	}
	return s.res.StopReason
}

func (s *turnEndState) lastAssistantText() string {
	msgs := s.state.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleAssistant && strings.TrimSpace(msgs[i].Content) != "" {
			return msgs[i].Content
		}
	}
	return ""
}

func (s *turnEndState) turnEndsWithAnswer() error {
	if s.runErr != nil {
		return fmt.Errorf("the turn failed: %v", s.runErr)
	}
	if s.stop() != acp.StopReasonEndTurn {
		return fmt.Errorf("stop reason = %q, want end_turn", s.stop())
	}
	if got := s.lastAssistantText(); !strings.Contains(got, turnEndAnswer) {
		return fmt.Errorf("last answer %q is not the model's answer", got)
	}
	return nil
}

func (s *turnEndState) turnFailsWith(want string) error {
	if s.runErr == nil || !strings.Contains(s.runErr.Error(), want) {
		return fmt.Errorf("turn error = %v, want one naming %q", s.runErr, want)
	}
	return nil
}

func (s *turnEndState) modelCalledTimes(n int) error {
	if got := len(s.stub.calls()); got != n {
		return fmt.Errorf("the model was called %d times, want %d", got, n)
	}
	return nil
}

func (s *turnEndState) turnStopsAtLimit() error {
	if s.runErr != nil {
		return fmt.Errorf("the turn failed: %v", s.runErr)
	}
	if s.stop() != acp.StopReasonMaxTurns {
		return fmt.Errorf("stop reason = %q, want max_turns", s.stop())
	}
	return nil
}

func (s *turnEndState) transcriptEndsWithNotice(steps int) error {
	msgs := s.state.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, "agent.max_turns") ||
		!strings.Contains(last.Content, fmt.Sprintf("after %d steps", steps)) {
		return fmt.Errorf("the transcript ends with %+v, want the notice naming agent.max_turns and %d steps", last, steps)
	}
	return nil
}

func (s *turnEndState) resultCarriesTheNotice() error {
	msgs := s.state.GetMessages()
	if s.res == nil || s.res.StopNotice == "" || s.res.StopNotice != msgs[len(msgs)-1].Content {
		return fmt.Errorf("prompt result notice = %+v, want the transcript's notice", s.res)
	}
	return nil
}

func (s *turnEndState) uiLogHasNoCopy() error {
	for _, e := range s.state.GetUILog() {
		if strings.Contains(e.Message, "agent.max_turns") {
			return fmt.Errorf("the UI log carries the notice too: %+v", e)
		}
	}
	return nil
}

func (s *turnEndState) transcriptKeepsPartial(text string) error {
	for _, m := range s.state.GetMessages() {
		if m.Role == llm.RoleAssistant && strings.HasPrefix(strings.TrimSpace(m.Content), text) && !strings.Contains(m.Content, turnEndAnswer) {
			return nil
		}
	}
	return fmt.Errorf("no assistant message keeps the partial answer %q", text)
}

func (s *turnEndState) nextRequestContinued() error {
	calls := s.stub.calls()
	if len(calls) < 2 {
		return fmt.Errorf("the model was called %d time(s), want a second request", len(calls))
	}
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(calls[1], &req); err != nil {
		return err
	}
	// The partial answer, then the request to go on, in that order; the
	// turn context block may follow them.
	partialAt := -1
	for i, m := range req.Messages {
		text := fmt.Sprint(m.Content)
		switch {
		case m.Role == "assistant" && strings.Contains(text, s.partial):
			partialAt = i
		case partialAt >= 0 && m.Role == "user" && strings.Contains(strings.ToLower(text), "continue"):
			return nil
		}
	}
	if partialAt < 0 {
		return fmt.Errorf("the second request does not carry the partial answer: %s", calls[1])
	}
	return fmt.Errorf("the second request does not ask the model to continue after the partial answer: %s", calls[1])
}

func (s *turnEndState) turnAnnouncedThePause() error {
	s.client.mu.Lock()
	defer s.client.mu.Unlock()
	for _, p := range s.client.parks {
		if p.Phase == acp.LLMRetryPhaseContinuing && p.DelayMS == 5 {
			return nil
		}
	}
	return fmt.Errorf("no park with the 5 ms pause was announced: %+v", s.client.parks)
}

func initializeTurnEndScenario(sc *godog.ScenarioContext) {
	s := &turnEndState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})
	sc.Step(`^a model that reads a file (\d+) times before it answers$`, s.modelReadsBeforeAnswering)
	sc.Step(`^a model whose first answer breaks off with "server error (\d+)" after "([^"]+)"$`, s.modelBreaksOffOnce)
	sc.Step(`^an agent with no step limit configured$`, s.agentWithoutLimit)
	sc.Step(`^an agent whose agent\.max_turns is (\d+)$`, s.agentWithMaxTurns)
	sc.Step(`^an agent that does not carry a cut answer on$`, s.agentThatDoesNotCarryOn)
	sc.Step(`^the user sends a prompt$`, s.userSendsPrompt)
	sc.Step(`^the turn ends with the model's answer$`, s.turnEndsWithAnswer)
	sc.Step(`^the turn fails with "([^"]+)"$`, s.turnFailsWith)
	sc.Step(`^the model was called (\d+) times$`, s.modelCalledTimes)
	sc.Step(`^the turn stops at its step limit$`, s.turnStopsAtLimit)
	sc.Step(`^the transcript ends with a notice that names agent\.max_turns and (\d+) steps$`, s.transcriptEndsWithNotice)
	sc.Step(`^the prompt result carries the same notice$`, s.resultCarriesTheNotice)
	sc.Step(`^the session's UI log carries no copy of it$`, s.uiLogHasNoCopy)
	sc.Step(`^the transcript keeps "([^"]+)" that the user already saw$`, s.transcriptKeepsPartial)
	sc.Step(`^the next request carried that part and asked the model to continue$`, s.nextRequestContinued)
	sc.Step(`^the turn announced the pause before it went on$`, s.turnAnnouncedThePause)
}

func TestTurnEndReasonsFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "turn_end_reasons",
		ScenarioInitializer: initializeTurnEndScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/turn_end_reasons.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("turn end reasons feature failed")
	}
}

// runTurnEnd runs one turn of the stand with adjust changing the agent
// section, and returns the state for assertions.
func runTurnEnd(t *testing.T, script func(n int, w io.Writer), adjust func(*config.Agent)) *turnEndState {
	t.Helper()
	s := &turnEndState{}
	if err := s.reset(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	s.serve(script)
	_ = s.agentWithoutLimit()
	if adjust != nil {
		adjust(&s.cfg.Agent)
	}
	if err := s.userSendsPrompt(); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestProviderRecoveryBudgetRunsOut: a lane that keeps failing costs the turn
// its continuation budget (agent.llm_continue_max, 3), then the turn ends with
// the provider's error.
func TestProviderRecoveryBudgetRunsOut(t *testing.T) {
	s := runTurnEnd(t, func(n int, w io.Writer) {
		// A different cut each time: an answer restarted word for word is the
		// repeat detector's case, which withholds the tools and asks for an answer.
		sseFailMidAnswer(w, fmt.Sprintf("Part %d of the notes", n), 500)
	}, nil)
	if s.runErr == nil || !strings.Contains(s.runErr.Error(), "server error 500") {
		t.Fatalf("turn error = %v, want the provider's 500", s.runErr)
	}
	if got := len(s.stub.calls()); got != 1+config.AgentDefaultLLMContinueMax {
		t.Fatalf("the model was called %d times, want the call and %d continuations", got, config.AgentDefaultLLMContinueMax)
	}
}

// TestProviderRecoveryLeavesRefusalsAlone: a 400 in the middle of an answer
// is the provider refusing the request, which no pause changes.
func TestProviderRecoveryLeavesRefusalsAlone(t *testing.T) {
	s := runTurnEnd(t, func(n int, w io.Writer) { sseFailMidAnswer(w, "Part", 400) }, nil)
	if s.runErr == nil || len(s.stub.calls()) != 1 {
		t.Fatalf("turn error = %v after %d calls, want the refusal after one", s.runErr, len(s.stub.calls()))
	}
}

// TestProviderFailureBeforeAnyOutputIsTheLaddersJob: a lane that answers 503
// until the shared allowance is spent cannot bypass it through the stall ladder.
// Nothing is kept and nothing is asked to continue.
func TestProviderFailureBeforeAnyOutputIsTheLaddersJob(t *testing.T) {
	var failed int
	s := &turnEndState{}
	if err := s.reset(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	s.stub = &scriptedOpenAI{script: func(n int, w io.Writer) { sseAnswer(w, turnEndAnswer) }}
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		// Four refusals: the call and the wrapper's three retries.
		if strings.Contains(string(raw), `"tools"`) && failed < 4 {
			failed++
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"no healthy upstream"}}`)
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(raw)))
		s.stub.ServeHTTP(w, r)
	}))
	_ = s.agentWithoutLimit()
	retryMax := 3
	s.cfg.Agent.LLMRetryMax = &retryMax
	s.cfg.Agent.LLMStallRetryDelaysMS = []int{1}
	if err := s.userSendsPrompt(); err != nil {
		t.Fatal(err)
	}
	if s.runErr == nil || !strings.Contains(s.runErr.Error(), "retry allowance exhausted") || failed != 4 {
		t.Fatalf("error = %v after %d failed requests", s.runErr, failed)
	}
	if len(s.stub.calls()) != 0 {
		t.Fatal("the stall ladder bypassed the exhausted shared allowance")
	}

}

// TestProviderRecoveryPauseHonoursStop: a Stop during the pause ends the turn
// as a stop, not as the provider's error.
func TestProviderRecoveryPauseHonoursStop(t *testing.T) {
	s := &turnEndState{}
	if err := s.reset(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	s.serve(func(n int, w io.Writer) { sseFailMidAnswer(w, "Part", 500) })
	_ = s.agentWithoutLimit()
	s.cfg.Agent.LLMContinueErrorDelaysMS = []int{120000}
	st := &session.State{ID: "sess_turn_end_stop", CWD: s.cwd, Mode: session.ModeAgent, SessionDir: filepath.Join(s.root, "bundle")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.SetCancel(cancel)
	// What a Stop does (Manager.HandleSessionCancel): mark it, then cancel.
	time.AfterFunc(200*time.Millisecond, func() {
		st.SetUserCancelledTurn()
		st.Cancel()
	})
	start := time.Now()
	stop, err := agent.NewAgent(s.cfg, st, noopSender{}, slog.Default()).Run(ctx, []acp.ContentBlock{{Type: "text", Text: "go"}})
	if err != nil || stop != string(acp.StopReasonCancelled) {
		t.Fatalf("Run = %q, %v; want cancelled", stop, err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("the pause did not end on Stop (%s)", time.Since(start))
	}
}

// TestProviderRecoveryPauseCutByADeadline: a deadline (not the user) that
// ends the pause ends the turn with the provider's failure, not as a Stop.
func TestProviderRecoveryPauseCutByADeadline(t *testing.T) {
	s := &turnEndState{}
	if err := s.reset(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	s.serve(func(n int, w io.Writer) { sseFailMidAnswer(w, "Part", 500) })
	_ = s.agentWithoutLimit()
	s.cfg.Agent.LLMContinueErrorDelaysMS = []int{120000}
	st := &session.State{ID: "sess_turn_end_deadline", CWD: s.cwd, Mode: session.ModeAgent, SessionDir: filepath.Join(s.root, "bundle")}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	stop, err := agent.NewAgent(s.cfg, st, noopSender{}, slog.Default()).Run(ctx, []acp.ContentBlock{{Type: "text", Text: "go"}})
	if stop == string(acp.StopReasonCancelled) || err == nil || !strings.Contains(err.Error(), "server error 500") {
		t.Fatalf("Run = %q, %v; want the provider's failure", stop, err)
	}
}
