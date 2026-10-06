package session_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// managerWithStore builds a manager over a fresh on-disk store, plus the sender whose
// updates the tests count loads by.
func managerWithStore(t *testing.T) (*session.Manager, *captureSender, string) {
	t.Helper()
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	if err := os.MkdirAll(store.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	sender := &captureSender{}
	return session.NewManager(testConfig(), sender, noopRunner, slog.Default(), cwd, store), sender, cwd
}

// countLoads reports how many times a session was replayed from disk. loadSessionFromDisk
// emits exactly one available-commands update per load, so this is a load counter.
func countLoads(sender *captureSender) int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	n := 0
	for _, u := range sender.ups {
		if _, ok := u.(acp.AvailableCommandsUpdate); ok {
			n++
		}
	}
	return n
}

// persistSession creates a session, then forgets it in memory so the next Ensure has to
// come off disk - the state the SPA's boot fan-out finds.
func persistSession(t *testing.T, m *session.Manager, cwd string) string {
	t.Helper()
	res, err := m.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	m.ForgetLiveSession(res.SessionID)
	return res.SessionID
}

// The SPA opens ~10 per-session endpoints at once when a panel boots. Each one used to run a
// full load, and every load closed the previous one's state - so the requests cannibalised
// each other while holding all of the webview's connections.
func TestConcurrentEnsureLoadsTheSessionOnce(t *testing.T) {
	m, sender, cwd := managerWithStore(t)
	id := persistSession(t, m, cwd)

	sender.mu.Lock()
	sender.ups = nil
	sender.mu.Unlock()

	const callers = 8
	states := make([]*session.State, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			states[i], errs[i] = m.EnsureHTTPSession(context.Background(), id, cwd)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}
	for i := 1; i < callers; i++ {
		if states[i] != states[0] {
			t.Fatalf("caller %d got a different session object than caller 0", i)
		}
	}
	if got := countLoads(sender); got != 1 {
		t.Fatalf("expected exactly one load from disk, got %d", got)
	}
}

// A session/new that reopens a persisted bundle parks its replay until the ACP client has
// the response naming the id. Nothing on the HTTP path ever writes that response, so the
// flight layer has to publish the parked work itself - otherwise a whole transcript stays
// closed over in the state and the panel gets no history at all.
func TestHTTPLoadPublishesTheReplayInsteadOfParkingIt(t *testing.T) {
	m, sender, cwd := managerWithStore(t)
	id := persistSession(t, m, cwd)

	sender.mu.Lock()
	sender.ups = nil
	sender.mu.Unlock()

	if _, err := m.EnsureHTTPSession(context.Background(), id, cwd); err != nil {
		t.Fatalf("EnsureHTTPSession: %v", err)
	}

	// The slash catalogue is the update HandleSessionReady always emits, so seeing it means
	// the drain ran rather than that the load happened to write inline.
	if got := countLoads(sender); got != 1 {
		t.Fatalf("expected one available-commands update after an HTTP load, got %d", got)
	}
	st := m.SessionByID(id)
	if st == nil {
		t.Fatal("session not live after EnsureHTTPSession")
	}
	if st.HasPendingReadyNotifyForTest() {
		t.Fatal("HTTP load left a parked replay nothing will ever publish")
	}
}

// The leader runs on a context detached from its own request: a webview that aborts its
// fetch (session switch, reload) must not take the other waiters down with it.
func TestEnsureSurvivesACancelledLeader(t *testing.T) {
	m, _, cwd := managerWithStore(t)
	id := persistSession(t, m, cwd)

	leaderCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := m.EnsureHTTPSession(leaderCtx, id, cwd); err != nil {
		t.Fatalf("a cancelled caller should still get the session: %v", err)
	}
	if st := m.SessionByID(id); st == nil {
		t.Fatal("expected the session to be live after the cancelled leader finished")
	}
}

// Read-only routes must not conjure a session for an id that was never persisted, or a stale
// deep link would silently create an empty chat instead of answering 404.
func TestLoadPersistedSessionRejectsAMissingSnapshot(t *testing.T) {
	m, _, cwd := managerWithStore(t)

	_, err := m.LoadPersistedSession(context.Background(), "sess_deadbeefdeadbeefdeadbeef", cwd)
	if !errors.Is(err, session.ErrSessionSnapshotMissing) {
		t.Fatalf("expected ErrSessionSnapshotMissing, got %v", err)
	}
	if st := m.SessionByID("sess_deadbeefdeadbeefdeadbeef"); st != nil {
		t.Fatal("a read-only load must not create a session")
	}
}

// A turn arriving while read-only routes are asking for the same unknown id still has to
// create it: the two share a flight, and the creating caller must not inherit the "missing"
// answer the read-only one settled for.
func TestEnsureCreatesAfterJoiningAReadOnlyFlight(t *testing.T) {
	m, _, cwd := managerWithStore(t)
	id := "sess_0123456789abcdef01234567"

	var wg sync.WaitGroup
	start := make(chan struct{})
	var readErr error
	var created *session.State
	var createErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, readErr = m.LoadPersistedSession(context.Background(), id, cwd)
	}()
	go func() {
		defer wg.Done()
		<-start
		created, createErr = m.EnsureHTTPSession(context.Background(), id, cwd)
	}()
	close(start)
	wg.Wait()

	if readErr != nil && !errors.Is(readErr, session.ErrSessionSnapshotMissing) {
		t.Fatalf("read-only caller: unexpected error %v", readErr)
	}
	if createErr != nil {
		t.Fatalf("creating caller: %v", createErr)
	}
	if created == nil || created.GetID() != id {
		t.Fatalf("expected the session to be created with the pinned id, got %#v", created)
	}
}
