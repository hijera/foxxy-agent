package session_test

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// TestHelperSilentSessionMCPServer is not a real test: with GO_WANT_SILENT_SESSION_MCP=1 it
// stands in for an MCP server that never answers the handshake - a cold npx that is still
// downloading its package, which is exactly the case that used to freeze a panel.
func TestHelperSilentSessionMCPServer(t *testing.T) {
	if os.Getenv("GO_WANT_SILENT_SESSION_MCP") != "1" {
		t.Skip("helper process")
	}
	in := bufio.NewReader(os.Stdin)
	for {
		if _, err := in.ReadBytes('\n'); err != nil {
			return
		}
	}
}

// managerWithSilentMCP builds a manager whose config declares one MCP server that never
// completes its handshake. Declared through cfg.MCPServers so the trust gate allows it
// without an approval (it is operator configuration, not a project declaration).
func managerWithSilentMCP(t *testing.T) (*session.Manager, string) {
	t.Helper()
	cwd := t.TempDir()
	cfg := testConfig()
	cfg.Paths = config.Paths{Home: t.TempDir(), CWD: cwd}
	cfg.MCPServers = []config.MCPServerConfig{{
		Name:    "cold",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperSilentSessionMCPServer"},
		Env:     []config.EnvVarConfig{{Name: "GO_WANT_SILENT_SESSION_MCP", Value: "1"}},
	}}
	return session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), cwd, nil), cwd
}

// Creating a session must not wait on the servers. Over HTTP this call sits on a request a
// panel is blocked on, and one slow server used to hold it (and one of the webview's six
// connections) for minutes.
func TestSessionNewDoesNotBlockOnAColdMCPServer(t *testing.T) {
	m, cwd := managerWithSilentMCP(t)

	done := make(chan string, 1)
	go func() {
		res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
		if err != nil {
			done <- ""
			return
		}
		done <- res.SessionID
	}()

	select {
	case id := <-done:
		if id == "" {
			t.Fatal("HandleSessionNew failed")
		}
		t.Cleanup(func() { m.ForgetLiveSession(id) })
	case <-time.After(5 * time.Second):
		t.Fatal("HandleSessionNew is still waiting for the MCP handshake")
	}
}

// The gate is not vacuous: a working server does show up, and WaitMCPReady is what tells a
// caller it may look.
func TestWaitMCPReadyResolvesWhenTheServerConnects(t *testing.T) {
	cwd := t.TempDir()
	cfg := testConfig()
	cfg.Paths = config.Paths{Home: t.TempDir(), CWD: cwd}
	cfg.MCPServers = []config.MCPServerConfig{{
		Name:    "warm",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperMCPMarkerServer"},
		Env: []config.EnvVarConfig{
			{Name: "GO_WANT_MCP_MARKER", Value: "1"},
		},
	}}
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), cwd, nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := m.SessionByID(res.SessionID)
	if st == nil {
		t.Fatal("session not registered")
	}
	defer st.CloseAll()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := st.WaitMCPReady(ctx); err != nil {
		t.Fatalf("WaitMCPReady: %v", err)
	}
	if !st.MCPReady() {
		t.Fatal("MCPReady is false after WaitMCPReady returned")
	}
	if len(st.GetMCPClients()) != 1 {
		t.Fatalf("expected the configured server to be connected, got %d clients", len(st.GetMCPClients()))
	}
}

// A caller that gives up (a cancelled turn, a Stop) gets its context error back rather than
// waiting for the handshake to time out.
func TestWaitMCPReadyRespectsTheCallerDeadline(t *testing.T) {
	m, cwd := managerWithSilentMCP(t)
	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := m.SessionByID(res.SessionID)
	if st == nil {
		t.Fatal("session not registered")
	}
	defer st.CloseAll()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := st.WaitMCPReady(ctx); err == nil {
		t.Fatal("expected the caller deadline to win over a hanging handshake")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("WaitMCPReady waited %s past its deadline", elapsed)
	}
}

