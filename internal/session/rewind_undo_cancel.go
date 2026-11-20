package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	rewindUndoTurnFile   = "turn.json"
	rewindUndoCancelFile = "cancel.json"
)

// rewindUndoTurn names one running prompt under one rewind snapshot.
type rewindUndoTurn struct {
	SnapshotID string `json:"snapshotId"`
	TurnID     string `json:"turnId"`
}

func readRewindUndoTurn(path string) rewindUndoTurn {
	data, err := os.ReadFile(path)
	if err != nil {
		return rewindUndoTurn{}
	}
	var turn rewindUndoTurn
	if json.Unmarshal(data, &turn) != nil {
		return rewindUndoTurn{}
	}
	return turn
}

// RequestRewindUndoCancel asks the process that owns the edited prompt to
// stop it. The owner checks the same prompt and snapshot before cancelling. A
// turn marker only names a watcher that wrote one, so a marker this process can
// see through is refused rather than answered.
func (m *Manager) RequestRewindUndoCancel(sessionID, snapshotID string) error {
	st := m.getSession(sessionID)
	if st == nil {
		return ErrRewindUndoUnavailable
	}
	dir := st.GetPersistedSessionDir()
	snap := readRewindUndoSnapshot(dir)
	if snap == nil || snapshotID == "" || snap.snapshotID() != snapshotID {
		return ErrRewindUndoUnavailable
	}
	undoDir := filepath.Join(dir, rewindUndoDirName)
	turn := readRewindUndoTurn(filepath.Join(undoDir, rewindUndoTurnFile))
	if turn.TurnID == "" || turn.SnapshotID != snapshotID {
		return ErrSessionTurnActive
	}
	// The marker proves only that some watcher wrote it. When the turn runs in
	// this process and no watcher of ours stands behind that marker, the
	// process that wrote it died mid-turn and nothing will ever read a cancel:
	// refuse now rather than leave the caller polling for a stop that cannot
	// come. A turn held by another process is that process' to vouch for.
	if m.SessionTurnActiveInProcess(sessionID) && !st.holdsUndoWatchTurn(turn.TurnID) {
		_ = os.Remove(filepath.Join(undoDir, rewindUndoTurnFile))
		return ErrSessionTurnActive
	}
	return writeJSONAtomic(filepath.Join(undoDir, rewindUndoCancelFile), turn)
}

// watchRewindUndoCancel registers the first prompt admitted on a freshly cut
// transcript. The caller owns the turn lock through registration and cleanup.
func watchRewindUndoCancel(ctx context.Context, st *State, cancel context.CancelFunc) func() {
	dir := st.GetPersistedSessionDir()
	if dir == "" {
		return func() {}
	}
	snap := readRewindUndoSnapshot(dir)
	if snap == nil || snap.snapshotID() == "" || !st.isRewindEditedTurn(snap) {
		return func() {}
	}
	undoDir := filepath.Join(dir, rewindUndoDirName)
	turnPath := filepath.Join(undoDir, rewindUndoTurnFile)
	turn := rewindUndoTurn{SnapshotID: snap.snapshotID(), TurnID: rand.Text()}
	if err := writeJSONAtomic(turnPath, turn); err != nil {
		return func() {}
	}
	st.setUndoWatchTurn(turn.TurnID)
	cancelPath := filepath.Join(undoDir, rewindUndoCancelFile)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if readRewindUndoTurn(cancelPath) != turn {
					continue
				}
				if ctx.Err() == nil && st.rewindUndoValid(snap) {
					st.SetUserCancelledTurn()
					cancel()
				}
				return
			}
		}
	}()
	return func() {
		<-finished
		st.setUndoWatchTurn("")
		_ = os.Remove(turnPath)
		// A request this watcher has already taken, or never got to read, must
		// not stay in the bundle to be read as someone else's.
		_ = os.Remove(cancelPath)
	}
}

// setUndoWatchTurn records the turn a live watcher of this process stands
// behind, or clears it with "".
func (st *State) setUndoWatchTurn(turn string) {
	st.mu.Lock()
	st.undoWatchTurn = turn
	st.mu.Unlock()
}

// holdsUndoWatchTurn reports whether a live watcher of this process stands
// behind the given turn marker.
func (st *State) holdsUndoWatchTurn(turn string) bool {
	if turn == "" {
		return false
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.undoWatchTurn == turn
}

func (st *State) isRewindEditedTurn(snap *rewindUndoSnapshot) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.Messages) == snap.BaseLen &&
		(snap.ID == "" || st.historyRewriteID == snap.ID) && rewindUndoValid(st.Messages, snap)
}
