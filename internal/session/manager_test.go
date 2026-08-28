package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mcp"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type noopSender struct{}

func (noopSender) SendSessionUpdate(string, interface{}) error { return nil }

func (noopSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow"}, nil
}

type captureSender struct {
	mu  sync.Mutex
	ups []interface{}
}

func (c *captureSender) SendSessionUpdate(_ string, u interface{}) error {
	c.mu.Lock()
	c.ups = append(c.ups, u)
	c.mu.Unlock()
	return nil
}

func (c *captureSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow"}, nil
}

func (c *captureSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

func (noopSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

func testConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "p1", Type: "openai", APIKey: "k"},
			{Name: "p2", Type: "openai", APIKey: "k"},
			{Name: "p3", Type: "anthropic", APIKey: "k"},
		},
		Models: []config.ModelEntry{
			{Model: "p1/gpt-4o"},
			{Model: "p2/gpt-4o-mini"},
			{Model: "p3/claude-3"},
		},
		Agent: config.Agent{Model: "p1/gpt-4o"},
	}
}

func noopRunner(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
	return string(acp.StopReasonEndTurn), nil
}

func TestInitializeWithPersistenceAdvertisesLoad(t *testing.T) {
	cfg := testConfig()
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	if err := os.MkdirAll(store.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "/tmp", store)
	res, err := m.HandleInitialize(context.Background(), acp.InitializeParams{ProtocolVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !res.AgentCapabilities.LoadSession {
		t.Fatal("expected LoadSession true with store")
	}
	if res.AgentCapabilities.SessionCapabilities == nil {
		t.Fatal("expected SessionCapabilities with store")
	}
}

func TestManagerSessionNewUsesDefaultCWWhenClientEmpty(t *testing.T) {
	defaultDir := t.TempDir()
	want, err := filepath.Abs(defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	var gotCWD string
	runner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		gotCWD = st.GetCWD()
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, runner, slog.Default(), defaultDir, nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: ""})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}
	if _, err := m.HandleSessionPrompt(context.Background(), acp.SessionPromptParams{
		SessionID: res.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "x"}},
	}); err != nil {
		t.Fatalf("HandleSessionPrompt: %v", err)
	}
	if gotCWD != want {
		t.Fatalf("session cwd %q, want %q", gotCWD, want)
	}
}

func TestManagerSessionNewIncludesConfigOptions(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "", nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
		return
	}
	if len(res.ConfigOptions) < 2 {
		t.Fatalf("expected at least mode + model config options, got %d", len(res.ConfigOptions))
	}
	var modeOpt, modelOpt *acp.ConfigOption
	for i := range res.ConfigOptions {
		switch res.ConfigOptions[i].ID {
		case "mode":
			modeOpt = &res.ConfigOptions[i]
		case "model":
			modelOpt = &res.ConfigOptions[i]
		}
	}
	if modeOpt == nil {
		t.Fatal("expected config option id mode")
		return
	}
	if modeOpt.Category != "mode" || modeOpt.Type != "select" {
		t.Fatalf("mode option: %+v", modeOpt)
	}
	if modeOpt.CurrentValue != "agent" {
		t.Fatalf("expected current mode agent, got %q", modeOpt.CurrentValue)
	}
	var askModeFound, debugModeFound bool
	for _, option := range modeOpt.Options {
		if option.Value == string(session.ModeAsk) && option.Name == "Ask" {
			askModeFound = true
		}
		if option.Value == string(session.ModeDebug) && option.Name == "Debug" {
			debugModeFound = true
		}
	}
	if !askModeFound {
		t.Fatalf("expected Ask mode option, got %+v", modeOpt.Options)
	}
	if !debugModeFound {
		t.Fatalf("expected Debug mode option, got %+v", modeOpt.Options)
	}
	if modelOpt == nil {
		t.Fatal("expected config option id model")
		return
	}
	if modelOpt.Category != "model" || modelOpt.Type != "select" {
		t.Fatalf("model option: %+v", modelOpt)
	}
	if len(modelOpt.Options) != 3 {
		t.Fatalf("expected 3 model choices, got %d", len(modelOpt.Options))
	}
	if modelOpt.CurrentValue != "p1/gpt-4o" {
		t.Fatalf("expected default model p1/gpt-4o for agent mode, got %q", modelOpt.CurrentValue)
	}
}