// Closing a session releases whoever is waiting on its connect, and the clients that connect
// afterwards are closed instead of being attached to state nobody owns.
func TestCloseAllReleasesWaitersAndDropsLateClients(t *testing.T) {
	m, cwd := managerWithSilentMCP(t)
	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := m.SessionByID(res.SessionID)
	if st == nil {
		t.Fatal("session not registered")
	}

	waited := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		waited <- st.WaitMCPReady(ctx)
	}()

	st.CloseAll()

	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("a waiter on a closed session should be released cleanly, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CloseAll did not release the waiter")
	}
	if len(st.GetMCPClients()) != 0 {
		t.Fatalf("a closed session must not hold MCP clients, got %d", len(st.GetMCPClients()))
	}
}

// The point of the readiness gate: making the connect asynchronous must not cost the first
// turn its MCP tools. The runner stands in for the agent, which reads the client list once per
// turn when it builds its tool definitions.
func TestFirstTurnWaitsForConfiguredMCPServers(t *testing.T) {
	cwd := t.TempDir()
	cfg := testConfig()
	cfg.Paths = config.Paths{Home: t.TempDir(), CWD: cwd}
	cfg.MCPServers = []config.MCPServerConfig{{
		Name:    "warm",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperMCPMarkerServer"},
		Env:     []config.EnvVarConfig{{Name: "GO_WANT_MCP_MARKER", Value: "1"}},
	}}
	seen := make(chan int, 1)
	runner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		seen <- len(st.GetMCPClients())
		return string(acp.StopReasonEndTurn), nil
	}
	m := session.NewManager(cfg, noopSender{}, runner, slog.Default(), cwd, nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.ForgetLiveSession(res.SessionID) })

	if _, err := m.HandleSessionPrompt(context.Background(), acp.SessionPromptParams{
		SessionID: res.SessionID,
		Prompt:    []acp.ContentBlock{{Type: "text", Text: "hello"}},
	}); err != nil {
		t.Fatalf("HandleSessionPrompt: %v", err)
	}

	select {
	case got := <-seen:
		if got != 1 {
			t.Fatalf("the first turn saw %d MCP clients, want 1", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the runner never ran")
	}
}

// A turn tells the client what it is waiting for, so a panel can say "connecting MCP servers"
// instead of looking like the model went quiet. A warm session says nothing at all.
func TestTurnAnnouncesTheMCPWaitOnlyWhenItWaits(t *testing.T) {
	cwd := t.TempDir()
	cfg := testConfig()
	cfg.Paths = config.Paths{Home: t.TempDir(), CWD: cwd}
	cfg.MCPServers = []config.MCPServerConfig{{
		Name:    "warm",
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperMCPMarkerServer"},
		Env:     []config.EnvVarConfig{{Name: "GO_WANT_MCP_MARKER", Value: "1"}},
	}}
	sender := &captureSender{}
	m := session.NewManager(cfg, sender, noopRunner, slog.Default(), cwd, nil)

	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.ForgetLiveSession(res.SessionID) })

	prompt := func() {
		if _, err := m.HandleSessionPrompt(context.Background(), acp.SessionPromptParams{
			SessionID: res.SessionID,
			Prompt:    []acp.ContentBlock{{Type: "text", Text: "hello"}},
		}); err != nil {
			t.Fatalf("HandleSessionPrompt: %v", err)
		}
	}
	phases := func() []string {
		sender.mu.Lock()
		defer sender.mu.Unlock()
		var out []string
		for _, u := range sender.ups {
			if p, ok := u.(acp.MCPPhaseUpdate); ok {
				out = append(out, p.Phase)
			}
		}
		return out
	}

	prompt()
	first := phases()
	if len(first) != 0 && (len(first) != 2 || first[0] != acp.MCPPhaseConnecting || first[1] != acp.MCPPhaseReady) {
		t.Fatalf("first turn phases = %v, want none or connecting+ready", first)
	}

	// The servers are up by now, so the second turn must be silent about them.
	before := len(phases())
	prompt()
	if got := len(phases()); got != before {
		t.Fatalf("a warm turn announced an MCP wait: phases = %v", phases())
	}
}
