//go:build http

package httpserver

import (
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// liveDiffTTL is how long one comparison of a running turn is reused. The
// review window asks once per file, a few at a time, so without it a window over
// forty files would walk the workspace forty times for one look; the card asks
// at most once per finished tool call, further apart than this.
const liveDiffTTL = 300 * time.Millisecond

// liveTurn is a turn this process is running: the workspace it runs in and the
// snapshot taken before it started. It lets the changed-files card, opened
// mid-turn, report what the turn has already written - the turn's own diff is
// only stored once it ends.
type liveTurn struct {
	cwd    string
	before *session.WorkspaceSnapshot

	mu       sync.Mutex
	cached   *session.WorkspaceDiff
	cachedAt time.Time
}

// diff compares the workspace with the pre-turn snapshot, reusing a comparison
// younger than liveDiffTTL.
func (t *liveTurn) diff() *session.WorkspaceDiff {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.cachedAt.IsZero() && time.Since(t.cachedAt) < liveDiffTTL {
		return t.cached
	}
	d, err := session.LiveWorkspaceDiff(t.cwd, t.before)
	if err != nil {
		d = nil
	}
	t.cached, t.cachedAt = d, time.Now()
	return d
}

// beginLiveTurn registers a turn that is about to run. Both profile doors hold
// the session's turn lock before they get here, so there is one per session.
func (s *Server) beginLiveTurn(sessionID, cwd string, before *session.WorkspaceSnapshot) *liveTurn {
	t := &liveTurn{cwd: cwd, before: before}
	s.liveTurnMu.Lock()
	defer s.liveTurnMu.Unlock()
	if s.liveTurns == nil {
		s.liveTurns = make(map[string]*liveTurn)
	}
	s.liveTurns[sessionID] = t
	return t
}

// endLiveTurn retires a turn's entry. It only removes that very entry, so a
// late call can never retire the turn that came after it.
func (s *Server) endLiveTurn(sessionID string, t *liveTurn) {
	if t == nil {
		return
	}
	s.liveTurnMu.Lock()
	defer s.liveTurnMu.Unlock()
	if s.liveTurns[sessionID] == t {
		delete(s.liveTurns, sessionID)
	}
}

// liveTurnDiff reports what the running turn of a session has changed so far,
// and whether a turn is running at all.
func (s *Server) liveTurnDiff(sessionID string) (*session.WorkspaceDiff, bool) {
	s.liveTurnMu.Lock()
	t := s.liveTurns[sessionID]
	s.liveTurnMu.Unlock()
	if t == nil {
		return nil, false
	}
	return t.diff(), true
}

// settleTurnDiff records what a finished turn did to the workspace, retires its
// live entry and tells the clients the change set is settled.
//
// A turn that failed after it ran - the provider dropped the answer halfway -
// still wrote what it wrote, so it is captured like one that finished. A turn
// that never ran (the session was busy, a child transcript is read-only) is
// told apart by the user-turn count, which only a turn that reached the agent
// moves; a diff taken for it would file whatever else changed in the folder
// under the previous turn's number.
func (s *Server) settleTurnDiff(st *session.State, before *session.WorkspaceSnapshot, live *liveTurn, turnsBefore int, runErr error) {
	if runErr != nil && session.TurnNumber(st.GetMessages()) <= turnsBefore {
		id := st.GetID()
		s.endLiveTurn(id, live)
		s.publishSessionChanges(id)
		return
	}
	s.captureAndStoreTurnDiff(st, before, live)
}
