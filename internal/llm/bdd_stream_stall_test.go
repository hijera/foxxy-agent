package llm

// Godog harness for features/llm_stream_stall.feature: exercises the real
// OpenAI provider, built through NewProvider with a stream idle timeout,
// against a stub upstream that starts answering and then goes quiet. The
// guard has to cut the stream after the idle time and keep the delivered text
// next to a stall error; keep-alive comments must not hold it off, and
// progress frames must. Ported from upstream 1.1.47 and adapted to the
// fork's chunk-driven guard (stream_idle_guard.go).

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

	"github.com/cucumber/godog"
)

type streamStallState struct {
	server   *httptest.Server
	provider Provider
	requests atomic.Int32
	idle     time.Duration
	resp     *Response
	callErr  error
}

func (s *streamStallState) reset() {
	s.cleanup()
	s.provider = nil
	s.requests.Store(0)
	s.idle = 0
	s.resp = nil
	s.callErr = nil
}

func (s *streamStallState) cleanup() {
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
}

func stallDelta(text string) string {
	return "data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"content\":\"" + text + "\"}}],\"id\":\"chatcmpl-s1\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n"
}

// stallCompletion is the answer the healthy request streams.
func stallCompletion(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w,
		stallDelta("Hello after retry")+
			"data: {\"choices\":[{\"finish_reason\":\"stop\",\"index\":0,\"delta\":{}}],\"id\":\"chatcmpl-s2\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n"+
			"data: [DONE]\n\n")
}

func (s *streamStallState) newProvider(idleMS int) error {
	s.idle = time.Duration(idleMS) * time.Millisecond
	provider, err := NewProvider(ProviderInput{
		Type:              "openai",
		Model:             "test-model",
		BaseURL:           s.server.URL,
		RetryMax:          1,
		RetryBase:         time.Millisecond,
		RetryMaxDelay:     time.Millisecond,
		StreamIdleTimeout: s.idle,
	})
	if err != nil {
		return fmt.Errorf("create openai provider: %w", err)
	}
	s.provider = provider
	return nil
}

// aProviderStallingAfterTextDeltas streams two deltas and then holds the
// connection open without another byte until the client gives up.
func (s *streamStallState) aProviderStallingAfterTextDeltas(idleMS int) error {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Both deltas in one write, so a slow runner cannot open a gap the
		// guard would take for the stall.
		_, _ = io.WriteString(w, stallDelta("Hello")+stallDelta(" fr"))
		flusher.Flush()
		// The model is stuck: nothing more arrives until the client cancels.
		<-r.Context().Done()
	}))
	return s.newProvider(idleMS)
}

// aProviderSendingKeepalivesAfterTextDeltas streams two deltas and then only
// keep-alive comments and empty data lines, the way a gateway keeps a
// connection warm for a model that is gone.
func (s *streamStallState) aProviderSendingKeepalivesAfterTextDeltas(idleMS int) error {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, stallDelta("Hello")+stallDelta(" fr"))
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(30 * time.Millisecond):
				_, _ = io.WriteString(w, ": keep-alive\n\ndata:\n\n")
				flusher.Flush()
			}
		}
	}))
	return s.newProvider(idleMS)
}

// aProviderStreamingToolArguments streams a tool call's arguments a fragment
// at a time for longer than the idle time, then finishes it.
func (s *streamStallState) aProviderStreamingToolArguments(idleMS, forMS int) error {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		deadline := time.Now().Add(time.Duration(forMS) * time.Millisecond)
		first := true
		for time.Now().Before(deadline) {
			frag := "a"
			if first {
				frag, first = `{"q":"`, false
			}
			_, _ = io.WriteString(w, toolArgFrame(frag))
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(40 * time.Millisecond):
			}
		}
		_, _ = io.WriteString(w, toolArgFrame(`"}`)+
			"data: {\"choices\":[{\"finish_reason\":\"tool_calls\",\"index\":0,\"delta\":{}}],\"id\":\"c1\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n"+
			"data: [DONE]\n\n")
		flusher.Flush()
	}))
	return s.newProvider(idleMS)
}

