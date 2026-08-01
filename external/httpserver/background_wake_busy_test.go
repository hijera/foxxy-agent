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

// wakeBusySession builds a server whose first turn parks in the runner until the
// returned channel is closed, so the session turn lock can be held on demand.
func wakeBusySession(t *testing.T) (srv *Server, mgr *session.Manager, sessionID string, release func()) {
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

	var turns atomic.Int32
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		if turns.Add(1) == 1 {
			<-holdCh
		}
		return string(acp.StopReasonEndTurn), nil
	}

	mgr = session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, &session.FileStore{Root: sessRoot})
	srv = New(cfg, mgr, slog.Default(), root)
	t.Cleanup(func() {
		closeRelease()
		srv.Drain()
		bgtask.Default().SetDraining(false)
	})

	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: root})
	if err != nil {
		t.Fatalf("session new: %v", err)
	}
	return srv, mgr, res.SessionID, closeRelease
}

// occupySession starts a turn and waits until it actually holds the turn lock,
// so a wake racing ahead of it cannot make the test pass without retrying.
func occupySession(t *testing.T, mgr *session.Manager, sessionID string) (done chan struct{}) {
	t.Helper()

	done = make(chan struct{})
	go func() {
		defer close(done)
		_, _ = mgr.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
			SessionID: sessionID,
			Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "user turn"}},
		}, planRunNoopSender{}, nil)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		unlock, err := mgr.AcquireComposerTurnLockWaiting(context.Background(), sessionID,
			mgr.SessionByID(sessionID), 10*time.Millisecond)
		if err != nil {
			return done
		}
		// We took it instead: the turn has not reached the lock yet.
		unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("user turn never took the session turn lock")
	return done
}

func TestWakeTurnRetriesWhileTheSessionIsBusy(t *testing.T) {
	srv, mgr, sessionID, release := wakeBusySession(t)
	userTurnDone := occupySession(t, mgr, sessionID)

	wakeErr := make(chan error, 1)
	go func() {
		wakeErr <- srv.runWakeTurn(context.Background(), sessionID, "a background task finished")
	}()

	// Upstream's waker would have returned ErrSessionTurnBusy by now and the
	// notification would be gone.
	select {
	case err := <-wakeErr:
		t.Fatalf("wake returned %v instead of retrying while the session was busy", err)
	case <-time.After(1500 * time.Millisecond):
	}

	release()
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
	srv, mgr, sessionID, _ := wakeBusySession(t)
	occupySession(t, mgr, sessionID)

	wakeErr := make(chan error, 1)
	go func() {
		wakeErr <- srv.runWakeTurn(context.Background(), sessionID, "a background task finished")
	}()

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
