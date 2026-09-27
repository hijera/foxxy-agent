package agent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func sseDelta(text string) string {
	return "data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"content\":\"" + text + "\"}}],\"id\":\"c\",\"model\":\"m\",\"object\":\"chat.completion.chunk\"}\n\n"
}

const sseStop = "data: {\"choices\":[{\"finish_reason\":\"stop\",\"index\":0,\"delta\":{}}],\"id\":\"c\",\"model\":\"m\",\"object\":\"chat.completion.chunk\"}\n\ndata: [DONE]\n\n"

// hubStep is what the stub hub does with one request.
type hubStep int

const (
	hubDrop   hubStep = iota // a delta, then the connection dies
	hubStall                 // a delta, then silence
	hubAnswer                // the rest of the answer
)

func scriptedHub(t *testing.T, steps ...hubStep) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(hits.Add(1))
		step := steps[len(steps)-1]
		if n <= len(steps) {
			step = steps[n-1]
		}
		fl := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch step {
		case hubDrop:
			_, _ = io.WriteString(w, sseDelta("Hello fr"))
			fl.Flush()
			panic(http.ErrAbortHandler)
		case hubStall:
			_, _ = io.WriteString(w, sseDelta("Hello fr"))
			fl.Flush()
			<-r.Context().Done()
		default:
			_, _ = io.WriteString(w, sseDelta("om the hub.")+sseStop)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func recoveryAgent(t *testing.T, srv *httptest.Server, tune func(*config.Agent)) (*Agent, *session.State, *stallSender) {
	t.Helper()
	idle := 150
	agentCfg := config.Agent{
		Model: "stub/model", MaxTurns: 5, LLMStreamIdleTimeoutMS: &idle,
		LLMContinueErrorDelaysMS: []int{40},
	}
	if tune != nil {
		tune(&agentCfg)
	}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "stub", Type: "openai", APIKey: "test", APIBase: srv.URL}},
		Models:    []config.ModelEntry{{Model: "stub/model", MaxTokens: 100}},
		Agent:     agentCfg,
	}
	st := &session.State{ID: "sess_recovery", CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	sender := &stallSender{}
	return NewAgent(cfg, st, sender, nil), st, sender
}

func runRecovery(t *testing.T, ag *Agent) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: "hello"}})
}

func assistantTexts(st *session.State) string {
	var b strings.Builder
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleAssistant {
			b.WriteString(m.Content)
		}
	}
	return b.String()
}

// A connection that dies after the answer began does not end the turn: the text
// the user watched arrive is kept, and after the configured pause the model is
// asked to go on from it (upstream 1.2.9, issue #246).
func TestProviderFailureMidAnswerIsCarriedOn(t *testing.T) {
	srv, hits := scriptedHub(t, hubDrop, hubAnswer)
	ag, st, sender := recoveryAgent(t, srv, nil)

	stop, err := runRecovery(t, ag)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("requests = %d, want the failed one and its continuation", got)
	}
	if got := assistantTexts(st); got != "Hello from the hub." {
		t.Errorf("assistant text = %q, want the kept part and its continuation", got)
	}
	var parked bool
	sender.mu.Lock()
	for _, u := range sender.retries {
		if u.Phase == acp.LLMRetryPhaseContinuing && u.DelayMS == 40 {
			parked = true
		}
	}
	sender.mu.Unlock()
	if !parked {
		t.Error("the turn did not announce the park with the configured pause")
	}
}

// fork(continue-path) guard: upstream turns its recovery off with
// llm_retry_max: 0; here that switch is agent.llm_continue, and the retry count
// of the wrapper has nothing to say about carrying an answer on.
func TestRecoveryIgnoresRetryMaxZero(t *testing.T) {
	srv, hits := scriptedHub(t, hubDrop, hubAnswer)
	zero := 0
	ag, st, _ := recoveryAgent(t, srv, func(a *config.Agent) { a.LLMRetryMax = &zero })

	if _, err := runRecovery(t, ag); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hits.Load() != 2 || assistantTexts(st) != "Hello from the hub." {
		t.Fatalf("llm_retry_max: 0 turned the recovery off: requests=%d text=%q", hits.Load(), assistantTexts(st))
	}
}

