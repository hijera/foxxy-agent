//go:build http

package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/hijera/foxxycode-agent/internal/acp"
)

// toolExecWaits holds the delegated tool calls awaiting an IDE client's
// result, keyed by tool call id. The nonce is the token broadcast with the
// request; a result must echo it, so a POST that never saw the request cannot
// forge an outcome for the agent's transcript.
type toolExecWait struct {
	nonce string
	ch    chan acp.ToolExecutionResult
}

var (
	toolExecWaitsMu sync.Mutex
	toolExecWaits   = make(map[string]toolExecWait)
)

// newIDENonce mints the one-time token of a tool_execute request.
func newIDENonce() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func registerToolExecWait(toolCallID, nonce string) <-chan acp.ToolExecutionResult {
	ch := make(chan acp.ToolExecutionResult, 1)
	toolExecWaitsMu.Lock()
	toolExecWaits[toolCallID] = toolExecWait{nonce: nonce, ch: ch}
	toolExecWaitsMu.Unlock()
	return ch
}

func unregisterToolExecWait(toolCallID string) {
	toolExecWaitsMu.Lock()
	delete(toolExecWaits, toolCallID)
	toolExecWaitsMu.Unlock()
}

// CompleteToolExecResult resolves a pending delegated tool call. False means
// no call is waiting under this id, or the nonce does not match the one
// broadcast with it.
func CompleteToolExecResult(toolCallID, nonce string, res acp.ToolExecutionResult) bool {
	toolExecWaitsMu.Lock()
	w, ok := toolExecWaits[toolCallID]
	if !ok || w.nonce != nonce {
		toolExecWaitsMu.Unlock()
		return false
	}
	delete(toolExecWaits, toolCallID)
	toolExecWaitsMu.Unlock()
	w.ch <- res
	return true
}
