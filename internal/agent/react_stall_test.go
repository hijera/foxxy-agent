package agent

// End-to-end coverage for the two ways a saturated provider kills a turn:
// a call that answers nothing at all (waited out and replayed) and a stream that
// stops mid-answer (kept and continued).

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openai/openai-go"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// stallSender records the transient status updates the loop emits while parked.
type stallSender struct {
	resumePermissionSender
	mu      sync.Mutex
	retries []acp.LLMRetryUpdate
}

func (s *stallSender) SendSessionUpdate(_ string, update interface{}) error {
	if u, ok := update.(acp.LLMRetryUpdate); ok {
		s.mu.Lock()
		s.retries = append(s.retries, u)
		s.mu.Unlock()
	}
	return nil
}

func (s *stallSender) waitingPhases() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, u := range s.retries {
		if u.Phase == acp.LLMRetryPhaseWaiting {
			n++
		}
	}
	return n
}

// stallBehaviour is how the fake provider answers one call.
type stallBehaviour struct {
	// silent blocks until the agent cancels, having delivered nothing - the
	// first-token watchdog's case.
	silent bool
	// partial streams this text and then blocks until cancelled: the mid-answer
	// stall.
	partial string
	// partialReason streams this reasoning and then blocks until cancelled: the
	// mid-answer stall of a model that had not started writing its answer yet.
	partialReason string
	// progressOnly streams this many Progress chunks, spaced apart, before
	// answering: a model writing one large tool call, invisible to the caller.
	progressOnly int
	// answer is streamed and returned normally.
	answer string
	// deltaThenErr streams this text and then fails: a transport that dies
	// mid-stream, which returns no response even though the user saw the text.
	deltaThenErr string
	// err fails the call outright without delivering anything.
	err error
	// cancelWithToolCall streams deltaThenErr and then returns a half-written tool
	// call alongside context.Canceled - what the stream readers hand back when a
	// cancelled stream is finalized mid-arguments.
	cancelWithToolCall bool
}

// stallProvider simulates the reported hub. Each entry of script says how the
// next call behaves; calls past the end repeat the last entry.
type stallProvider struct {
	script []stallBehaviour

	mu    sync.Mutex
	calls int
	seen  [][]llm.Message
}

func (p *stallProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, fmt.Errorf("Complete must not be used by the stall suite")
}

func (p *stallProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// request returns the messages of the nth call, 1-based, for the tests that care
// which attempt carried which nudge.
func (p *stallProvider) request(n int) []llm.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n < 1 || n > len(p.seen) {
		return nil
	}
	return p.seen[n-1]
}

func (p *stallProvider) lastMessages() []llm.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.seen) == 0 {
		return nil
	}
	return p.seen[len(p.seen)-1]
}

