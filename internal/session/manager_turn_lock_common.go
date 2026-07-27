package session

import (
	"context"
	"errors"
	"sync"
	"time"
)

// stubTurnMu serializes prompt turns per session when there is no persisted bundle directory
// or on platforms without flock (see manager_turn_lock_unix.go / manager_turn_lock_stub.go).
func (m *Manager) acquireStubTurnLock(sessionID string) (unlock func(), _ error) {
	v, _ := m.stubTurnMu.LoadOrStore(sessionID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, ErrSessionTurnBusy
	}
	return func() { mu.Unlock() }, nil
}

// turnLockRetryInterval paces AcquireComposerTurnLockWaiting between attempts.
const turnLockRetryInterval = 25 * time.Millisecond

// AcquireComposerTurnLockWaiting is AcquireComposerTurnLock with a bounded wait for the
// previous turn to let go. A cancelled turn does not free the session the instant
// session/cancel returns - it still has to unwind, persist, and diff the workspace - so a
// user who hits Stop and immediately sends again (typically to retry on a faster model)
// would otherwise be told the session is busy. Waits until the lock is free, ctx ends, or
// maxWait elapses; then reports ErrSessionTurnBusy like the non-waiting call.
func (m *Manager) AcquireComposerTurnLockWaiting(ctx context.Context, sessionID string, st *State, maxWait time.Duration) (unlock func(), err error) {
	deadline := time.Now().Add(maxWait)
	for {
		unlock, err = m.acquirePromptTurnLock(sessionID, st)
		if err == nil || !errors.Is(err, ErrSessionTurnBusy) {
			return unlock, err
		}
		if maxWait <= 0 || !time.Now().Before(deadline) {
			return nil, ErrSessionTurnBusy
		}
		timer := time.NewTimer(turnLockRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ErrSessionTurnBusy
		case <-timer.C:
		}
	}
}