func TestManagerSetConfigOptionModel(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "", nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}

	out, err := m.HandleSessionSetConfigOption(context.Background(), acp.SessionSetConfigOptionParams{
		SessionID: res.SessionID,
		ConfigID:  "model",
		Value:     "p3/claude-3",
	})
	if err != nil {
		t.Fatalf("HandleSessionSetConfigOption: %v", err)
	}
	if out == nil || len(out.ConfigOptions) < 2 {
		t.Fatalf("expected config options in result, got %+v", out)
	}
	var current string
	for _, o := range out.ConfigOptions {
		if o.ID == "model" {
			current = o.CurrentValue
			break
		}
	}
	if current != "p3/claude-3" {
		t.Fatalf("expected model p3/claude-3 after set, got %q", current)
	}
}

func TestManagerSetConfigOptionMode(t *testing.T) {
	cfg := testConfig()
	sender := &captureSender{}
	m := session.NewManager(cfg, sender, noopRunner, slog.Default(), "", nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}

	out, err := m.HandleSessionSetConfigOption(context.Background(), acp.SessionSetConfigOptionParams{
		SessionID: res.SessionID,
		ConfigID:  "mode",
		Value:     "plan",
	})
	if err != nil {
		t.Fatalf("HandleSessionSetConfigOption: %v", err)
	}
	var modeCur, modelCur string
	for _, o := range out.ConfigOptions {
		switch o.ID {
		case "mode":
			modeCur = o.CurrentValue
		case "model":
			modelCur = o.CurrentValue
		}
	}
	if modeCur != "plan" {
		t.Fatalf("expected mode plan, got %q", modeCur)
	}
	// No explicit model override: effective model stays agent.model (p1/gpt-4o).
	if modelCur != "p1/gpt-4o" {
		t.Fatalf("expected effective model p1/gpt-4o for plan mode without override, got %q", modelCur)
	}
	var modeWire map[string]interface{}
	for _, update := range sender.ups {
		if _, ok := update.(acp.ModeUpdate); !ok {
			continue
		}
		data, err := json.Marshal(update)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &modeWire); err != nil {
			t.Fatal(err)
		}
	}
	if modeWire["currentModeId"] != "plan" {
		t.Fatalf("currentModeId = %#v, want plan in %#v", modeWire["currentModeId"], modeWire)
	}
	if _, ok := modeWire["modeId"]; ok {
		t.Fatalf("deprecated modeId emitted in %#v", modeWire)
	}
}

func TestManagerSetConfigOptionAskMode(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "", nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}

	out, err := m.HandleSessionSetConfigOption(context.Background(), acp.SessionSetConfigOptionParams{
		SessionID: res.SessionID,
		ConfigID:  "mode",
		Value:     "ask",
	})
	if err != nil {
		t.Fatalf("HandleSessionSetConfigOption ask: %v", err)
	}
	for _, option := range out.ConfigOptions {
		if option.ID == "mode" && option.CurrentValue == "ask" {
			return
		}
	}
	t.Fatalf("Ask mode was not selected: %+v", out.ConfigOptions)
}

func TestManagerSetConfigOptionDebugMode(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "", nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}

	out, err := m.HandleSessionSetConfigOption(context.Background(), acp.SessionSetConfigOptionParams{
		SessionID: res.SessionID,
		ConfigID:  "mode",
		Value:     "debug",
	})
	if err != nil {
		t.Fatalf("HandleSessionSetConfigOption debug: %v", err)
	}
	for _, option := range out.ConfigOptions {
		if option.ID == "mode" && option.CurrentValue == "debug" {
			return
		}
	}
	t.Fatalf("Debug mode was not selected: %+v", out.ConfigOptions)
}

