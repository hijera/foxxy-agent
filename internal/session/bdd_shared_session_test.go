package session_test

// Godog harness for features/session_shared_between_processes.feature: two
// Managers with stores of their own over one sessions root stand for two
// processes on one home - an editor panel's `foxxycode http` and the Telegram
// gateway - holding the same session live.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type sharedSessionWorld struct {
	root, cwd string
	id        string
	procs     map[string]*session.Manager

	mu sync.Mutex
	// began is how many messages the last turn of each process found in the
	// session when it started.
	began map[string]int
}

func (w *sharedSessionWorld) reset() error {
	w.cleanup()
	root, err := os.MkdirTemp("", "foxxycode-bdd-shared-root-*")
	if err != nil {
		return err
	}
	cwd, err := os.MkdirTemp("", "foxxycode-bdd-shared-cwd-*")
	if err != nil {
		return err
	}
	w.root, w.cwd, w.id = root, cwd, ""
	w.began = map[string]int{}
	w.procs = map[string]*session.Manager{}
	for _, name := range []string{"panel", "gateway"} {
		w.procs[name] = session.NewManager(testConfig(), noopSender{}, w.runner(name), slog.Default(), cwd, &session.FileStore{Root: root})
	}
	return nil
}

func (w *sharedSessionWorld) cleanup() {
	for _, dir := range []string{w.root, w.cwd} {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
	w.root, w.cwd = "", ""
}

// runner answers every prompt with "<process>: <prompt>" and records how many
// messages the session held when the turn began.
func (w *sharedSessionWorld) runner(name string) session.AgentRunner {
	return func(_ context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		text := prompt[0].Text
		w.mu.Lock()
		w.began[name] = len(st.GetMessages())
		w.mu.Unlock()
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: text})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: name + ": " + text})
		return string(acp.StopReasonEndTurn), nil
	}
}

func (w *sharedSessionWorld) sessionHeldByBoth() error {
	ctx := context.Background()
	res, err := w.procs["panel"].HandleSessionNew(ctx, acp.SessionNewParams{CWD: w.cwd})
	if err != nil {
		return err
	}
	w.id = res.SessionID
	// The session exists on disk once the panel has saved it; the gateway
	// loads it the way /resume does.
	return nil
}

func (w *sharedSessionWorld) runsATurn(proc, text string) error {
	_, err := w.procs[proc].HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
		SessionID: w.id,
		Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: text}},
	}, noopSender{}, nil)
	return err
}

func (w *sharedSessionWorld) panelRanFirstTurn(text string) error {
	if err := w.runsATurn("panel", text); err != nil {
		return err
	}
	_, err := w.procs["gateway"].EnsureHTTPSession(context.Background(), w.id, w.cwd)
	return err
}

func (w *sharedSessionWorld) switchesMode(proc, mode string) error {
	return w.procs[proc].HandleSessionSetMode(context.Background(), acp.SessionSetModeParams{SessionID: w.id, ModeID: mode})
}

func (w *sharedSessionWorld) turnBeganOn(proc string, want int) error {
	w.mu.Lock()
	got, ok := w.began[proc]
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("the %s ran no turn", proc)
	}
	if got != want {
		return fmt.Errorf("the %s's turn began on %d messages, want %d", proc, got, want)
	}
	return nil
}

func (w *sharedSessionWorld) transcript() ([]string, error) {
	b, err := os.ReadFile(filepath.Join(w.root, w.id, session.MessagesFileName))
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Messages []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(wrap.Messages))
	for _, m := range wrap.Messages {
		out = append(out, m.Content)
	}
	return out, nil
}

func (w *sharedSessionWorld) transcriptIs(want string) error {
	got, err := w.transcript()
	if err != nil {
		return err
	}
	if strings.Join(got, " | ") != want {
		return fmt.Errorf("transcript on disk = %q, want %q", strings.Join(got, " | "), want)
	}
	return nil
}

func (w *sharedSessionWorld) modeOnDisk(want string) error {
	meta, err := (&session.FileStore{Root: w.root}).ReadMeta(w.id)
	if err != nil {
		return err
	}
	if meta.Mode != want {
		return fmt.Errorf("mode on disk = %q, want %q", meta.Mode, want)
	}
	return nil
}

func initializeSharedSessionScenario(sc *godog.ScenarioContext) {
	w := &sharedSessionWorld{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, w.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.cleanup()
		return ctx, nil
	})
	sc.Step(`^a session held live by the panel and the gateway$`, w.sessionHeldByBoth)
	sc.Step(`^the panel ran a turn "([^"]*)"$`, w.panelRanFirstTurn)
	sc.Step(`^the (panel|gateway) runs a turn "([^"]*)"$`, w.runsATurn)
	sc.Step(`^the (panel|gateway) switches the session to "([^"]*)" mode$`, w.switchesMode)
	sc.Step(`^the (panel|gateway)'s turn began on (\d+) messages$`, w.turnBeganOn)
	sc.Step(`^the transcript on disk is "([^"]*)"$`, w.transcriptIs)
	sc.Step(`^the session on disk is in "([^"]*)" mode$`, w.modeOnDisk)
}

func TestSessionSharedBetweenProcessesFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "session-shared-between-processes",
		ScenarioInitializer: initializeSharedSessionScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/session_shared_between_processes.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("session shared between processes feature suite failed")
	}
}