func (p *stallProvider) Stream(ctx context.Context, messages []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.seen = append(p.seen, append([]llm.Message(nil), messages...))
	p.mu.Unlock()

	b := p.script[len(p.script)-1]
	if n-1 < len(p.script) {
		b = p.script[n-1]
	}

	switch {
	case b.cancelWithToolCall:
		onChunk(llm.StreamChunk{TextDelta: b.deltaThenErr})
		return &llm.Response{
			Content: b.deltaThenErr,
			ToolCalls: []llm.ToolCall{
				{ID: "call_cut", Name: "read", InputJSON: `{"path":"a.g`},
			},
		}, context.Canceled

	case b.deltaThenErr != "":
		onChunk(llm.StreamChunk{TextDelta: b.deltaThenErr})
		return nil, b.err

	case b.err != nil:
		return nil, b.err

	case b.silent:
		<-ctx.Done()
		return nil, ctx.Err()

	case b.partial != "":
		onChunk(llm.StreamChunk{TextDelta: b.partial})
		<-ctx.Done()
		return &llm.Response{Content: b.partial}, ctx.Err()

	case b.partialReason != "":
		onChunk(llm.StreamChunk{ReasoningDelta: b.partialReason})
		<-ctx.Done()
		return &llm.Response{}, ctx.Err()

	case b.progressOnly > 0:
		for i := 0; i < b.progressOnly; i++ {
			select {
			case <-ctx.Done():
				return &llm.Response{}, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			onChunk(llm.StreamChunk{Progress: true})
		}
		onChunk(llm.StreamChunk{TextDelta: b.answer})
		return &llm.Response{Content: b.answer, StopReason: "end_turn"}, nil

	default:
		onChunk(llm.StreamChunk{TextDelta: b.answer})
		return &llm.Response{Content: b.answer, StopReason: "end_turn"}, nil
	}
}

// stallHarness builds an agent whose guards trip in milliseconds, so the tests
// exercise the real timers without waiting out a real schedule.
type stallHarness struct {
	ag       *Agent
	st       *session.State
	provider *stallProvider
	sender   *stallSender
}

func newStallHarness(t *testing.T, provider *stallProvider, tune func(*config.Agent)) *stallHarness {
	t.Helper()
	cwd, err := os.MkdirTemp("", "foxxycode-stall-cwd-*")
	if err != nil {
		t.Fatal(err)
	}
	sessionDir, err := os.MkdirTemp("", "foxxycode-stall-sess-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(cwd)
		_ = os.RemoveAll(sessionDir)
	})

	firstToken := 60
	stall := 60
	maxWait := 5000
	agentCfg := config.Agent{
		Model:                  "fake/model",
		MaxTurns:               12,
		LLMFirstTokenTimeoutMS: &firstToken,
		LLMStallTimeoutMS:      &stall,
		LLMStallRetryDelaysMS:  []int{1, 2},
		LLMStallRetryMaxWaitMS: &maxWait,
	}
	if tune != nil {
		tune(&agentCfg)
	}

	h := &stallHarness{
		provider: provider,
		sender:   &stallSender{},
		st: &session.State{
			ID:         "sess_stall",
			CWD:        cwd,
			Mode:       session.ModeAgent,
			SessionDir: sessionDir,
		},
	}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     agentCfg,
	}
	h.ag = NewAgent(cfg, h.st, h.sender, nil)
	h.ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	return h
}

func (h *stallHarness) run(t *testing.T) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: "do the thing"}})
}

func (h *stallHarness) assistantMessages() []llm.Message {
	var out []llm.Message
	for _, m := range h.st.GetMessages() {
		if m.Role == llm.RoleAssistant {
			out = append(out, m)
		}
	}
	return out
}

// TestSilentProviderIsWaitedOutAndRetried is the reported bug: the model answers
// nothing, and instead of a red SYSTEM row the turn pauses and tries again.
func TestSilentProviderIsWaitedOutAndRetried(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{silent: true},
		{answer: "Here is the answer."},
	}}
	h := newStallHarness(t, p, nil)

	stop, err := h.run(t)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if got := p.callCount(); got != 2 {
		t.Errorf("provider called %d times, want 2 (one silent, one answering)", got)
	}
	if got := h.sender.waitingPhases(); got != 1 {
		t.Errorf("emitted %d waiting phases, want 1", got)
	}
	msgs := h.assistantMessages()
	if len(msgs) != 1 || msgs[0].Content != "Here is the answer." {
		t.Errorf("transcript = %+v, want exactly the answer", msgs)
	}
}

// TestSilentRetryDoesNotConsumeReactTurns pins the loop-bound arithmetic: a call
// that produced nothing is not a reasoning step, so it must not eat max_turns.
func TestSilentRetryDoesNotConsumeReactTurns(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{silent: true}, {silent: true}, {silent: true},
		{answer: "Finally."},
	}}
	h := newStallHarness(t, p, func(c *config.Agent) { c.MaxTurns = 1 })

	stop, err := h.run(t)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn with max_turns 1 and three retries", stop)
	}
	if got := p.callCount(); got != 4 {
		t.Errorf("provider called %d times, want 4", got)
	}
}

// TestSilentRetryDisabledKeepsTheOldError is the settings switch: off restores
// today's behaviour exactly, including the message wording.
func TestSilentRetryDisabledKeepsTheOldError(t *testing.T) {
	off := false
	p := &stallProvider{script: []stallBehaviour{{silent: true}}}
	h := newStallHarness(t, p, func(c *config.Agent) { c.LLMStallRetry = &off })

	stop, err := h.run(t)
	if err == nil {
		t.Fatal("expected the turn to fail when the retry is switched off")
	}
	if stop != string(acp.StopReasonRefused) {
		t.Errorf("stop reason = %q, want agent_refused", stop)
	}
	if !strings.Contains(err.Error(), "model did not respond") {
		t.Errorf("error = %q, want the original wording", err)
	}
	if strings.Contains(err.Error(), "gave up after") {
		t.Errorf("error = %q, must not mention retries when none were attempted", err)
	}
	if got := p.callCount(); got != 1 {
		t.Errorf("provider called %d times, want 1", got)
	}
}

