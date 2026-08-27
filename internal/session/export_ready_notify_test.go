package session

// Test-only seam for the deferred session/new replay. The parked closure is deliberately
// unexported - only HandleSessionReady may drain it - but a test has to be able to assert
// that no closure was left behind on a path where nothing will ever drain one.

// HasPendingReadyNotifyForTest reports whether session updates are still parked on this state.
func (s *State) HasPendingReadyNotifyForTest() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingReadyNotify != nil
}
