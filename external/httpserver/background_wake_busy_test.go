//go:build http

package httpserver

// Fork-specific: upstream's session turn lock queues, so a wake landing mid-turn
// simply waits. Ours fails fast with ErrSessionTurnBusy (which is what lets the
// composer answer 409 session_busy instead of hanging), and BackgroundWaker
// drops a batch whose run returned an error. Without a retry here, a task
// finishing while the user is mid-turn loses its notification outright.

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// busySession is a server whose first turn parks inside the runner, so the
// session turn lock is held for as long as the test wants it held.
type busySession struct {
	srv       *Server
	mgr       *session.Manager
	sessionID string
	// entered closes once the first turn is inside the runner. The runner only
	// runs after the turn lock has been acquired, so this is a definitive signal
	// that the session is busy -- unlike probing the lock, which is not portable:
	// on unix it is an flock with stale-lock breaking, so a probe from this same
	// process can take it away from the turn that holds it.
	entered chan struct{}
	release func()
}

// wakeBusySession builds a server whose first turn parks in the runner until
// release is called, so the session turn lock can be held on demand.
func wakeBusySession(t *testing.T) *busySession {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("home: %v", err)
	}
	sessRoot := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessRoot, 0o755); err != nil {
		t.Fatalf("sessions: %v", err)
	}

	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}

	holdCh := make(chan struct{})
	var released atomic.Bool
	closeRelease := func() {
		if released.CompareAndSwap(false, true) {
			close(holdCh)
		}
	}

	entered := make(chan struct{})
	var turns atomic.Int32
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		if turns.Add(1) == 1 {
			close(entered)
			<-holdCh
		}
		return string(acp.StopReasonEndTurn), nil
	}

	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, &session.FileStore{Root: sessRoot})
	srv := New(cfg, mgr, slog.Default(), root)
	t.Cleanup(func() {
		closeRelease()
		srv.Drain()
		bgtask.Default().SetDraining(false)
	})

	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: root})
	if err != nil {
		t.Fatalf("session new: %v", err)
	}
	return &busySession{
		srv: srv, mgr: mgr, sessionID: res.SessionID,
		entered: entered, release: closeRelease,
	}
}

// occupy starts a turn and waits until it is inside the runner, so a wake
// racing ahead of it cannot make the test pass without ever retrying.
func (b *busySession) occupy(t *testing.T) (done chan struct{}) {
	t.Helper()

	done = make(chan struct{})
	go func() {
		defer close(done)
		_, _ = b.mgr.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
			SessionID: b.sessionID,
			Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "user turn"}},
		}, planRunNoopSender{}, nil)
	}()

	select {
	case <-b.entered:
		return done
	case <-time.After(15 * time.Second):
		t.Fatalf("user turn never reached the runner, so the session never went busy")
		return done
	}
}

// wake runs the wake turn in the background and reports its result.
func (b *busySession) wake() chan error {
	out := make(chan error, 1)
	go func() {
		out <- b.srv.runWakeTurn(context.Background(), b.sessionID, "a background task finished")
	}()
	return out
}

func TestWakeTurnRetriesWhileTheSessionIsBusy(t *testing.T) {
	b := wakeBusySession(t)
	userTurnDone := b.occupy(t)
	wakeErr := b.wake()

	// Upstream's waker would have returned ErrSessionTurnBusy by now and the
	// notification would be gone.
	select {
	case err := <-wakeErr:
		t.Fatalf("wake returned %v instead of retrying while the session was busy", err)
	case <-time.After(1500 * time.Millisecond):
	}

	b.release()
	<-userTurnDone

	select {
	case err := <-wakeErr:
		if err != nil {
			t.Fatalf("wake failed after the session freed up: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("wake never landed after the session freed up")
	}
}

// A wake retrying against a session that never frees up must not pin a
// goroutine, and bgWG with it, past shutdown.
func TestWakeTurnStopsRetryingOnceDraining(t *testing.T) {
	b := wakeBusySession(t)
	userTurnDone := b.occupy(t)
	// The occupying turn keeps writing into the session bundle, and on Windows a
	// still-open handle makes t.TempDir's cleanup fail with "directory is not
	// empty". Registered after t.TempDir, so it runs before it.
	t.Cleanup(func() {
		b.release()
		<-userTurnDone
	})

	wakeErr := b.wake()

	// Let it enter the retry loop, then close the pool the way Drain does.
	time.Sleep(1200 * time.Millisecond)
	bgtask.Default().SetDraining(true)

	select {
	case err := <-wakeErr:
		if err != nil {
			t.Fatalf("draining wake returned %v, want nil", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("wake kept retrying after the pool started draining")
	}
}