// The legacy session/set_mode path is gated on IsValidMode, and the modes it
// advertises in session/new come from a separate literal list - so a mode can be
// settable while staying invisible to an ACP client. Pin both together.
func TestManagerSetModeDebugIsAdvertised(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "", nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}
	if res.Modes == nil {
		t.Fatal("session/new returned no mode state")
	}
	advertised := make(map[string]bool, len(res.Modes.AvailableModes))
	for _, mode := range res.Modes.AvailableModes {
		advertised[mode.ID] = true
	}
	for _, want := range []string{"agent", "plan", "docs", "ask", "debug"} {
		if !advertised[want] {
			t.Errorf("session/new does not advertise mode %q: %+v", want, res.Modes.AvailableModes)
		}
	}

	if err := m.HandleSessionSetMode(context.Background(), acp.SessionSetModeParams{
		SessionID: res.SessionID,
		ModeID:    "debug",
	}); err != nil {
		t.Fatalf("HandleSessionSetMode debug: %v", err)
	}
	if got := m.SessionByID(res.SessionID).GetMode(); got != string(session.ModeDebug) {
		t.Fatalf("session mode: want debug got %q", got)
	}
}

func TestManagerSetConfigOptionUnknownValue(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "", nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}

	_, err = m.HandleSessionSetConfigOption(context.Background(), acp.SessionSetConfigOptionParams{
		SessionID: res.SessionID,
		ConfigID:  "model",
		Value:     "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for unknown model id")
	}
}

func TestSessionLoadDoesNotRewriteSessionUpdatedAt(t *testing.T) {
	root := t.TempDir()
	store := &session.FileStore{Root: root}
	cfg := testConfig()

	id := "sess_list_order_keep"
	dir, err := store.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(dir, "session.json")
	var meta struct {
		Version   int    `json:"version"`
		ID        string `json:"id"`
		CWD       string `json:"cwd"`
		Mode      string `json:"mode"`
		UpdatedAt string `json:"updatedAt"`
	}
	before := "2019-06-01T12:00:00Z"
	if err := json.Unmarshal(slurpFile(t, metaPath), &meta); err != nil {
		t.Fatal(err)
	}
	meta.CWD = "/tmp"
	meta.Mode = "agent"
	meta.UpdatedAt = before
	writeJSONIndent(t, metaPath, meta)
	msgPath := filepath.Join(dir, "messages.json")
	msgWrap := map[string]interface{}{
		"version":  1,
		"messages": []interface{}{},
	}
	writeJSONIndent(t, msgPath, msgWrap)

	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "/tmp", store)
	ctx := context.Background()
	if _, err := m.HandleSessionLoad(ctx, acp.SessionLoadParams{
		SessionID:  id,
		CWD:        "/tmp",
		MCPServers: nil,
	}); err != nil {
		t.Fatal(err)
	}
	var metaAfter struct {
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.Unmarshal(slurpFile(t, metaPath), &metaAfter); err != nil {
		t.Fatal(err)
	}
	if metaAfter.UpdatedAt != before {
		t.Fatalf("session.json updatedAt changed on load: %q -> %q", before, metaAfter.UpdatedAt)
	}
}

func slurpFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeJSONIndent(t *testing.T, path string, v interface{}) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManagerPersistMessagesAndReload(t *testing.T) {
	root := t.TempDir()
	store := &session.FileStore{Root: root}
	cfg := testConfig()

	persistRunner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "u"})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "a"})
		return string(acp.StopReasonEndTurn), nil
	}

	m1 := session.NewManager(cfg, noopSender{}, persistRunner, slog.Default(), "/tmp", store)
	ctx := context.Background()
	res1, err := m1.HandleSessionNew(ctx, acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	id := res1.SessionID
	if _, err := m1.HandleSessionPrompt(ctx, acp.SessionPromptParams{
		SessionID: id,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "ignored-by-test-runner"}},
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := store.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Messages) != 2 {
		t.Fatalf("expected 2 persisted messages after prompt, got %d", len(snap.Messages))
	}

	var afterReload int
	peekRunner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		afterReload = len(st.GetMessages())
		return string(acp.StopReasonEndTurn), nil
	}

	m2 := session.NewManager(cfg, noopSender{}, peekRunner, slog.Default(), "/tmp", store)
	if _, err := m2.HandleSessionLoad(ctx, acp.SessionLoadParams{
		SessionID:  id,
		CWD:        "/tmp",
		MCPServers: nil,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := m2.HandleSessionPrompt(ctx, acp.SessionPromptParams{
		SessionID: id,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "check"}},
	}); err != nil {
		t.Fatal(err)
	}
	if afterReload != 2 {
		t.Fatalf("session/load should restore 2 persisted messages before turn runs, got %d", afterReload)
	}
}

