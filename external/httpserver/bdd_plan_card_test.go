//go:build http

package httpserver

// Godog harness for features/plan_card_placement.feature: drives a real plan-mode
// turn over POST /v1/responses and checks what the bundled UI needs to render the
// plan card — the design-plan SSE event, the plan_document transcript row, and the
// tools.plan_no_self_run guard.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/agent"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const bddFencedPlan = `---
name: Improve the Qwen prompt
overview: Expand the model-family guidance.
todos:
  - content: Read the current prompt
    status: completed
  - content: Draft the replacement
    status: pending
---
## Summary

Rewrite the prompt.
`

// bddFenceLessPlan is the shape Qwen-family models actually emit: the frontmatter
// keys without the --- fences.
const bddFenceLessPlan = `name: Improve the Qwen prompt
overview: Expand the model-family guidance.
todos:
  - content: Read the current prompt
    status: completed

## Summary

Rewrite the prompt.
`

// planningProvider writes a design plan mid-turn and keeps talking afterwards,
// which is what puts the plan_document row in the middle of the message list.
type planningProvider struct {
	content   string
	leavePlan bool
	calls     int
}

func (p *planningProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "canned", StopReason: "end_turn"}, nil
}

func (p *planningProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	switch p.calls {
	case 1:
		args, err := json.Marshal(map[string]string{"slug": "improve-qwen-prompt", "content": p.content})
		if err != nil {
			return nil, err
		}
		tc := llm.ToolCall{ID: "call_plan_write", Name: "plan_write", InputJSON: string(args)}
		onChunk(llm.StreamChunk{TextDelta: "I'll write the plan now."})
		onChunk(llm.StreamChunk{ToolCall: &tc})
		return &llm.Response{
			Content:    "I'll write the plan now.",
			ToolCalls:  []llm.ToolCall{tc},
			StopReason: "tool_calls",
		}, nil
	case 2:
		if p.leavePlan {
			tc := llm.ToolCall{ID: "call_plan_exit", Name: "plan_exit", InputJSON: "{}"}
			onChunk(llm.StreamChunk{ToolCall: &tc})
			return &llm.Response{ToolCalls: []llm.ToolCall{tc}, StopReason: "tool_calls"}, nil
		}
	}
	const done = "The plan is ready — review it and run it when you are happy."
	onChunk(llm.StreamChunk{TextDelta: done})
	return &llm.Response{Content: done, StopReason: "end_turn"}, nil
}

type planCardFeatureState struct {
	root      string
	ts        *httptest.Server
	mgr       *session.Manager
	srv       *Server
	sessionID string
	sse       string
	cfg       *config.Config
	// A stand-in for the IntelliJ / VS Code plugin: a live /foxxycode/ide/events
	// subscriber whose payloads land in ideEvents.
	ideCancel context.CancelFunc
	ideEvents chan string
}

func (s *planCardFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-plan-card-*")
	if err != nil {
		return err
	}
	s.root = root
	s.sessionID = ""
	s.sse = ""
	s.cfg = nil
	return nil
}

func (s *planCardFeatureState) close() {
	if s.ideCancel != nil {
		s.ideCancel()
		s.ideCancel = nil
	}
	s.ideEvents = nil
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

// The step runs after the server is up, so flip the live config the agent reads
// per turn rather than rebuilding the stack.
func (s *planCardFeatureState) forbidSelfRun() error {
	if s.cfg == nil {
		return fmt.Errorf("server not started")
	}
	on := true
	s.cfg.Tools.PlanNoSelfRun = &on
	return nil
}

func (s *planCardFeatureState) startServer(content string, leavePlan bool) error {
	sessRoot := filepath.Join(s.root, "sessions")
	if err := os.MkdirAll(sessRoot, 0o755); err != nil {
		return err
	}
	cfg := &config.Config{
		Paths:     config.Paths{Home: filepath.Join(s.root, "home"), CWD: s.root},
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100, Temperature: 0.2}},
		Agent:     config.Agent{Model: "fake/model"},
	}
	s.cfg = cfg
	provider := &planningProvider{content: content, leavePlan: leavePlan}
	fakeFactory := func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, snd acp.UpdateSender) (string, error) {
		ag := agent.NewAgent(cfg, st, snd, slog.Default())
		ag.SetProviderFactory(fakeFactory)
		return ag.Run(ctx, prompt)
	}
	store := &session.FileStore{Root: sessRoot}
	s.mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, store)
	s.srv = New(cfg, s.mgr, slog.Default(), s.root)
	s.srv.agentProviderFactory = fakeFactory
	s.ts = httptest.NewServer(s.srv.Handler())
	return nil
}

func (s *planCardFeatureState) startPlanningServer() error {
	return s.startServer(bddFencedPlan, false)
}

func (s *planCardFeatureState) startFenceLessServer() error {
	return s.startServer(bddFenceLessPlan, false)
}

func (s *planCardFeatureState) startLeavingServer() error {
	return s.startServer(bddFencedPlan, true)
}

func (s *planCardFeatureState) sessionInPlanMode() error {
	res, err := s.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.root})
	if err != nil {
		return err
	}
	s.sessionID = res.SessionID
	st := s.mgr.SessionByID(s.sessionID)
	if st == nil {
		return fmt.Errorf("session %q not registered", s.sessionID)
	}
	st.SetMode("plan")
	return nil
}

func (s *planCardFeatureState) askForAPlan() error {
	payload := map[string]interface{}{
		"model":  "plan",
		"input":  "plan the prompt rewrite",
		"stream": true,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, s.ts.URL+"/v1/responses", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /v1/responses status %d", res.StatusCode)
	}
	var sb strings.Builder
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		sb.WriteString(sc.Text())
		sb.WriteByte('\n')
	}
	s.sse = sb.String()
	return sc.Err()
}

func (s *planCardFeatureState) sseCarriesDesignPlan() error {
	if !strings.Contains(s.sse, "event: plan") {
		return fmt.Errorf("no plan event in stream: %s", s.sse)
	}
	if !strings.Contains(s.sse, `"foxxycode.dev/planKind":"design"`) {
		return fmt.Errorf("plan event is not marked as a design plan: %s", s.sse)
	}
	if !strings.Contains(s.sse, `"foxxycode.dev/planSlug":"improve-qwen-prompt"`) {
		return fmt.Errorf("plan event does not carry the slug: %s", s.sse)
	}
	return nil
}

func (s *planCardFeatureState) messages() ([]map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet,
		s.ts.URL+"/foxxycode/sessions/"+s.sessionID+"/messages", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var parsed struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Messages, nil
}

func (s *planCardFeatureState) planDocumentRow() (map[string]interface{}, error) {
	msgs, err := s.messages()
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		pd, ok := m["plan_document"].(map[string]interface{})
		if ok && pd["slug"] == "improve-qwen-prompt" {
			return pd, nil
		}
	}
	return nil, fmt.Errorf("no plan_document row for improve-qwen-prompt in %d messages", len(msgs))
}

func (s *planCardFeatureState) transcriptHasPlanRow() error {
	_, err := s.planDocumentRow()
	return err
}

// The plan row is written mid-turn, so the assistant text that follows it must be
// persisted too — that text is what the card has to render below.
func (s *planCardFeatureState) assistantTextFollowsThePlan() error {
	msgs, err := s.messages()
	if err != nil {
		return err
	}
	planAt := -1
	for i, m := range msgs {
		if _, ok := m["plan_document"].(map[string]interface{}); ok {
			planAt = i
		}
	}
	if planAt < 0 {
		return fmt.Errorf("no plan_document row found")
	}
	for _, m := range msgs[planAt+1:] {
		if m["role"] == "assistant" && strings.Contains(fmt.Sprint(m["content"]), "The plan is ready") {
			return nil
		}
	}
	return fmt.Errorf("assistant text after the plan row is missing")
}

func (s *planCardFeatureState) cardShowsFrontmatterName() error {
	pd, err := s.planDocumentRow()
	if err != nil {
		return err
	}
	if got := fmt.Sprint(pd["name"]); got != "Improve the Qwen prompt" {
		return fmt.Errorf("plan name = %q, want the frontmatter name", got)
	}
	return nil
}

func (s *planCardFeatureState) cardBodyIsMarkdown() error {
	pd, err := s.planDocumentRow()
	if err != nil {
		return err
	}
	body := fmt.Sprint(pd["body"])
	if strings.Contains(body, "overview:") {
		return fmt.Errorf("card body still holds raw frontmatter: %q", body)
	}
	if !strings.Contains(body, "## Summary") {
		return fmt.Errorf("card body lost the markdown section: %q", body)
	}
	return nil
}

func (s *planCardFeatureState) planFileIsFenced() error {
	st := s.mgr.SessionByID(s.sessionID)
	if st == nil {
		return fmt.Errorf("session %q not registered", s.sessionID)
	}
	path := filepath.Join(st.GetPersistedSessionDir(), "plans", "improve-qwen-prompt.plan.md")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(b), "---\n") {
		return fmt.Errorf("plan file does not start with a fence: %q", string(b))
	}
	return nil
}

func (s *planCardFeatureState) sessionModeIs(want string) error {
	st := s.mgr.SessionByID(s.sessionID)
	if st == nil {
		return fmt.Errorf("session %q not registered", s.sessionID)
	}
	if got := st.GetMode(); got != want {
		return fmt.Errorf("session mode = %q, want %q", got, want)
	}
	return nil
}

// ideePluginListening opens the same SSE stream the editor plugins consume and
// buffers every `data:` payload, so the scenario asserts on what a real plugin
// would have received rather than on the in-process hub.
func (s *planCardFeatureState) idePluginListening() error {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.ts.URL+"/foxxycode/ide/events", nil)
	if err != nil {
		cancel()
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		cancel()
		return fmt.Errorf("GET /foxxycode/ide/events status %d", res.StatusCode)
	}
	// The handler primes the stream with a comment, so by the time this returns
	// the subscription is registered and no broadcast can be missed.
	sc := bufio.NewScanner(res.Body)
	if !sc.Scan() {
		res.Body.Close()
		cancel()
		return fmt.Errorf("ide event stream closed before the priming comment")
	}
	lines := make(chan string, 32)
	s.ideEvents = lines
	s.ideCancel = func() {
		cancel()
		res.Body.Close()
	}
	go func() {
		defer close(lines)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			select {
			case lines <- strings.TrimSpace(strings.TrimPrefix(line, "data:")):
			default:
			}
		}
	}()
	return nil
}

func (s *planCardFeatureState) showThePlanInTheIDE() error {
	url := s.ts.URL + "/foxxycode/sessions/" + s.sessionID + "/plans/improve-qwen-prompt/open-in-ide"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST open-in-ide status %d", res.StatusCode)
	}
	var parsed struct {
		Delivered bool `json:"delivered"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return err
	}
	if !parsed.Delivered {
		return fmt.Errorf("server reported no IDE client to deliver to")
	}
	return nil
}

