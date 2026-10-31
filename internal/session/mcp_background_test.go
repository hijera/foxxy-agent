package session

// Tests of the background MCP connect (mcp_background.go): session/new
// returning before the servers answer, the control updates the console
// listens for, a turn waiting for the tool list, Stop ending that wait, a
// reload superseding a pending dial, and a teardown releasing it.

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/mcp"
)

// controlCapture is a sender that also listens for control updates, the way
// the console does.
type controlCapture struct {
	mcpTestSender
	mu      sync.Mutex
	updates []MCPConnectUpdate
}

// The sender can be replaced after session/new while its background notifier
// is still running, including on a session with no configured servers.
func TestMCPNotificationsDuringSenderReplacement(t *testing.T) {
	m := &Manager{}
	st := &State{}
	gen, _, _ := st.beginBackgroundMCP(nil)
	st.finishBackgroundMCP(gen, nil)
	capture := &controlCapture{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			m.SetServer(capture)
			m.SetServer(nil)
		}
	}()
	for range 1000 {
		m.sendMCPConnectUpdate(st)
	}
	wg.Wait()
	m.SetServer(capture)
	m.sendMCPConnectUpdate(st)
	if update, ok := capture.shown(); !ok || !update.Done {
		t.Fatal("the current sender did not receive the completed MCP snapshot")
	}
}

func (c *controlCapture) SendControlUpdate(_ string, update any) error {
	if u, ok := update.(MCPConnectUpdate); ok {
		c.mu.Lock()
		c.updates = append(c.updates, u)
		c.mu.Unlock()
	}
	return nil
}

// shown is the snapshot a surface renders after the updates so far: the
// newest by generation, since the console drops one older than the last it
// applied (applyMCPConnect). The connect's notifier and a reload that
// supersedes it send from two goroutines, so the notifier can deliver a
// snapshot it read before the reload after the reload's own, and the last
// update delivered is not always the newest.
func (c *controlCapture) shown() (MCPConnectUpdate, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.updates) == 0 {
		return MCPConnectUpdate{}, false
	}
	newest := c.updates[0]
	for _, u := range c.updates[1:] {
		if u.Generation >= newest.Generation {
			newest = u
		}
	}
	return newest, true
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// backgroundFixture is a manager that defers connects, with one gated server
// the test releases when it wants the dial to settle.
type backgroundFixture struct {
	mgr     *Manager
	st      *State
	sender  *controlCapture
	release string
	started string
}

// setup, when given, adjusts the manager before the session is created - the
// dial reads the manager's settings from its worker, so a test changes them
// here and never afterwards.
func newBackgroundFixture(t *testing.T, runner AgentRunner, setup func(*Manager)) *backgroundFixture {
	t.Helper()
	dir := t.TempDir()
	f := &backgroundFixture{
		sender:  &controlCapture{},
		release: filepath.Join(dir, "release"),
		started: filepath.Join(dir, "started"),
	}
	f.mgr = NewManager(reloadTestConfig(gatedMCPServer("gated", f.started, f.release)), f.sender, runner, slog.Default(), t.TempDir(), nil)
	f.mgr.SetBackgroundMCPConnect(true)
	if setup != nil {
		setup(f.mgr)
	}
	begin := time.Now()
	res, err := f.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(begin); took > 2*time.Second {
		t.Fatalf("session/new took %s while a server was still connecting", took)
	}
	f.st = f.mgr.SessionByID(res.SessionID)
	t.Cleanup(f.mgr.CloseMCP)
	t.Cleanup(f.st.CloseAll)
	return f
}

func (f *backgroundFixture) releaseServer() {
	_ = os.WriteFile(f.release, []byte("1"), 0o644)
}

