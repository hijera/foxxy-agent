package agent

// Live probes for the cross-attempt repeat guard (attempt_repeat.go), against a
// real OpenAI-compatible endpoint. Skipped unless FOXXYCODE_LIVE_MODEL and
// NEURALDEEP_API_KEY are set, like live_neuraldeep_test.go whose helpers these
// reuse, so the normal suite stays deterministic and offline.
//
//	FOXXYCODE_LIVE_MODEL=qwen3.6-35b-a3b-noreason NEURALDEEP_API_KEY=... \
//	  go test ./internal/agent -run TestLiveStall -count=1 -v -timeout 900s
//
// The stub suites already prove the wiring. What only a real model can answer is
// whether the nudges change its behaviour: streamStallNudge asks it to carry on,
// and the whole bug is that it starts the answer over instead.

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// liveStallTask is the shape of turn this bug shows up in: a multi-step fix the
// model opens by restating the plan, which is exactly what it rewrites when a
// continuation is misread as a fresh start.
const liveStallTask = "You are fixing compilation errors in a Java service. " +
	"There are four: wrong constant names (ATS_PARTICIPANT_CODE_FIELD, ATS_PERSON_ID_FIELD), " +
	"personsRepository returning the Persons entity rather than Person, " +
	"InfoFieldType having no constructor with arguments, and MsgInFields being an entity " +
	"rather than common.MsgInFields. Walk through the fixes one at a time."

// liveStallPartial is what the transcript holds after the stall guard cut the
// stream: the opening of that answer, trimmed at a line boundary the way
// trimToResumeBoundary leaves it.
const liveStallPartial = "Understood. I will fix the compilation errors. The main problems:\n" +
	"1. Wrong constant names (ATS_PARTICIPANT_CODE_FIELD, ATS_PERSON_ID_FIELD)\n" +
	"2. personsRepository returns the Persons entity, not Person\n" +
	"3. InfoFieldType has no constructor with arguments\n"