// TestSilentRetryGivesUpWithinBudget bounds the wait: once the budget is spent
// the user gets the error, and it says why it took so long.
func TestSilentRetryGivesUpWithinBudget(t *testing.T) {
	budget := 4
	p := &stallProvider{script: []stallBehaviour{{silent: true}}}
	h := newStallHarness(t, p, func(c *config.Agent) {
		c.LLMStallRetryDelaysMS = []int{2}
		c.LLMStallRetryMaxWaitMS = &budget
	})

	_, err := h.run(t)
	if err == nil {
		t.Fatal("expected the turn to fail once the retry budget was spent")
	}
	if !strings.Contains(err.Error(), "gave up after 2 retries") {
		t.Errorf("error = %q, want it to name the retries it spent", err)
	}
	if got := p.callCount(); got != 3 {
		t.Errorf("provider called %d times, want 3 (initial plus two retries)", got)
	}
}

// TestRetryableProviderErrorIsWaitedOut covers the other half of a saturated
// gateway: a 503 that internal/llm has already exhausted its own retries on.
func TestRetryableProviderErrorIsWaitedOut(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{err: fmt.Errorf("openai stream: 503 Service Unavailable")},
		{answer: "Recovered."},
	}}
	h := newStallHarness(t, p, nil)

	stop, err := h.run(t)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if got := p.callCount(); got != 2 {
		t.Errorf("provider called %d times, want 2", got)
	}
}

// TestStalledStreamKeepsPartialAndContinues is the mid-answer case: the text the
// user already watched arrive survives, and the model is asked to carry on.
func TestStalledStreamKeepsPartialAndContinues(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{partial: "The first half of the answer."},
		{answer: " And the rest."},
	}}
	h := newStallHarness(t, p, nil)

	stop, err := h.run(t)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}

	msgs := h.assistantMessages()
	if len(msgs) != 2 {
		t.Fatalf("transcript holds %d assistant messages, want 2 (partial then continuation)", len(msgs))
	}
	if msgs[0].Content != "The first half of the answer." {
		t.Errorf("partial answer = %q, want it preserved verbatim", msgs[0].Content)
	}
	if len(msgs[0].ToolCalls) != 0 {
		t.Errorf("a stalled message must not carry tool calls, got %+v", msgs[0].ToolCalls)
	}

	// The continuation prompt is LLM-facing only.
	var nudged bool
	for _, m := range p.lastMessages() {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, "cut off part-way through") {
			nudged = true
		}
	}
	if !nudged {
		t.Error("the second request carried no continuation nudge")
	}
	for _, m := range h.st.GetMessages() {
		if strings.Contains(m.Content, "cut off part-way through") {
			t.Error("the continuation nudge leaked into the persisted transcript")
		}
	}
}

// TestStallWatchdogRearmsOnProgressOnlyFrames is the 984-frame regression: a
// model writing one large tool call delivers nothing the caller can see, and
// must not be mistaken for a dead connection.
func TestStallWatchdogRearmsOnProgressOnlyFrames(t *testing.T) {
	// Twenty frames at 10ms each is 200ms of stream against a 60ms guard: without
	// the progress signal re-arming it, this is cut long before the answer.
	p := &stallProvider{script: []stallBehaviour{
		{progressOnly: 20, answer: "Tool call finished."},
	}}
	h := newStallHarness(t, p, nil)

	stop, err := h.run(t)
	if err != nil {
		t.Fatalf("a healthy stream was cut: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if got := p.callCount(); got != 1 {
		t.Errorf("provider called %d times, want 1 (no retry of a healthy stream)", got)
	}
	if got := h.sender.waitingPhases(); got != 0 {
		t.Errorf("a healthy stream emitted %d waiting phases, want 0", got)
	}
}

// TestStallContinuationIsBounded stops a provider that never finishes from
// looping forever: the partial answer stays, and the turn ends with a notice.
func TestStallContinuationIsBounded(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{{partial: "Half an answer."}}}
	h := newStallHarness(t, p, nil)

	stop, err := h.run(t)
	if err == nil {
		t.Fatal("expected the turn to stop once the continuation budget ran out")
	}
	if stop != string(acp.StopReasonRefused) {
		t.Errorf("stop reason = %q, want agent_refused", stop)
	}
	if !strings.Contains(err.Error(), "stopped sending data mid-answer") {
		t.Errorf("error = %q, want it to name the provider as the cause", err)
	}
	if got := p.callCount(); got != maxStallContinuations+1 {
		t.Errorf("provider called %d times, want %d", got, maxStallContinuations+1)
	}
	if len(h.assistantMessages()) == 0 {
		t.Error("the partial answer must survive in the transcript")
	}
}