func TestHandleSessionCancelEndsBlockedPrompt(t *testing.T) {
	cfg := testConfig()
	blockStarted := make(chan struct{})
	runner := func(ctx context.Context, _ *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		close(blockStarted)
		<-ctx.Done()
		return string(acp.StopReasonCancelled), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), "/tmp", nil)
	ctx := context.Background()
	res, err := mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	id := res.SessionID

	var wg sync.WaitGroup
	wg.Add(1)
	var out *acp.SessionPromptResult
	var promptErr error
	go func() {
		defer wg.Done()
		out, promptErr = mgr.HandleSessionPrompt(ctx, acp.SessionPromptParams{
			SessionID: id,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: "hello"}},
		})
	}()

	<-blockStarted
	mgr.HandleSessionCancel(acp.SessionCancelParams{SessionID: id})
	wg.Wait()

	if promptErr != nil {
		t.Fatalf("prompt: %v", promptErr)
	}
	if out == nil {
		t.Fatal("nil prompt result")
	}
	if out.StopReason != acp.StopReasonCancelled {
		t.Fatalf("stop reason %q want %q", out.StopReason, acp.StopReasonCancelled)
	}
}

func TestHandleSessionPromptWithSenderDetachFromRequestSurvivesParentCancel(t *testing.T) {
	res, perr, ctxErr := runPromptWithCancelledParent(t, &session.PromptRunOpts{
		SkipTurnLock:      true,
		DetachFromRequest: true,
	})
	if perr != nil {
		t.Fatalf("prompt: %v", perr)
	}
	if ctxErr != nil {
		t.Fatalf("a detached turn must not see its parent's cancellation: %v", ctxErr)
	}
	if res == nil || res.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("unexpected %+v err=%v", res, perr)
	}
}

// Holding the turn lock outside the manager says nothing about who owns the turn's
// lifetime. A caller that only sets SkipTurnLock - a non-streaming composer POST, which
// can be stopped only by hanging up - keeps request-scoped cancellation.
func TestHandleSessionPromptWithSenderStaysRequestScopedWithoutDetach(t *testing.T) {
	_, _, ctxErr := runPromptWithCancelledParent(t, &session.PromptRunOpts{SkipTurnLock: true})
	if ctxErr == nil {
		t.Fatal("turn context outlived the cancelled parent without DetachFromRequest")
	}
}

// runPromptWithCancelledParent runs one turn whose parent context is cancelled while the
// runner blocks, and reports what the runner saw of its own context.
func runPromptWithCancelledParent(t *testing.T, opts *session.PromptRunOpts) (*acp.SessionPromptResult, error, error) {
	t.Helper()
	runBlock := make(chan struct{})
	cont := make(chan struct{})
	var ctxErr error
	runner := func(ctx context.Context, _ *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		close(runBlock)
		<-cont
		ctxErr = ctx.Err()
		return string(acp.StopReasonEndTurn), nil
	}
	m := session.NewManager(testConfig(), noopSender{}, runner, slog.Default(), "/tmp", nil)
	sn, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	var res *acp.SessionPromptResult
	var perr error
	go func() {
		defer wg.Done()
		res, perr = m.HandleSessionPromptWithSender(ctx, acp.SessionPromptParams{
			SessionID: sn.SessionID,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: "x"}},
		}, noopSender{}, opts)
	}()
	<-runBlock
	cancel()
	close(cont)
	wg.Wait()
	return res, perr, ctxErr
}

func TestSessionTurnActiveInProcessDuringTurn(t *testing.T) {
	runBlock := make(chan struct{})
	cont := make(chan struct{})
	runner := func(ctx context.Context, _ *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		close(runBlock)
		<-cont
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, runner, slog.Default(), "/tmp", nil)
	sn, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if m.SessionTurnActiveInProcess(sn.SessionID) {
		t.Fatal("turn should be inactive before prompt")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = m.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
			SessionID: sn.SessionID,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: "x"}},
		}, noopSender{}, &session.PromptRunOpts{SkipTurnLock: true})
	}()
	<-runBlock
	if !m.SessionTurnActiveInProcess(sn.SessionID) {
		t.Fatal("turn should be active while runner is in flight")
	}
	close(cont)
	wg.Wait()
	if m.SessionTurnActiveInProcess(sn.SessionID) {
		t.Fatal("turn should be inactive after completion")
	}
}

