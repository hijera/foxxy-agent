//go:build http

package httpserver

import (
	"context"
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

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// turnRunnerMaxBlock bounds how long the fake runner holds a turn, so a regression fails the
// test instead of hanging it.
const turnRunnerMaxBlock = 20 * time.Second

// backendWindow stands in for one IDE window: its own Server and Manager, over a sessions
// root shared with the other window - which is exactly what two open projects give you, since
// the port is per window but the FoxxyCode home is not.
type backendWindow struct {
	ts      *httptest.Server
	mgr     *session.Manager
	started chan struct{}
	release chan struct{}
	ran     bool
	mu      sync.Mutex
	relOnce sync.Once
}

// releaseTurn lets the blocked runner finish. Safe to call more than once: the test releases
// explicitly and cleanup releases again for the failure paths.
func (w *backendWindow) releaseTurn() {
	w.relOnce.Do(func() { close(w.release) })
}

func newBackendWindow(t *testing.T, cwd, sessRoot string) *backendWindow {
	t.Helper()
	w := &backendWindow{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: filepath.Join(sessRoot, ".."), CWD: cwd},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		w.mu.Lock()
		w.ran = true
		w.mu.Unlock()
		var sb strings.Builder
		for _, b := range prompt {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: strings.TrimSpace(sb.String())})
		select {
		case w.started <- struct{}{}:
		default:
		}
		select {
		case <-w.release:
		case <-ctx.Done():
		// A window that should never have got here must not hang the suite: without the
		// cross-process lock the second window enters this runner and would block forever.
		case <-time.After(turnRunnerMaxBlock):
		}
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "done"})
		return string(acp.StopReasonEndTurn), nil
	}
	w.mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), cwd, &session.FileStore{Root: sessRoot})
	srv := New(cfg, w.mgr, slog.Default(), cwd)
	w.ts = httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		w.releaseTurn()
		w.ts.Close()
		srv.Drain()
	})
	return w
}

func (w *backendWindow) prompt(sid, text string) (int, string) {
	body := `{"model":"agent","stream":true,"input":"` + text + `"}`
	req, err := http.NewRequest(http.MethodPost, w.ts.URL+"/v1/responses", strings.NewReader(body))
	if err != nil {
		return -1, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", sid)
	res, err := w.ts.Client().Do(req)
	if err != nil {
		return -1, err.Error()
	}
	defer func() { _ = res.Body.Close() }()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

func (w *backendWindow) didRun() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ran
}

// Two IDE windows must not run a turn on the same session at once. Both persist the whole
// transcript, so the one that finishes last overwrites the other - the losing turn disappears
// entirely, prompt included.
func TestSecondWindowCannotStartATurnOnABusySession(t *testing.T) {
	cwd := t.TempDir()
	root := t.TempDir()
	sessRoot := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	windowA := newBackendWindow(t, cwd, sessRoot)
	windowB := newBackendWindow(t, cwd, sessRoot)

	res, err := windowA.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	sid := res.SessionID

	// Window A starts a turn and stays inside the runner.
	done := make(chan int, 1)
	go func() {
		code, _ := windowA.prompt(sid, "window A")
		done <- code
	}()
	select {
	case <-windowA.started:
	case <-time.After(15 * time.Second):
		t.Fatal("window A never entered its turn")
	}

	// Window B asks for the same session while A holds it.
	codeB, bodyB := windowB.prompt(sid, "window B")

	if windowB.didRun() {
		t.Fatalf("window B ran a turn on a session window A was holding (status %d)", codeB)
	}
	if codeB != http.StatusConflict {
		t.Fatalf("window B got status %d, want %d (session busy); body: %s",
			codeB, http.StatusConflict, truncateForLog(bodyB))
	}

	// Releasing A hands the session over.
	windowA.releaseTurn()
	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("window A finished with status %d", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("window A never finished")
	}
}

func truncateForLog(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