// liveStallRounds is how many times each nudge is measured. A model is not a pure
// function; one sample proves nothing either way.
func liveStallRounds() int {
	if v := strings.TrimSpace(os.Getenv("FOXXYCODE_LIVE_ROUNDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 3
}

func liveStallProvider(t *testing.T, model, key string, maxTokens int) llm.Provider {
	t.Helper()
	p, err := llm.NewProvider(llm.ProviderInput{
		Type: "neuraldeep", Model: model, APIKey: key,
		MaxTokens: maxTokens, Temperature: 0.2,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return p
}

// liveRestarted reports whether an answer is the partial being written again,
// judged by the very predicate the agent judges it with. Reusing sameAttempt is
// the point: a probe with its own idea of "the same answer" would measure
// something the guard does not act on.
func liveRestarted(partial, answer string) bool {
	return sameAttempt(
		attemptFingerprint("", partial, nil),
		attemptFingerprint("", answer, nil),
	)
}

// liveSkipOnHubOutage separates "the hub is having a bad minute" from "the endpoint
// refused what we sent". The classifier is the production one: stallRetryableError
// is what the agent itself uses to decide a failure is worth waiting out, so a
// 5xx, a dropped connection or a header timeout skips the probe, while a 4xx -
// a payload this endpoint will not accept, which is exactly the kind of regression
// a live probe exists to catch - fails it.
func liveSkipOnHubOutage(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if stallRetryableError(err) {
		t.Skipf("endpoint unavailable, nothing measured: %v", err)
	}
	t.Fatalf("live call rejected: %v", err)
}

func liveHead(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 90 {
		return s[:90]
	}
	return s
}

// TestLiveStallNudgeChangesTheAnswer measures the assumption the fix rests on: after
// a dropped stream, does the model carry on, or write its opening again - and does
// naming the repeat change that?
func TestLiveStallNudgeChangesTheAnswer(t *testing.T) {
	model, key := liveSkipUnlessConfigured(t)
	// The provider takes the bare model id; the "neuraldeep/" prefix is the config-level
	// id that the agent strips before it builds a ProviderInput.
	provider := liveStallProvider(t, model, key, 1200)

	ask := func(nudge string) string {
		msgs := []llm.Message{
			{Role: llm.RoleSystem, Content: "You are a coding agent. Be concise."},
			{Role: llm.RoleUser, Content: liveStallTask},
			{Role: llm.RoleAssistant, Content: liveStallPartial},
			{Role: llm.RoleUser, Content: nudge},
		}
		resp, err := provider.Complete(context.Background(), msgs, nil)
		liveSkipOnHubOutage(t, err)
		return resp.Content
	}

	rounds := liveStallRounds()
	repeatNudge := repeatedAttemptNudge([]string{"read(PersonService.java)", "grep(ATS_PERSON_ID_FIELD)"}, 2)

	politeRestarts, repeatRestarts := 0, 0
	for i := 0; i < rounds; i++ {
		polite := ask(streamStallNudge)
		politeAgain := liveRestarted(liveStallPartial, polite)
		if politeAgain {
			politeRestarts++
		}
		t.Logf("round %d streamStallNudge  restarted=%v head=%q", i+1, politeAgain, liveHead(polite))

		repeat := ask(repeatNudge)
		repeatAgain := liveRestarted(liveStallPartial, repeat)
		if repeatAgain {
			repeatRestarts++
		}
		t.Logf("round %d repeatedAttempt   restarted=%v head=%q", i+1, repeatAgain, liveHead(repeat))
	}

	t.Logf("restarted the answer: streamStallNudge %d/%d, repeatedAttemptNudge %d/%d",
		politeRestarts, rounds, repeatRestarts, rounds)
	if repeatRestarts > politeRestarts {
		t.Errorf("the repeat nudge restarts the answer more often than the polite one (%d vs %d of %d): "+
			"it is meant to stop the restart, not provoke it", repeatRestarts, politeRestarts, rounds)
	}
}

// TestLiveStallEmptyPartialRepeat measures the bug in the shape the operator
// actually hits it.
//
// A reasoning model that stalls before its first line of answer leaves a partial
// with no content at all, and this endpoint drops reasoning_content on the way in
// (verified: 1039 characters of it add zero prompt tokens). So the continuation
// request carries empty assistant turns and a nudge saying "carry on from the
// message above" - and the model, having nothing above, writes its opening again.
// Every attempt, for as long as the budget lasts.
//
// The probe reproduces that request exactly and asks whether naming the repeat
// breaks it: does the answer still open the way a fresh attempt opens?
func TestLiveStallEmptyPartialRepeat(t *testing.T) {
	model, key := liveSkipUnlessConfigured(t)
	provider := liveStallProvider(t, model, key, 4000)

	ask := func(tail ...llm.Message) string {
		msgs := append([]llm.Message{
			{Role: llm.RoleSystem, Content: "You are a coding agent. Be concise."},
			{Role: llm.RoleUser, Content: liveStallTask},
		}, tail...)
		resp, err := provider.Complete(context.Background(), msgs, nil)
		liveSkipOnHubOutage(t, err)
		return resp.Content
	}

	// What two stalls leave behind: one assistant message per cut answer, both
	// empty by the time the endpoint has stripped the reasoning off them.
	stalled := []llm.Message{
		{Role: llm.RoleAssistant, Content: "", Reasoning: "The user wants four compilation errors fixed. I will start with the constant names."},
		{Role: llm.RoleAssistant, Content: "", Reasoning: "The user wants four compilation errors fixed. I will start with the constant names."},
	}
	repeatNudge := repeatedAttemptNudge([]string{"read(PersonService.java)", "grep(ATS_PERSON_ID_FIELD)"}, 2)

	rounds := liveStallRounds()
	oldRepeats, newRepeats := 0, 0
	for i := 0; i < rounds; i++ {
		// The opening a fresh attempt produces is the one every stalled attempt keeps
		// rewriting, so it is what a continuation must not reproduce.
		baseline := ask()
		t.Logf("round %d baseline opening       head=%q", i+1, liveHead(baseline))

		old := ask(append(append([]llm.Message(nil), stalled...),
			llm.Message{Role: llm.RoleUser, Content: streamStallNudge})...)
		oldSame := liveRestarted(baseline, old)
		if oldSame {
			oldRepeats++
		}
		t.Logf("round %d streamStallNudge     repeats the opening=%v head=%q", i+1, oldSame, liveHead(old))

		fixed := ask(append(append([]llm.Message(nil), stalled...),
			llm.Message{Role: llm.RoleUser, Content: repeatNudge})...)
		newSame := liveRestarted(baseline, fixed)
		if newSame {
			newRepeats++
		}
		t.Logf("round %d repeatedAttemptNudge repeats the opening=%v head=%q", i+1, newSame, liveHead(fixed))
	}

	t.Logf("opening rewritten: streamStallNudge %d/%d, repeatedAttemptNudge %d/%d",
		oldRepeats, rounds, newRepeats, rounds)
	if newRepeats > oldRepeats {
		t.Errorf("the repeat nudge rewrites the opening more often than the polite one (%d vs %d of %d)",
			newRepeats, oldRepeats, rounds)
	}
}

// liveCuttingProvider delivers the first cutAfter bytes of a real stream and then
// goes quiet, which is what the hub does to roughly half of its long generations.
// The agent's own idle watchdog is what notices; nothing here fakes an error.
type liveCuttingProvider struct {
	inner llm.Provider
	// cutAfter is a budget of delivered answer/reasoning bytes, cutFrames one of
	// delivered frames of any kind. Both are needed: a stream carrying only
	// tool-call arguments delivers no text at all - those deltas are accumulated
	// inside the provider and never reach onChunk - so a byte budget alone would
	// never fire on a turn that is calling tools, which is most of them.
	cutAfter  int
	cutFrames int
	cuts      *int32
	requests  *[][]llm.Message
}

func (p *liveCuttingProvider) Complete(ctx context.Context, m []llm.Message, t []llm.ToolDefinition) (*llm.Response, error) {
	*p.requests = append(*p.requests, append([]llm.Message(nil), m...))
	return p.inner.Complete(ctx, m, t)
}

func (p *liveCuttingProvider) Stream(ctx context.Context, m []llm.Message, t []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	*p.requests = append(*p.requests, append([]llm.Message(nil), m...))
	delivered, frames := 0, 0
	silent := false
	return p.inner.Stream(ctx, m, t, func(c llm.StreamChunk) {
		if silent {
			return
		}
		delivered += len(c.TextDelta) + len(c.ReasoningDelta)
		frames++
		onChunk(c)
		if delivered >= p.cutAfter || frames >= p.cutFrames {
			silent = true
			atomic.AddInt32(p.cuts, 1)
		}
	})
}

// TestLiveStallRepeatDrivesTheLoop runs the real ReAct loop against the real
// endpoint with every stream cut mid-answer, and checks the loop reacts the way
// the design says: it continues rather than dying, it escalates from the polite
// nudge to the repeat nudge once the same opening comes back, and it terminates.
func TestLiveStallRepeatDrivesTheLoop(t *testing.T) {
	model, key := liveSkipUnlessConfigured(t)

	cwd := t.TempDir()
	stallMS, firstTokenMS, retryWait := 2000, 60000, 2000
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "neuraldeep", Type: "neuraldeep", APIKey: key}},
		Models: []config.ModelEntry{{
			Model: "neuraldeep/" + model, MaxTokens: 1200, MaxContextTokens: 128000, Temperature: 0.2,
		}},
		Agent: config.Agent{
			Model: "neuraldeep/" + model, MaxTurns: 4,
			LLMStallTimeoutMS:      &stallMS,
			LLMFirstTokenTimeoutMS: &firstTokenMS,
			LLMStallRetryDelaysMS:  []int{200},
			LLMStallRetryMaxWaitMS: &retryWait,
		},
		Tools: config.Tools{PermissionMode: config.PermModeBypass},
	}

	st := &session.State{ID: "sess_live_stall", CWD: cwd, Mode: session.ModeAgent}
	ag := NewAgent(cfg, st, resumePermissionSender{}, nil)

	var cuts int32
	var requests [][]llm.Message
	ag.providerFactory = func(in llm.ProviderInput) (llm.Provider, error) {
		inner, err := llm.NewProvider(in)
		if err != nil {
			return nil, err
		}
		return &liveCuttingProvider{inner: inner, cutAfter: 120, cutFrames: 12, cuts: &cuts, requests: &requests}, nil
	}

	stop, runErr := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text",
		Text: liveStallTask + " Answer in prose, walking through each fix in turn, and do not call any tools."}})

	polite, noText, repeat := 0, 0, 0
	for i, req := range requests {
		last := ""
		if len(req) > 0 {
			last = req[len(req)-1].Content
		}
		switch {
		case last == streamStallNudge:
			polite++
			t.Logf("request %d carried streamStallNudge", i+1)
		case last == stallNoTextNudge:
			noText++
			t.Logf("request %d carried stallNoTextNudge", i+1)
		case strings.Contains(last, "begins the same way"):
			repeat++
			t.Logf("request %d carried the repeat nudge: %q", i+1, liveHead(last))
		}
	}
	t.Logf("stop=%s err=%v cuts=%d requests=%d (polite=%d no-text=%d repeat=%d)",
		stop, runErr, atomic.LoadInt32(&cuts), len(requests), polite, noText, repeat)

	if atomic.LoadInt32(&cuts) == 0 {
		t.Skip("the model answered inside the cut window; nothing was stalled")
	}
	if len(requests) < 2 {
		t.Fatalf("a cut stream did not produce a continuation request: %d request(s)", len(requests))
	}
	if polite+noText+repeat == 0 {
		// Legitimate: a cut that left neither text nor reasoning has nothing to carry
		// on from, so the loop replays the identical request rather than nudging.
		t.Log("no continuation nudge: every cut landed before anything was delivered, so the calls were replayed")
	}
	// Whether the model repeats itself is the model's business - with visible text to
	// carry on from it usually does not. What the loop owes is conditional: if two
	// attempts did open the same way, the polite "carry on" must have given way to
	// the nudge that names the repeat. Judged with the production predicate over what
	// the attempts actually persisted.
	var openings []string
	repeatedItself := false
	for _, m := range st.GetMessages() {
		if m.Role != llm.RoleAssistant {
			continue
		}
		fp := attemptFingerprint(m.Reasoning, m.Content, m.ToolCalls)
		if fp == "" {
			continue
		}
		for _, prev := range openings {
			if sameAttempt(prev, fp) {
				repeatedItself = true
			}
		}
		openings = append(openings, fp)
	}
	t.Logf("attempts persisted=%d repeated=%v", len(openings), repeatedItself)
	if repeatedItself && repeat == 0 {
		t.Errorf("two attempts opened the same way and the repeat nudge never fired")
	}
	if !repeatedItself {
		t.Log("the model carried on rather than restarting; the repeat guard had nothing to fire on")
	}
}