// With agent.llm_continue off a provider failure mid-answer ends the turn, as it
// did before the recovery existed.
func TestProviderRecoveryOffEndsTheTurn(t *testing.T) {
	srv, hits := scriptedHub(t, hubDrop, hubAnswer)
	off := false
	ag, _, _ := recoveryAgent(t, srv, func(a *config.Agent) { a.LLMContinue = &off })

	if _, err := runRecovery(t, ag); err == nil {
		t.Fatal("expected the turn to end with the provider's failure")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

// fork(continue-budget) guard: a stall and a provider failure draw on one
// budget per turn, agent.llm_continue_max, and neither is a step of max_turns.
func TestContinuationBudgetSharedPerTurnNotMaxTurns(t *testing.T) {
	srv, hits := scriptedHub(t, hubStall, hubDrop, hubStall)
	two := 2
	ag, st, _ := recoveryAgent(t, srv, func(a *config.Agent) {
		a.LLMContinueMax = &two
		a.MaxTurns = 1
	})

	_, err := runRecovery(t, ag)
	if err == nil {
		t.Fatal("expected the turn to end once the shared budget ran out")
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("requests = %d, want the first call and two continuations", got)
	}
	if got := assistantTexts(st); !strings.HasPrefix(got, "Hello fr") {
		t.Errorf("the partial answers were not kept: %q", got)
	}
}

// The pause after a provider failure is the configured list, or the longer
// pause the provider asked for, up to llm_continue_retry_after_max_ms.
func TestErrorContinueDelayHonoursRetryAfterUpToTheCap(t *testing.T) {
	var cfg config.Agent
	asked := &llm.QuotaResetError{Delay: 10 * time.Minute, Cause: errors.New("limit")}
	if got := errorContinueDelay(&cfg, 0, errors.New("boom")); got != 5*time.Second {
		t.Errorf("first pause = %v, want 5s", got)
	}
	if got := errorContinueDelay(&cfg, 5, errors.New("boom")); got != 20*time.Second {
		t.Errorf("a later pause = %v, want the last entry, 20s", got)
	}
	if got := errorContinueDelay(&cfg, 0, asked); got != 2*time.Minute {
		t.Errorf("a 10 min Retry-After = %v, want the 2 min cap", got)
	}
	zero := 0
	cfg.LLMContinueRetryAfterMaxMS = &zero
	if got := errorContinueDelay(&cfg, 0, asked); got != 5*time.Second {
		t.Errorf("with the cap at 0 the pause = %v, want the list's 5s", got)
	}
}

// fork(stop-notice-transcript) guard: a turn cut by its step limit says so in
// the answer and in the transcript, not in the UI log, and hands the same text
// to the session manager.
func TestMaxTurnsNoticeStreamedStoredNotUILog(t *testing.T) {
	p := scriptedStream{steps: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "no_such_tool", InputJSON: `{}`}}, StopReason: "tool_calls"},
	}}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 1},
	}
	st := &session.State{ID: "sess_cap", CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	ag := NewAgent(cfg, st, &loopGuardSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return &p, nil }

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "go"}})
	if err != nil || stop != string(acp.StopReasonMaxTurns) {
		t.Fatalf("stop=%q err=%v, want max_turns", stop, err)
	}
	want := ag.maxTurnsNotice(1)
	msgs := st.GetMessages()
	if last := msgs[len(msgs)-1]; last.Role != llm.RoleAssistant || last.Content != want {
		t.Fatalf("last message = %+v, want the notice in the transcript", last)
	}
	if got := st.TakeTurnStopNotice(); got != want {
		t.Fatalf("stop notice = %q, want %q", got, want)
	}
	for _, e := range st.GetUILog() {
		if strings.Contains(e.Message, "step limit") {
			t.Fatalf("the notice was written to the UI log too: %+v", e)
		}
	}
}

// scriptedStream answers each call with the next response of its script,
// repeating the last one.
type scriptedStream struct {
	steps []llm.Response
	calls int
}

func (p *scriptedStream) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, errors.New("Complete must not be used here")
}

func (p *scriptedStream) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	r := p.steps[min(p.calls, len(p.steps)-1)]
	p.calls++
	for i := range r.ToolCalls {
		tc := r.ToolCalls[i]
		onChunk(llm.StreamChunk{ToolCall: &tc})
	}
	if r.Content != "" {
		onChunk(llm.StreamChunk{TextDelta: r.Content})
	}
	out := r
	return &out, nil
}

// A subagent that stopped short reports its answer and, under it, why.
func TestSubagentReportKeepsTheAnswerAboveTheStopNotice(t *testing.T) {
	notice := "The subagent stopped after 3 steps, the step limit set by agent.max_turns. Its report may be incomplete: raise the limit or run it again."
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "task"},
		{Role: llm.RoleAssistant, Content: "Found two call sites."},
		{Role: llm.RoleAssistant, Content: notice},
	}
	got := subagentReport(msgs, &acp.SessionPromptResult{StopReason: acp.StopReasonMaxTurns, StopNotice: notice})
	if got != "Found two call sites.\n\n"+notice {
		t.Fatalf("report = %q", got)
	}
	if got := subagentReport(msgs[:2], &acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}); got != "Found two call sites." {
		t.Fatalf("report without a notice = %q", got)
	}
}