// TestBackgroundConnectReturnsBeforeServersAnswer: session/new returns while
// the server is still in its handshake, the snapshot says so, and the client
// is installed once the server answers.
func TestBackgroundConnectReturnsBeforeServersAnswer(t *testing.T) {
	f := newBackgroundFixture(t, nil, nil)
	if got := f.st.GetMCPClients(); len(got) != 0 {
		t.Fatalf("session/new installed %d clients before the server answered", len(got))
	}
	snap, recorded := f.st.MCPConnectSnapshot()
	if !recorded || snap.Done || len(snap.Servers) != 1 || snap.Servers[0].State != MCPConnectStateConnecting {
		t.Fatalf("snapshot = %+v (recorded %v), want one server connecting", snap, recorded)
	}
	if connected, total := snap.Counts(); connected != 0 || total != 1 {
		t.Fatalf("counts = %d/%d, want 0/1", connected, total)
	}
	f.releaseServer()
	if !waitUntil(t, 10*time.Second, func() bool { s, _ := f.st.MCPConnectSnapshot(); return s.Done }) {
		t.Fatal("the dial never settled after the release")
	}
	snap, _ = f.st.MCPConnectSnapshot()
	if snap.Servers[0].State != MCPConnectStateConnected || snap.Servers[0].Tools != 1 {
		t.Fatalf("server after release = %+v, want connected with 1 tool", snap.Servers[0])
	}
	if got := clientNames(f.st); len(got) != 1 || got[0] != "gated" {
		t.Fatalf("clients = %v, want [gated]", got)
	}
	// The notifier delivers from a goroutine of its own once the record says
	// done, so the surface hears it a moment after the record changed.
	if !waitUntil(t, 5*time.Second, func() bool { got, ok := f.sender.shown(); return ok && got.Done }) {
		got, ok := f.sender.shown()
		t.Fatalf("control update shown = %+v (%v), want done", got, ok)
	}
}