func TestSessionNewSendsAvailableSlashCommandsUpdate(t *testing.T) {
	skRoot := t.TempDir()
	skillDir := filepath.Join(skRoot, "probe")
	if err := os.MkdirAll(filepath.Join(skillDir, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "demo", "SKILL.md"), []byte("# Demo skill\n\nRuns demo flow.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.Skills.Dirs = []string{skillDir}
	snd := &captureSender{}
	m := session.NewManager(cfg, snd, noopRunner, slog.Default(), t.TempDir(), nil)
	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}
	m.HandleSessionReady(res.SessionID)
	var slash *acp.AvailableCommandsUpdate
	for _, u := range snd.ups {
		if v, ok := u.(acp.AvailableCommandsUpdate); ok && v.SessionUpdate == acp.UpdateTypeAvailableCommandsUpdate {
			slash = &v
			break
		}
	}
	if slash == nil {
		t.Fatalf("expected AvailableCommandsUpdate in %#v", snd.ups)
		return
	}
	// Skills (demo + the two bundled ones) plus the built-in compact command
	// (coddy compaction engine) and the always-present plugin command.
	if len(slash.AvailableCommands) != 5 {
		t.Fatalf("unexpected commands %+v", slash.AvailableCommands)
	}
	names := map[string]bool{}
	for _, c := range slash.AvailableCommands {
		names[c.Name] = true
	}
	if !names["demo"] || !names["generate-rules"] || !names["configure-foxxycode"] || !names["compact"] || !names["plugin"] {
		t.Fatalf("expected demo, generate-rules, configure-foxxycode, compact and plugin, got %+v", slash.AvailableCommands)
	}
}

func TestSetSessionWorkspaceSwitchesCwdAndPersists(t *testing.T) {
	cfg := testConfig()
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	if err := os.MkdirAll(store.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "/tmp", store)

	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	for _, d := range []string{alpha, beta} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: alpha})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}
	st := m.SessionByID(res.SessionID)
	if st == nil {
		t.Fatal("session not registered")
	}

	if err := m.SetSessionWorkspace(st, beta); err != nil {
		t.Fatalf("SetSessionWorkspace: %v", err)
	}
	if got := st.GetCWD(); got != beta {
		t.Fatalf("cwd = %q, want %q", got, beta)
	}

	raw, err := os.ReadFile(filepath.Join(store.Root, res.SessionID, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		CWD string `json:"cwd"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.CWD != beta {
		t.Fatalf("persisted cwd = %q, want %q", meta.CWD, beta)
	}

	if err := m.SetSessionWorkspace(st, filepath.Join(root, "missing")); err == nil {
		t.Fatal("expected error for missing folder")
	}
	if got := st.GetCWD(); got != beta {
		t.Fatalf("cwd changed on failed switch: %q", got)
	}
}

func TestSetSessionWorkspaceRejectsActiveTurn(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	for _, dir := range []string{alpha, beta} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		close(entered)
		<-release
		return string(acp.StopReasonEndTurn), nil
	}
	m := session.NewManager(testConfig(), noopSender{}, runner, slog.Default(), alpha, nil)
	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: alpha})
	if err != nil {
		t.Fatal(err)
	}
	st := m.SessionByID(res.SessionID)
	if st == nil {
		t.Fatal("session not registered")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = m.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
			SessionID: res.SessionID,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: "hold"}},
		}, noopSender{}, &session.PromptRunOpts{SkipTurnLock: true})
	}()
	<-entered
	defer func() {
		close(release)
		<-done
	}()
	if err := m.SetSessionWorkspace(st, beta); !errors.Is(err, session.ErrSessionTurnBusy) {
		t.Fatalf("SetSessionWorkspace error = %v, want ErrSessionTurnBusy", err)
	}
	if got := st.GetCWD(); got != alpha {
		t.Fatalf("cwd changed during active turn: %q", got)
	}
}

func TestEffectiveMCPServersMergesGlobalAndProject(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{MCPServers: []config.MCPServerConfig{
		{Name: "cfg-srv", Command: "cfg-mcp"},
		{Name: "off-srv", Command: "off-mcp", Disabled: true},
	}}
	cfg.Paths.Home = home
	cwd := t.TempDir()

	// Global <home>/mcp.json overrides config.yaml; project overrides both.
	if err := config.UpsertMCPJSONServer(config.GlobalMCPJSONPath(home), "home-srv", config.MCPJSONServer{Command: "home-mcp"}); err != nil {
		t.Fatal(err)
	}
	if err := config.UpsertMCPJSONServer(config.GlobalMCPJSONPath(home), "cfg-srv", config.MCPJSONServer{Command: "home-override"}); err != nil {
		t.Fatal(err)
	}
	if err := config.UpsertMCPJSONServer(config.MCPJSONPath(cwd), "home-srv", config.MCPJSONServer{Command: "proj-override"}); err != nil {
		t.Fatal(err)
	}
	if err := config.UpsertMCPJSONServer(config.MCPJSONPath(cwd), "proj-srv", config.MCPJSONServer{Command: "proj-mcp"}); err != nil {
		t.Fatal(err)
	}

	servers := session.EffectiveMCPServers(cfg, cwd, slog.Default())
	if len(servers) != 4 {
		t.Fatalf("servers = %+v, want 4", servers)
	}
	byName := map[string]config.MCPServerConfig{}
	for _, s := range servers {
		byName[s.Name] = s
	}
	if byName["cfg-srv"].Command != "home-override" {
		t.Errorf("cfg-srv command = %q, want global mcp.json override", byName["cfg-srv"].Command)
	}
	if byName["home-srv"].Command != "proj-override" {
		t.Errorf("home-srv command = %q, want project override", byName["home-srv"].Command)
	}
	if !byName["off-srv"].Disabled {
		t.Errorf("off-srv must keep its disabled flag in the effective list")
	}
	if _, ok := byName["proj-srv"]; !ok {
		t.Errorf("proj-srv missing from effective list")
	}

	// A broken project mcp.json must not fail the session; config.yaml plus
	// the global file still apply.
	if err := os.WriteFile(filepath.Join(cwd, ".foxxycode", "mcp.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	servers = session.EffectiveMCPServers(cfg, cwd, slog.Default())
	if len(servers) != 3 {
		t.Fatalf("servers with broken project mcp.json = %+v, want 3", servers)
	}
}

func TestEffectiveMCPServersReportsABrokenFileOnce(t *testing.T) {
	// The effective list is rebuilt on every turn and every MCP tool call, so a
	// file that stays broken must not warn each time. A different failure, or the
	// same one after a good load, is reported again.
	home := t.TempDir()
	cfg := &config.Config{}
	cfg.Paths.Home = home
	cwd := t.TempDir()

	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn}))
	path := config.MCPJSONPath(cwd)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	warnings := func() int {
		return strings.Count(logged.String(), "failed to load mcp.json")
	}

	write("{broken")
	for i := 0; i < 5; i++ {
		session.EffectiveMCPServers(cfg, cwd, log)
	}
	if got := warnings(); got != 1 {
		t.Fatalf("same failure logged %d times, want 1:\n%s", got, logged.String())
	}

	// Fixing the file and breaking it again must warn again, so a real
	// regression is never swallowed by the deduplication.
	write(`{"mcpServers":{}}`)
	session.EffectiveMCPServers(cfg, cwd, log)
	write("{broken")
	session.EffectiveMCPServers(cfg, cwd, log)
	if got := warnings(); got != 2 {
		t.Fatalf("a failure after a good load logged %d times total, want 2:\n%s", got, logged.String())
	}
}

func TestStateMCPToolFilter(t *testing.T) {
	st := &session.State{ID: "s", CWD: t.TempDir()}
	if allowed := st.GetMCPToolFilter(); !allowed("any", "tool") {
		t.Error("nil factory must allow everything")
	}
	st.MCPFilterFactory = func() func(server, tool string) bool {
		return func(server, tool string) bool { return tool == "echo" }
	}
	allowed := st.GetMCPToolFilter()
	if !allowed("srv", "echo") || allowed("srv", "write") {
		t.Error("factory-built filter must be used when set")
	}
}

func TestSetSessionWorkspaceReconnectsProjectMCPAndPreservesClientServers(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	for _, dir := range []string{home, alpha, beta} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	mcpServer := httptest.NewServer(&fakeBetaHandler{token: "workspace"})
	defer mcpServer.Close()
	if err := config.UpsertMCPJSONServer(config.MCPJSONPath(alpha), "alpha-project", config.MCPJSONServer{
		Type:          "http",
		URL:           mcpServer.URL,
		DisabledTools: []string{"alpha-disabled"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.UpsertMCPJSONServer(config.MCPJSONPath(beta), "beta-project", config.MCPJSONServer{
		Type:          "http",
		URL:           mcpServer.URL,
		DisabledTools: []string{"beta-disabled"},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	cfg.Paths.Home = home
	// Both declarations are project-local, so the workspace trust gate holds
	// them until the operator approves each one for its own folder. This test
	// is about what a workspace switch reconnects, not about the gate, so both
	// are approved up front; the gate itself is covered by
	// features/mcp_project_trust.feature.
	approveProjectMCP(t, cfg, alpha, "alpha-project")
	approveProjectMCP(t, cfg, beta, "beta-project")

	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), alpha, nil)
	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{
		CWD: alpha,
		MCPServers: []acp.MCPServer{{
			Name: "client-supplied",
			Type: "http",
			URL:  mcpServer.URL,
		}},
	})
	if err != nil {
		t.Fatalf("HandleSessionNew: %v", err)
	}
	st := m.SessionByID(res.SessionID)
	if st == nil {
		t.Fatal("session not registered")
	}
	defer st.CloseAll()

	assertMCPClientNames(t, st, "alpha-project", "client-supplied")
	clientSupplied := mcpClientByName(t, st, "client-supplied")
	alphaFilter := st.GetMCPToolFilter()
	if alphaFilter("alpha-project", "alpha-disabled") {
		t.Fatal("alpha project disabled tool is allowed")
	}

	if err := m.SetSessionWorkspace(st, beta); err != nil {
		t.Fatalf("SetSessionWorkspace: %v", err)
	}
	assertMCPClientNames(t, st, "beta-project", "client-supplied")
	if got := mcpClientByName(t, st, "client-supplied"); got != clientSupplied {
		t.Fatal("workspace switch replaced the client-supplied MCP connection")
	}
	result, err := mcpClientByName(t, st, "beta-project").CallTool(context.Background(), "get_token", `{}`)
	if err != nil {
		t.Fatalf("call beta project MCP tool: %v", err)
	}
	if result != "workspace" {
		t.Fatalf("beta project MCP tool result = %q, want workspace", result)
	}

	betaFilter := st.GetMCPToolFilter()
	if betaFilter("beta-project", "beta-disabled") {
		t.Fatal("beta project disabled tool is allowed after workspace switch")
	}
	if !betaFilter("alpha-project", "alpha-disabled") {
		t.Fatal("filter still reads the alpha project after workspace switch")
	}
}

func mcpClientByName(t *testing.T, st *session.State, name string) *mcp.Client {
	t.Helper()
	for _, client := range st.GetMCPClients() {
		if client.Name() == name {
			return client
		}
	}
	t.Fatalf("MCP client %q not found", name)
	return nil
}

func assertMCPClientNames(t *testing.T, st *session.State, want ...string) {
	t.Helper()
	// Configured servers connect in the background so a session load never blocks a request;
	// a turn waits through this same gate before it builds its tool set.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := st.WaitMCPReady(ctx); err != nil {
		t.Fatalf("waiting for configured MCP servers: %v", err)
	}
	got := make([]string, 0, len(st.GetMCPClients()))
	for _, client := range st.GetMCPClients() {
		got = append(got, client.Name())
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("MCP clients = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MCP clients = %v, want %v", got, want)
		}
	}
}

// approveProjectMCP records the operator's approval of a project-local server
// for one workspace, the way `foxxycode mcp trust` and the Settings shield do.
func approveProjectMCP(t *testing.T, cfg *config.Config, workspace, name string) {
	t.Helper()
	servers, err := mcp.ListManagedServers(cfg, workspace)
	if err != nil {
		t.Fatalf("list managed MCP servers for %s: %v", workspace, err)
	}
	for i := range servers {
		if servers[i].Config.Name == name {
			if err := mcp.NewTrustGate(cfg).Approve(workspace, servers[i]); err != nil {
				t.Fatalf("approve %q for %s: %v", name, workspace, err)
			}
			return
		}
	}
	t.Fatalf("mcp server %q not declared in %s", name, workspace)
}