func (s *planCardFeatureState) pluginAskedToOpenPlanFile() error {
	st := s.mgr.SessionByID(s.sessionID)
	if st == nil {
		return fmt.Errorf("session %q not registered", s.sessionID)
	}
	want := filepath.Join(st.GetPersistedSessionDir(), "plans", "improve-qwen-prompt.plan.md")
	deadline := time.After(5 * time.Second)
	for {
		select {
		case payload, ok := <-s.ideEvents:
			if !ok {
				return fmt.Errorf("ide event stream ended without an open_file event")
			}
			var ev struct {
				Type string `json:"type"`
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}
			if ev.Type != "open_file" {
				continue
			}
			if ev.Path != want {
				return fmt.Errorf("open_file path = %q, want %q", ev.Path, want)
			}
			if _, err := os.Stat(ev.Path); err != nil {
				return fmt.Errorf("the plugin was pointed at a file it cannot open: %w", err)
			}
			return nil
		case <-deadline:
			return fmt.Errorf("timed out waiting for an open_file event")
		}
	}
}

func (s *planCardFeatureState) refusalReportedToModel() error {
	msgs, err := s.messages()
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m["role"] != "tool" {
			continue
		}
		if strings.Contains(fmt.Sprint(m["content"]), "not available in plan mode") {
			return nil
		}
	}
	return fmt.Errorf("no refusal tool result in the transcript")
}