// TestSilentRetryStopsImmediatelyOnCancel is the Stop path: the user must not
// have to sit out the pause.
func TestSilentRetryStopsImmediatelyOnCancel(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{{silent: true}}}
	h := newStallHarness(t, p, func(c *config.Agent) {
		c.LLMStallRetryDelaysMS = []int{int(time.Hour / time.Millisecond)}
		zero := 0
		c.LLMStallRetryMaxWaitMS = &zero // unbounded, so only the cancel can end it
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var stop string
	var err error
	go func() {
		defer close(done)
		stop, err = h.ag.Run(ctx, []acp.ContentBlock{{Type: "text", Text: "do the thing"}})
	}()

	// Let the watchdog trip and the pause begin, then stop the turn.
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the turn was cancelled during the retry pause")
	}
	if err != nil {
		t.Errorf("cancelled turn returned an error: %v", err)
	}
	if stop != string(acp.StopReasonCancelled) {
		t.Errorf("stop reason = %q, want cancelled", stop)
	}
}

// TestReportedProviderFailuresAreRetried pins the exact error shapes seen against
// api.neuraldeep.ru. Both end a turn today; both are safe to replay because
// nothing reached the transcript.
func TestReportedProviderFailuresAreRetried(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "unexpected EOF",
			err: fmt.Errorf("openai stream: %w", &url.Error{
				Op:  "Post",
				URL: "https://api.neuraldeep.ru/v1/chat/completions",
				Err: fmt.Errorf("unexpected EOF"),
			}),
		},
		{
			// internal/llm refuses this one on its own, because from inside a
			// provider a deadline cannot be told from the caller giving up.
			name: "client timeout while reading body",
			err: fmt.Errorf("openai stream: %w", &url.Error{
				Op:  "Post",
				URL: "https://api.neuraldeep.ru/v1/chat/completions",
				Err: fmt.Errorf("context deadline exceeded (Client.Timeout or context cancellation while reading body): %w", context.DeadlineExceeded),
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &stallProvider{script: []stallBehaviour{
				{err: tc.err},
				{answer: "Recovered."},
			}}
			h := newStallHarness(t, p, nil)

			stop, err := h.run(t)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if stop != string(acp.StopReasonEndTurn) {
				t.Errorf("stop reason = %q, want end_turn", stop)
			}
			if got := p.callCount(); got != 2 {
				t.Errorf("provider called %d times, want 2 (one failure, one answer)", got)
			}
		})
	}
}

// TestRefusedRequestIsNotRetried keeps a bad key or an unknown model from burning
// the whole retry budget: a request the endpoint rejected will be rejected again.
func TestRefusedRequestIsNotRetried(t *testing.T) {
	// The shape the SDK actually returns: a typed error carrying the status, which
	// is what llm.HTTPStatus reads. A 4xx is a verdict on the request, not on the
	// endpoint's health.
	p := &stallProvider{script: []stallBehaviour{
		{err: fmt.Errorf("openai stream: %w", &openai.Error{StatusCode: 401})},
	}}
	h := newStallHarness(t, p, nil)

	_, err := h.run(t)
	if err == nil {
		t.Fatal("expected a rejected request to end the turn")
	}
	if got := p.callCount(); got != 1 {
		t.Errorf("provider called %d times, want 1 (a refusal must not be retried)", got)
	}
}

// TestTransportFailureAfterDeltasIsNotReplayed is the duplication guard. A
// transport failure mid-stream returns no response at all, yet the deltas it
// managed to send are already on the user's screen: replaying would show the
// same text twice.
func TestTransportFailureAfterDeltasIsNotReplayed(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{deltaThenErr: "Half a sentence", err: fmt.Errorf("openai stream: unexpected EOF")},
		{answer: "Recovered."},
	}}
	h := newStallHarness(t, p, nil)

	_, err := h.run(t)
	if err == nil {
		t.Fatal("expected the turn to surface the transport error rather than replay it")
	}
	if got := p.callCount(); got != 1 {
		t.Errorf("provider called %d times, want 1 (delivered output must never be replayed)", got)
	}
}

