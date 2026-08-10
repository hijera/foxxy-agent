package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
)

// ErrSessionSnapshotMissing is returned by LoadPersistedSession for an id that has no
// snapshot on disk. Read-only routes map it to 404 instead of creating an empty chat.
var ErrSessionSnapshotMissing = errors.New("session snapshot not found")

// loadFlight is one in-progress load of a session id, shared by everyone who asks for that
// id while it runs.
type loadFlight struct {
	done chan struct{}
	// create records whether the leader was allowed to create the session when no snapshot
	// existed, so a follower that *is* allowed does not inherit a "missing" answer.
	create bool
	st     *State
	err    error
}

// How many times a caller may re-enter the flight loop before giving up. Only the
// read-only-flight retry can loop at all, and one retry is already the pathological case.
const maxLoadFlightAttempts = 4

// ensureSessionSingleFlight resolves sessionID to a live session, running at most one load
// per id at a time.
//
// A panel opening a chat fans out roughly ten per-session GETs at once, and the turn's POST
// follows them. Without this, every one of those requests ran a full loadSessionFromDisk -
// which starts by closing and dropping whatever the previous loader had just published, so
// the requests undid each other while holding all of the webview's six connections, and the
// turn could not even leave the browser.
//
// The leader runs on a context detached from its caller: an aborted fetch (session switch,
// reload) must not cancel a load that the other waiters are relying on.
func (m *Manager) ensureSessionSingleFlight(ctx context.Context, sessionID, defaultCWD string, allowCreate bool) (*State, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil, fmt.Errorf("empty session id")
	}
	if err := ValidateFolderSessionID(id); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < maxLoadFlightAttempts; attempt++ {
		if existing := m.getSession(id); existing != nil {
			return existing, nil
		}

		mine := &loadFlight{done: make(chan struct{}), create: allowCreate}
		actual, joined := m.loadFlights.LoadOrStore(id, mine)
		if !joined {
			// Winning the slot is not enough to justify a load. A leader that finished
			// between the getSession above and this LoadOrStore has already published the
			// session and removed its flight, so there is nothing left to join - and
			// loading again would not merely repeat the read: loadSessionFromDisk drops
			// the live state first, so the second load closes the MCP clients of the
			// session the first one just brought up. Re-check before committing.
			st := m.getSession(id)
			var err error
			if st == nil {
				st, err = m.loadOrCreateSession(context.WithoutCancel(ctx), id, defaultCWD, allowCreate)
			}
			mine.st, mine.err = st, err
			m.loadFlights.Delete(id)
			close(mine.done)
			return st, err
		}

		other, ok := actual.(*loadFlight)
		if !ok {
			return nil, fmt.Errorf("internal: bad load flight for %s", id)
		}
		select {
		case <-other.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if other.err == nil {
			return other.st, nil
		}
		lastErr = other.err
		// The flight we joined was read-only and found nothing. We are allowed to create,
		// so run our own.
		if allowCreate && !other.create && errors.Is(other.err, ErrSessionSnapshotMissing) {
			continue
		}
		return nil, other.err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("session load did not settle: %s", id)
	}
	return nil, lastErr
}

// loadOrCreateSession is the body of one flight: reopen the persisted bundle, or - only when
// the caller is a turn rather than a read-only route - create an empty one under the id.
//
// Both branches finish with HandleSessionReady, which the ACP server calls after writing the
// session/new or session/load response. Nothing writes a response here - the session updates
// go to a separate SSE stream - so there is no ordering to wait for, but the call still has to
// happen: it is what publishes the slash-command catalogue, and over HTTP no ACP dispatch will
// ever make it.
func (m *Manager) loadOrCreateSession(ctx context.Context, id, defaultCWD string, allowCreate bool) (*State, error) {
	if m.store != nil && m.store.HasPersistedSnapshot(id) {
		if _, err := m.HandleSessionLoad(ctx, acp.SessionLoadParams{
			SessionID: id,
			CWD:       defaultCWD,
		}); err != nil {
			return nil, err
		}
		st := m.getSession(id)
		if st == nil {
			return nil, fmt.Errorf("session load incomplete: %s", id)
		}
		m.HandleSessionReady(id)
		return st, nil
	}
	if !allowCreate {
		return nil, ErrSessionSnapshotMissing
	}

	// preferredNewSessionID is a single slot on the manager: the pin and the create that
	// consumes it have to be one atomic step, or two sessions being created at once can
	// swap ids. Per-id single-flight does not cover this - the ids differ by definition.
	m.newSessionMu.Lock()
	m.SetPreferredSessionID(id)
	res, err := m.HandleSessionNew(ctx, acp.SessionNewParams{CWD: defaultCWD})
	m.newSessionMu.Unlock()
	if err != nil {
		return nil, err
	}
	if res.SessionID != id {
		return nil, fmt.Errorf("session id mismatch creating %s vs %s", id, res.SessionID)
	}
	st := m.getSession(id)
	if st == nil {
		return nil, fmt.Errorf("internal session missing after new: %s", id)
	}
	m.HandleSessionReady(id)
	return st, nil
}

// LoadPersistedSession returns the session for an id that already exists on disk, without
// creating it. This is what the read-only per-session routes use; they must answer 404 for
// an unknown id, and they must share one load when a panel asks for several at once.
func (m *Manager) LoadPersistedSession(ctx context.Context, sessionID, defaultCWD string) (*State, error) {
	return m.ensureSessionSingleFlight(ctx, sessionID, defaultCWD, false)
}