func initializePlanCardScenario(sc *godog.ScenarioContext) {
	s := &planCardFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server with a planning agent$`, s.startPlanningServer)
	sc.Step(`^a running foxxycode HTTP server with a planning agent that omits frontmatter fences$`, s.startFenceLessServer)
	sc.Step(`^a running foxxycode HTTP server with a planning agent that leaves plan mode$`, s.startLeavingServer)
	sc.Step(`^the model is forbidden from running the plan itself$`, s.forbidSelfRun)
	sc.Step(`^an HTTP session in plan mode$`, s.sessionInPlanMode)
	sc.Step(`^an editor plugin listening on the IDE event stream$`, s.idePluginListening)

	sc.Step(`^the user asks for a plan$`, s.askForAPlan)
	sc.Step(`^the user shows the plan in the IDE$`, s.showThePlanInTheIDE)
	sc.Step(`^the editor plugin is asked to open the plan file$`, s.pluginAskedToOpenPlanFile)

	sc.Step(`^the turn streams a design plan event carrying the plan slug$`, s.sseCarriesDesignPlan)
	sc.Step(`^the session transcript contains a plan document row for that slug$`, s.transcriptHasPlanRow)
	sc.Step(`^the assistant text of that turn is persisted after the plan was written$`, s.assistantTextFollowsThePlan)
	sc.Step(`^the plan card shows the plan name from the frontmatter$`, s.cardShowsFrontmatterName)
	sc.Step(`^the plan card body holds markdown rather than raw frontmatter$`, s.cardBodyIsMarkdown)
	sc.Step(`^the plan file on disk starts with a frontmatter fence$`, s.planFileIsFenced)
	sc.Step(`^the session mode is "([^"]+)"$`, s.sessionModeIs)
	sc.Step(`^the refused tool is reported back to the model$`, s.refusalReportedToModel)
}

func TestPlanCardPlacementFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "plan-card-placement",
		ScenarioInitializer: initializePlanCardScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/plan_card_placement.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("plan card placement feature suite failed")
	}
}