// TestStallContinuationDoesNotConsumeReactTurns is the twin of
// TestSilentRetryDoesNotConsumeReactTurns for the other provider failure. A
// connection that died mid-answer is not a reasoning step the model chose, so
// carrying on must not shrink max_turns either.
func TestStallContinuationDoesNotConsumeReactTurns(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{partial: "The first half."},
		{partial: " A bit more."},
		{answer: " And the rest."},
	}}
	h := newStallHarness(t, p, func(c *config.Agent) { c.MaxTurns = 1 })

	stop, err := h.run(t)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn with max_turns 1 and two stalls", stop)
	}
	if got := p.callCount(); got != 3 {
		t.Errorf("provider called %d times, want 3", got)
	}
}

// hasNudge reports whether one request carried an LLM-facing user message with
// this text. Every nudge in the ReAct loop travels that way and none of them are
// persisted, so the request slice is the only place to look for them.
func hasNudge(msgs []llm.Message, want string) bool {
	for _, m := range msgs {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, want) {
			return true
		}
	}
	return false
}

// TestRepeatedRestartTellsTheModelItIsRepeating is the reported bug. A hub that
// keeps dropping the same answer used to get the same polite "carry on" every
// time, and the model answered it by starting over - the operator watched the
// same opening paragraph arrive every few minutes. The second time an opening
// comes back, the model is told so instead.
func TestRepeatedRestartTellsTheModelItIsRepeating(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{{partial: "I will fix the compile errors. First the constants."}}}
	h := newStallHarness(t, p, nil)

	_, err := h.run(t)
	if err == nil {
		t.Fatal("expected the turn to stop once the continuation budget ran out")
	}
	if got := p.callCount(); got != maxStallContinuations+1 {
		t.Fatalf("provider called %d times, want %d", got, maxStallContinuations+1)
	}

	// The first continuation cannot know it is a repeat yet.
	if !hasNudge(p.request(2), "cut off part-way through") {
		t.Error("the first continuation did not carry the plain stall nudge")
	}
	// The second one does: the same opening came back.
	if !hasNudge(p.request(3), "begins the same way") {
		t.Error("the second continuation did not tell the model it was repeating itself")
	}
	for _, m := range h.st.GetMessages() {
		if strings.Contains(m.Content, "begins the same way") {
			t.Error("the repeat nudge leaked into the persisted transcript")
		}
	}
	if !strings.Contains(err.Error(), "restarted the same answer") {
		t.Errorf("error = %q, want it to name the restarts as well as the provider", err)
	}
	if !strings.Contains(err.Error(), "stopped sending data mid-answer") {
		t.Errorf("error = %q, want the provider still named as the cause", err)
	}
}

// TestRepeatedRestartAsksForAnAnswerWithToolsWithheld is the escalation: a model
// that keeps restarting is eventually made to answer from what it has, the same
// way a model with nothing left but a quarantined loop is.
func TestRepeatedRestartAsksForAnAnswerWithToolsWithheld(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{{partial: "I will fix the compile errors."}}}
	h := newStallHarness(t, p, nil)

	if _, err := h.run(t); err == nil {
		t.Fatal("expected the turn to stop once the continuation budget ran out")
	}
	if !hasNudge(p.request(4), "No tools are available on this request") {
		t.Error("the third continuation did not take the tools away and ask for the answer")
	}
}

// TestReasoningOnlyStallIsNotAskedToContinueFromNothing covers the partial that
// had no answer text: only reasoning got through, and an OpenAI-compatible
// endpoint replays that as an empty assistant message. "Continue from exactly
// where it stops" then points at nothing, which is what makes the model start
// over.
func TestReasoningOnlyStallIsNotAskedToContinueFromNothing(t *testing.T) {
	p := &stallProvider{script: []stallBehaviour{
		{partialReason: "Let me work out which constants are wrong."},
		{answer: "The constants are fixed."},
	}}
	h := newStallHarness(t, p, nil)

	stop, err := h.run(t)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if hasNudge(p.request(2), "Continue from exactly where it stops") {
		t.Error("a reasoning-only stall was told to continue from an empty message")
	}
	if !hasNudge(p.request(2), "only your internal reasoning") {
		t.Error("a reasoning-only stall did not get its own nudge")
	}
}