// TestTurnWaitsForBackgroundConnect: a prompt sent while the server connects
// starts only once the dial settled, with the server's tools in place.
func TestTurnWaitsForBackgroundConnect(t *testing.T) {
	entered := make(chan int, 1)
	runner := func(_ context.Context, st *State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		entered <- len(st.GetMCPClients())
		return string(acp.StopReasonEndTurn), nil
	}
	f := newBackgroundFixture(t, runner, nil)
	turn := make(chan error, 1)
	go func() {
		_, err := f.mgr.HandleSessionPrompt(context.Background(), acp.SessionPromptParams{
			SessionID: f.st.GetID(), Prompt: []acp.ContentBlock{{Type: "text", Text: "use the tool"}},
		})
		turn <- err
	}()
	select {
	case n := <-entered:
		t.Fatalf("the turn started with %d clients before the dial settled", n)
	case <-time.After(300 * time.Millisecond):
	}
	f.releaseServer()
	select {
	case n := <-entered:
		if n != 1 {
			t.Fatalf("the turn saw %d clients, want 1", n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the turn never started after the release")
	}
	if err := <-turn; err != nil {
		t.Fatal(err)
	}
}

// TestCancelDuringWaitEndsTurn: Stop while the turn waits for the dial ends
// the wait at once. The runner is entered with the cancelled context, as after
// any other stopped step of a turn, and the turn is over before the server
// has answered.
func TestCancelDuringWaitEndsTurn(t *testing.T) {
	entered := make(chan error, 1)
	runner := func(ctx context.Context, _ *State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		entered <- ctx.Err()
		if ctx.Err() != nil {
			return string(acp.StopReasonCancelled), nil
		}
		return string(acp.StopReasonEndTurn), nil
	}
	f := newBackgroundFixture(t, runner, nil)
	turn := make(chan *acp.SessionPromptResult, 1)
	go func() {
		res, err := f.mgr.HandleSessionPrompt(context.Background(), acp.SessionPromptParams{
			SessionID: f.st.GetID(), Prompt: []acp.ContentBlock{{Type: "text", Text: "hi"}},
		})
		if err != nil {
			t.Error(err)
		}
		turn <- res
	}()
	if !waitUntil(t, 5*time.Second, func() bool { return f.mgr.SessionTurnActiveInProcess(f.st.GetID()) }) {
		t.Fatal("the turn never became active")
	}
	select {
	case <-entered: //nolint:errcheck // Only completion is needed here.
		t.Fatal("the runner started before the dial settled or the turn was stopped")
	case <-time.After(200 * time.Millisecond):
	}
	f.mgr.HandleSessionCancel(acp.SessionCancelParams{SessionID: f.st.GetID()})
	select {
	case err := <-entered:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("the runner was entered with context error %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stopped turn never reached its runner")
	}
	select {
	case res := <-turn:
		if res == nil || res.StopReason != acp.StopReasonCancelled {
			t.Fatalf("result = %+v, want a cancelled turn", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled turn did not end")
	}
	if snap, _ := f.st.MCPConnectSnapshot(); snap.Done {
		t.Fatal("the dial settled although the server was never released")
	}
	f.releaseServer()
}

// TestReloadDuringPendingConnectLeavesNoDuplicates: a settings change while
// the dial is pending supersedes it; the late result is closed, not installed.
func TestReloadDuringPendingConnectLeavesNoDuplicates(t *testing.T) {
	f := newBackgroundFixture(t, nil, func(m *Manager) { m.SetMCPConnectTimeoutForTest(2 * time.Second) })
	next := reloadTestConfig(gatedMCPServer("gated", f.started, f.release), reloadTestMCPServer("good"))
	f.mgr.ReplaceConfig(next)
	f.releaseServer()
	if !waitUntil(t, 10*time.Second, func() bool { return !f.st.backgroundMCPRunning() }) {
		t.Fatal("the superseded dial never settled")
	}
	time.Sleep(200 * time.Millisecond) // a late install would land here
	names := clientNames(f.st)
	seen := map[string]int{}
	for _, n := range names {
		seen[n]++
	}
	for name, n := range seen {
		if n > 1 {
			t.Fatalf("server %q installed %d times: %v", name, n, names)
		}
	}
	if seen["good"] != 1 {
		t.Fatalf("clients = %v, want the reloaded server good", names)
	}
}

// TestReloadDuringPendingConnectTellsTheSurface: a reload that supersedes a
// background connect sends the surface a last snapshot of that connect,
// done, with the server it cut short marked cancelled rather than failed -
// otherwise the console's footer would count a server nobody dials any more.
func TestReloadDuringPendingConnectTellsTheSurface(t *testing.T) {
	f := newBackgroundFixture(t, nil, func(m *Manager) { m.SetMCPConnectTimeoutForTest(300 * time.Millisecond) })
	f.mgr.ReplaceConfig(reloadTestConfig(reloadTestMCPServer("good")))
	// The reload has sent its snapshot by the time ReplaceConfig returns. The
	// connect's first update, read before the reload, can still land after
	// it, and the surface drops that one by its generation.
	got, ok := f.sender.shown()
	if !ok || !got.Done {
		t.Fatalf("control update shown = %+v (%v), want a done snapshot", got, ok)
	}
	if len(got.Servers) != 1 || got.Servers[0].State != MCPConnectStateCancelled {
		t.Fatalf("servers = %+v, want the gated server cancelled", got.Servers)
	}
	f.releaseServer()
}

// TestCloseAllUnblocksWait: tearing the session down releases a turn waiting
// for its dial.
func TestCloseAllUnblocksWait(t *testing.T) {
	f := newBackgroundFixture(t, nil, nil)
	waited := make(chan error, 1)
	go func() { waited <- f.st.WaitMCPConnect(context.Background()) }()
	select {
	case <-waited: //nolint:errcheck // Only completion is needed here.
		t.Fatal("the wait ended before the dial settled")
	case <-time.After(200 * time.Millisecond):
	}
	f.st.CloseAll()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CloseAll did not release the wait")
	}
	f.releaseServer()
}

// New sessions connect asynchronously on every surface.
func TestAllSurfacesRecordBackgroundConnect(t *testing.T) {
	mgr := NewManager(reloadTestConfig(reloadTestMCPServer("good")), mcpTestSender{}, nil, slog.Default(), t.TempDir(), nil)
	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(res.SessionID)
	t.Cleanup(st.CloseAll)
	if _, recorded := st.MCPConnectSnapshot(); !recorded {
		t.Fatal("new session did not record its background connect")
	}
	if err := st.WaitMCPConnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clientNames(st); len(got) != 1 {
		t.Fatalf("clients = %v, want [good]", got)
	}
}

// TestHeldServerIsReportedNotDialed: a project declaration the gate holds
// shows as held in the snapshot and is not counted as connectable.
func TestHeldServerIsReportedNotDialed(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".foxxycode"), 0o755); err != nil {
		t.Fatal(err)
	}
	project := `{"mcpServers":{"project-tool":{"command":"` + filepath.ToSlash(os.Args[0]) + `","args":["-test.run=^TestGatedMCPHelperProcess$"]}}}`
	if err := os.WriteFile(filepath.Join(cwd, ".foxxycode", "mcp.json"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := reloadTestConfig(reloadTestMCPServer("good"))
	cfg.MCP.ProjectTrust = config.ProjectTrustAsk
	cfg.Paths.Home = t.TempDir()
	mgr := NewManager(cfg, mcpTestSender{}, nil, slog.Default(), cwd, nil)
	mgr.SetBackgroundMCPConnect(true)
	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(res.SessionID)
	t.Cleanup(st.CloseAll)
	if !waitUntil(t, 10*time.Second, func() bool { s, _ := st.MCPConnectSnapshot(); return s.Done }) {
		t.Fatal("the dial never settled")
	}
	snap, _ := st.MCPConnectSnapshot()
	var held *MCPServerConnect
	for i := range snap.Servers {
		if snap.Servers[i].Name == "project-tool" {
			held = &snap.Servers[i]
		}
	}
	if held == nil || held.State != MCPConnectStateHeld || held.Hint == "" {
		t.Fatalf("project server = %+v, want held with a hint", held)
	}
	if connected, total := snap.Counts(); connected != 1 || total != 1 {
		t.Fatalf("counts = %d/%d, want 1/1 (the held server is not dialed)", connected, total)
	}
}

// TestReloadClearsTheConnectRecord: once a reload has replaced the servers,
// the record of the background connect no longer describes the session and
// is gone, so a surface adopting the session later shows no stale notice;
// the snapshot the surface shows says the connect it was following is over,
// ordered after every snapshot read before it.
func TestReloadClearsTheConnectRecord(t *testing.T) {
	f := newBackgroundFixture(t, nil, func(m *Manager) { m.SetMCPConnectTimeoutForTest(300 * time.Millisecond) })
	first, _ := f.st.MCPConnectSnapshot()
	if first.Generation == 0 {
		t.Fatal("the first snapshot carries no generation")
	}
	// The gated stub writes this marker as soon as it spawns. A spawn still
	// in flight at teardown lands that write while t.TempDir() removes the
	// fixture dir and fails the cleanup with "directory not empty". The
	// connect stays in flight regardless: the stub still waits for release.
	if !waitUntil(t, 10*time.Second, func() bool {
		_, err := os.Stat(f.started)
		return err == nil
	}) {
		t.Fatal("the gated server did not start")
	}
	f.mgr.ReplaceConfig(reloadTestConfig(reloadTestMCPServer("good")))
	if _, recorded := f.st.MCPConnectSnapshot(); recorded {
		t.Fatal("the connect record outlived the reload that replaced the servers")
	}
	got, _ := f.sender.shown()
	if !got.Done || got.Generation <= first.Generation {
		t.Fatalf("update shown = %+v, want the followed connect done, ordered after the first snapshot (generation %d)", got, first.Generation)
	}
	f.releaseServer()
}

// TestApprovedProjectServerConnectsInTheBackground: a project declaration the
// operator approved is dialed like a configured one, through the gate, and
// installed when it answers.
func TestApprovedProjectServerConnectsInTheBackground(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".foxxycode"), 0o755); err != nil {
		t.Fatal(err)
	}
	started := filepath.Join(t.TempDir(), "started")
	entry := config.MCPJSONServer{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestGatedMCPHelperProcess$"},
		Env:     map[string]string{reloadTestMCPHelperEnv: "1", gatedMCPStartedEnv: started},
	}
	if err := config.UpsertMCPJSONServer(config.MCPJSONPath(cwd), "project-tool", entry); err != nil {
		t.Fatal(err)
	}
	cfg := reloadTestConfig()
	cfg.MCP.ProjectTrust = config.ProjectTrustAsk
	cfg.Paths.Home = home
	servers, err := mcp.ListManagedServers(cfg, cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, srv := range servers {
		if srv.Config.Name == "project-tool" {
			if err := mcp.NewTrustStore(home).Approve(cwd, config.MCPJSONPath(cwd), srv.Config); err != nil {
				t.Fatal(err)
			}
		}
	}
	mgr := NewManager(cfg, mcpTestSender{}, nil, slog.Default(), cwd, nil)
	mgr.SetBackgroundMCPConnect(true)
	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(res.SessionID)
	t.Cleanup(st.CloseAll)
	if !waitUntil(t, 10*time.Second, func() bool { s, _ := st.MCPConnectSnapshot(); return s.Done }) {
		t.Fatal("the dial never settled")
	}
	snap, _ := st.MCPConnectSnapshot()
	if len(snap.Servers) != 1 || snap.Servers[0].Name != "project-tool" || snap.Servers[0].State != MCPConnectStateConnected {
		t.Fatalf("snapshot = %+v, want project-tool connected", snap.Servers)
	}
	if got := clientNames(st); len(got) != 1 || got[0] != "project-tool" {
		t.Fatalf("clients = %v, want [project-tool]", got)
	}
	if _, err := os.Stat(started); err != nil {
		t.Fatal("the approved server was never spawned")
	}
	// Withdrawing the approval closes the server, and the record a surface
	// adopting the session reads says it waits for approval again.
	if err := mgr.SetMCPTrust(context.Background(), cwd, "project-tool", "", false); err != nil {
		t.Fatal(err)
	}
	if got := clientNames(st); len(got) != 0 {
		t.Fatalf("clients after the approval was withdrawn = %v, want none", got)
	}
	snap, _ = st.MCPConnectSnapshot()
	if len(snap.Servers) != 1 || snap.Servers[0].State != MCPConnectStateHeld {
		t.Fatalf("snapshot after the approval was withdrawn = %+v, want project-tool held", snap.Servers)
	}
}

// TestRevokedWhileConnectingIsNotInstalled: a project server whose approval
// is withdrawn while the background connect dials it is not installed when
// it answers, and the record shows it cancelled rather than connected.
func TestRevokedWhileConnectingIsNotInstalled(t *testing.T) {
	cwd, home, dir := t.TempDir(), t.TempDir(), t.TempDir()
	release := filepath.Join(dir, "release")
	entry := config.MCPJSONServer{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestGatedMCPHelperProcess$"},
		Env: map[string]string{
			reloadTestMCPHelperEnv: "1",
			gatedMCPStartedEnv:     filepath.Join(dir, "started"),
			gatedMCPReleaseEnv:     release,
		},
	}
	if err := config.UpsertMCPJSONServer(config.MCPJSONPath(cwd), "project-tool", entry); err != nil {
		t.Fatal(err)
	}
	cfg := reloadTestConfig()
	cfg.MCP.ProjectTrust = config.ProjectTrustAsk
	cfg.Paths.Home = home
	servers, err := mcp.ListManagedServers(cfg, cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, srv := range servers {
		if srv.Config.Name == "project-tool" {
			if err := mcp.NewTrustStore(home).Approve(cwd, config.MCPJSONPath(cwd), srv.Config); err != nil {
				t.Fatal(err)
			}
		}
	}
	mgr := NewManager(cfg, &controlCapture{}, nil, slog.Default(), cwd, nil)
	mgr.SetBackgroundMCPConnect(true)
	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(res.SessionID)
	t.Cleanup(st.CloseAll)
	if _, err := mcp.NewTrustStore(home).Revoke(cwd, "project-tool"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(t, 10*time.Second, func() bool { return !st.backgroundMCPRunning() }) {
		t.Fatal("the dial never settled")
	}
	if names := clientNames(st); len(names) != 0 {
		t.Fatalf("clients = %v, want none: the approval was withdrawn while the server connected", names)
	}
	snap, _ := st.MCPConnectSnapshot()
	if len(snap.Servers) != 1 || snap.Servers[0].State != MCPConnectStateCancelled {
		t.Fatalf("snapshot = %+v, want the server cancelled", snap.Servers)
	}
}

func TestDeclarationChangedWhileConnectingIsNotInstalled(t *testing.T) {
	home := t.TempDir()
	f := newBackgroundFixture(t, nil, func(m *Manager) { m.activeCfg().Paths.Home = home })
	if !waitForFile(t, f.started, 5*time.Second) {
		t.Fatal("the original declaration never started")
	}
	if err := config.UpsertMCPJSONServer(config.GlobalMCPJSONPath(home), "gated", config.MCPJSONServer{
		Command: os.Args[0], Args: []string{"-test.run=^TestMCPSettingsHelperProcess$"},
		Env: map[string]string{reloadTestMCPHelperEnv: "1"},
	}); err != nil {
		t.Fatal(err)
	}
	f.releaseServer()
	if err := f.st.WaitMCPReady(t.Context()); err != nil {
		t.Fatal(err)
	}
	if names := clientNames(f.st); len(names) != 0 {
		t.Fatalf("clients = %v, want no obsolete declaration installed", names)
	}
	if !f.st.hasPendingMCPServers() {
		t.Fatal("the new declaration was not parked for the next turn")
	}
}
