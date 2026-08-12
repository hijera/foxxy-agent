package session

import "context"

// Readiness gate for the configured MCP servers of one session.
//
// Connecting them used to happen inline on whatever goroutine loaded or created the session —
// which, over HTTP, is a request a panel is waiting on. A server that takes its time (a cold
// npx that downloads on first run) therefore held the request, and with it one of the
// webview's six connections, for as long as it liked.
//
// The connect now runs in the background and publishes through
// [State.replaceConfiguredMCPClients]. This gate is how a turn — the one caller that actually
// needs the tools — waits for it, and nobody else does.

// mcpGate is one pending connect. Closing is tracked explicitly rather than probed, because
// both the connect finishing and the session closing can settle the same gate.
type mcpGate struct {
	done   chan struct{}
	closed bool
}

// settle closes the gate. Caller must hold s.mu.
func (g *mcpGate) settle() {
	if g == nil || g.closed {
		return
	}
	g.closed = true
	close(g.done)
}

// beginConfiguredMCPConnect arms the gate for a connect that is about to start and returns the
// function that settles it. Overlapping connects (session load followed by a workspace switch)
// each own their own gate, so a slow one finishing cannot clear a newer one's pending state.
// On a closed session the gate stays settled and the returned function does nothing.
func (s *State) beginConfiguredMCPConnect() func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mcpClosed {
		return func() {}
	}
	gate := &mcpGate{done: make(chan struct{})}
	s.mcpReady = gate
	return func() {
		s.mu.Lock()
		gate.settle()
		if s.mcpReady == gate {
			s.mcpReady = nil
		}
		s.mu.Unlock()
	}
}

// WaitMCPReady blocks until the configured MCP servers of this session have finished
// connecting, the session is closed, or ctx is done. Returns ctx.Err() only in the last case.
//
// The loop matters: a workspace switch can arm a *new* connect while a caller is waiting on the
// old one, and that caller must wait for the servers it is actually going to use.
func (s *State) WaitMCPReady(ctx context.Context) error {
	for {
		s.mu.RLock()
		gate := s.mcpReady
		s.mu.RUnlock()
		if gate == nil {
			return nil
		}
		select {
		case <-gate.done:
			// Settled — loop once more in case a newer connect was armed meanwhile.
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// MCPReady reports whether the configured MCP servers have settled, without waiting. A turn
// uses this to stay silent in the common case instead of announcing a wait that never happens.
func (s *State) MCPReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mcpReady == nil
}

// markMCPClosed settles the gate permanently: the session is going away, so a connect still in
// flight must not hand it clients and must not hold anyone waiting.
func (s *State) markMCPClosed() {
	s.mu.Lock()
	s.mcpClosed = true
	s.mcpReady.settle()
	s.mcpReady = nil
	s.mu.Unlock()
}
