package agent

import (
	"context"
	"fmt"
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

// stallingHub is an OpenAI-compatible stub whose first request streams a
// delta and then goes silent; every later request answers in full.
func stallingHub(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if hits.Add(1) == 1 {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"content\":\"Hello fr\"}}],\"id\":\"c1\",\"model\":\"m\",\"object\":\"chat.completion.chunk\"}\n\n")
			flusher.Flush()
			<-r.Context().Done()
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"content\":\"om the hub.\"}}],\"id\":\"c2\",\"model\":\"m\",\"object\":\"chat.completion.chunk\"}\n\n"+
			"data: {\"choices\":[{\"finish_reason\":\"stop\",\"index\":0,\"delta\":{}}],\"id\":\"c2\",\"model\":\"m\",\"object\":\"chat.completion.chunk\"}\n\n"+
			"data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func stallingHubAgent(t *testing.T, srv *httptest.Server, tune func(*config.Agent)) (*Agent, *session.State) {
	t.Helper()
	idle := 200
	agentCfg := config.Agent{Model: "stub/model", MaxTurns: 3, LLMStreamIdleTimeoutMS: &idle}
	if tune != nil {
		tune(&agentCfg)
	}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "stub", Type: "openai", APIKey: "test", APIBase: srv.URL}},
		Models:    []config.ModelEntry{{Model: "stub/model", MaxTokens: 100}},
		Agent:     agentCfg,
	}
	st := &session.State{ID: "sess_stall", CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	return NewAgent(cfg, st, &loopGuardSender{}, nil), st
}

// Upstream 1.1.47's case, with agent.llm_continue off: a stream that goes quiet
// after its first deltas is cut by the stall guard of the real provider, the
// text the user watched arrive is persisted, and the turn ends with the stall
// named instead of waiting for a byte that never comes.
func TestStalledStreamKeepsPartialAnswerAndReportsTheStall(t *testing.T) {
	srv, _ := stallingHub(t)
	off := false
	ag, st := stallingHubAgent(t, srv, func(a *config.Agent) { a.LLMContinue = &off })

	// Bounded so a guard that never fires fails the test instead of hanging it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: "hello"}})
	if err == nil || !strings.Contains(err.Error(), "200ms") {
		t.Fatalf("the turn must end with the stall naming the idle time, got err=%v", err)
	}
	msgs := st.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || last.Content != "Hello fr" {
		t.Fatalf("the partial answer must be persisted, last message = %+v", last)
	}
}

// fork(stall-guard-layer) guard, end to end: the same cut over a real HTTP
// provider carries the answer on by default, and the turn ends with it whole.
func TestStalledStreamOverHTTPIsCarriedOn(t *testing.T) {
	srv, hits := stallingHub(t)
	ag, st := stallingHubAgent(t, srv, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stop, err := ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: "hello"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("requests = %d, want the cut one and its continuation", got)
	}
	var texts []string
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleAssistant {
			texts = append(texts, m.Content)
		}
	}
	if strings.Join(texts, "") != "Hello from the hub." {
		t.Fatalf("assistant messages = %q, want the partial and its continuation", texts)
	}
}

// silentStreamProvider never sends a chunk and returns only when the caller's
// context ends, as a provider does when the upstream holds the request.
type silentStreamProvider struct{}

func (silentStreamProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Complete must not be used here")
}

func (silentStreamProvider) Stream(ctx context.Context, _ []llm.Message, _ []llm.ToolDefinition, _ func(llm.StreamChunk)) (*llm.Response, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// A turn interrupted from outside (a signal, a shutdown) while the model has
// produced nothing says how long the model had been silent, so a kill after
// a long silent wait is not reported as a mere interruption (upstream 1.1.47).
func TestInterruptedBeforeOutputNamesTheSilence(t *testing.T) {
	guard := 60000
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "lane", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "lane/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "lane/model", MaxTurns: 3, LLMFirstTokenTimeoutMS: &guard},
	}
	st := &session.State{ID: "sess_silent", CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	ag := NewAgent(cfg, st, &loopGuardSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return silentStreamProvider{}, nil }

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	_, err := ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: "hello"}})
	if err == nil || !strings.Contains(err.Error(), "interrupted before producing a response") {
		t.Fatalf("expected the interruption error, got %v", err)
	}
	if !strings.Contains(err.Error(), "silent for") {
		t.Fatalf("the error must say how long the model was silent: %v", err)
	}
}
