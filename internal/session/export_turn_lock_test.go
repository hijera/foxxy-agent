package session

// Test-only seams for the cross-process turn lock. The lock is taken through the Manager on
// the live path, but a test needs to take it from a second process where building a whole
// Manager (config, store, runner) would add nothing.

// NewStateForTurnLockTest returns a State that reports sessionDir as its persisted bundle.
func NewStateForTurnLockTest(sessionDir string) *State {
	return &State{ID: "turn-lock-test", SessionDir: sessionDir}
}

// AcquireTurnLockForTest takes the per-bundle turn lock exactly as a prompt turn does.
func AcquireTurnLockForTest(st *State) (func(), error) {
	m := &Manager{}
	return m.acquirePromptTurnLock(st.ID, st)
}