// aProviderStallingOnceAfterEmptyFrame answers the first request with a
// role-only chunk and then nothing; the second request streams normally.
func (s *streamStallState) aProviderStallingOnceAfterEmptyFrame(idleMS int) error {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.requests.Add(1) == 1 {
			flusher, _ := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"finish_reason\":null,\"index\":0,\"delta\":{\"role\":\"assistant\"}}],\"id\":\"chatcmpl-s0\",\"model\":\"test-model\",\"object\":\"chat.completion.chunk\"}\n\n")
			flusher.Flush()
			<-r.Context().Done()
			return
		}
		stallCompletion(w)
	}))
	return s.newProvider(idleMS)
}

func (s *streamStallState) aStreamingCompletionIsRequested() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s.resp, s.callErr = s.provider.Stream(ctx,
		[]Message{{Role: RoleUser, Content: "hello"}},
		nil,
		func(StreamChunk) {})
	return nil
}

func (s *streamStallState) theCallFailsWithAStallError() error {
	if s.callErr == nil {
		return fmt.Errorf("expected a stall error, the call succeeded with %q", contentOf(s.resp))
	}
	if !IsStreamStalled(s.callErr) {
		return fmt.Errorf("error is not a stall: %v", s.callErr)
	}
	if !strings.Contains(s.callErr.Error(), s.idle.String()) {
		return fmt.Errorf("error does not name the idle time %v: %v", s.idle, s.callErr)
	}
	return nil
}

func (s *streamStallState) thePartialResponsePreservesText(want string) error {
	if s.resp == nil {
		return fmt.Errorf("no partial response returned next to the error")
	}
	if s.resp.Content != want {
		return fmt.Errorf("partial content = %q, want %q", s.resp.Content, want)
	}
	return nil
}

func (s *streamStallState) noPartialResponseIsKept() error {
	if s.resp != nil && (s.resp.Content != "" || s.resp.Reasoning != "") {
		return fmt.Errorf("a stream that showed nothing came back with %+v", s.resp)
	}
	return nil
}

func (s *streamStallState) theStubServerReceivedRequests(n int) error {
	if got := int(s.requests.Load()); got != n {
		return fmt.Errorf("upstream requests = %d, want %d", got, n)
	}
	return nil
}

func (s *streamStallState) theCallSucceedsWithOneToolCall() error {
	if s.callErr != nil {
		return fmt.Errorf("provider call failed: %v", s.callErr)
	}
	if s.resp == nil || len(s.resp.ToolCalls) != 1 {
		return fmt.Errorf("response = %+v, want one tool call", s.resp)
	}
	return nil
}

func initializeStreamStallScenario(sc *godog.ScenarioContext) {
	s := &streamStallState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		s.reset()
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.cleanup()
		return ctx, nil
	})

	sc.Step(`^an "openai" provider with a stream idle timeout of (\d+) ms pointed at a stub server that stalls after text deltas$`, s.aProviderStallingAfterTextDeltas)
	sc.Step(`^an "openai" provider with a stream idle timeout of (\d+) ms pointed at a stub server that sends keep-alive comments after text deltas$`, s.aProviderSendingKeepalivesAfterTextDeltas)
	sc.Step(`^an "openai" provider with a stream idle timeout of (\d+) ms pointed at a stub server that streams tool call arguments for (\d+) ms$`, s.aProviderStreamingToolArguments)
	sc.Step(`^an "openai" provider with a stream idle timeout of (\d+) ms whose upstream stalls once after an empty first frame and then streams a completion$`, s.aProviderStallingOnceAfterEmptyFrame)
	sc.Step(`^a streaming completion is requested$`, s.aStreamingCompletionIsRequested)
	sc.Step(`^the call fails with a stall error that names the idle time$`, s.theCallFailsWithAStallError)
	sc.Step(`^the partial response preserves text "([^"]*)"$`, s.thePartialResponsePreservesText)
	sc.Step(`^no partial response is kept$`, s.noPartialResponseIsKept)
	sc.Step(`^the stub server received (\d+) requests?$`, s.theStubServerReceivedRequests)
	sc.Step(`^the call succeeds with one tool call$`, s.theCallSucceedsWithOneToolCall)
}

func TestLLMStreamStallFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "llm-stream-stall",
		ScenarioInitializer: initializeStreamStallScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/llm_stream_stall.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("LLM stream stall feature suite failed")
	}
}
