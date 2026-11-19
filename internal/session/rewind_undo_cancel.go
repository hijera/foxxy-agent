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
// stop it. The owner checks the same prompt and snapshot before cancelling.
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
		_ = os.Remove(turnPath)
	}
}

func (st *State) isRewindEditedTurn(snap *rewindUndoSnapshot) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.Messages) == snap.BaseLen &&
		(snap.ID == "" || st.historyRewriteID == snap.ID) && rewindUndoValid(st.Messages, snap)
}
