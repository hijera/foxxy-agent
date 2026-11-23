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
// forge an outcome for the agent's transcript. Ids are only unique within one
// model answer ("call_0" repeats across sessions on several providers), so one
// id may hold several waits and the nonce is what singles one out.
type toolExecWait struct {
	nonce string
	ch    chan acp.ToolExecutionResult
}

var (
	toolExecWaitsMu sync.Mutex
	toolExecWaits   = make(map[string][]toolExecWait)
)

// newIDENonce mints the one-time token of a tool_execute request.
func newIDENonce() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func registerToolExecWait(toolCallID, nonce string) chan acp.ToolExecutionResult {
	ch := make(chan acp.ToolExecutionResult, 1)
	toolExecWaitsMu.Lock()
	toolExecWaits[toolCallID] = append(toolExecWaits[toolCallID], toolExecWait{nonce: nonce, ch: ch})
	toolExecWaitsMu.Unlock()
	return ch
}

// unregisterToolExecWait drops one wait, matched by its channel, leaving any
// other call registered under the same id alone.
func unregisterToolExecWait(toolCallID string, ch chan acp.ToolExecutionResult) {
	toolExecWaitsMu.Lock()
	defer toolExecWaitsMu.Unlock()
	ws := toolExecWaits[toolCallID]
	for i, w := range ws {
		if w.ch == ch {
			ws = append(ws[:i], ws[i+1:]...)
			break
		}
	}
	if len(ws) == 0 {
		delete(toolExecWaits, toolCallID)
	} else {
		toolExecWaits[toolCallID] = ws
	}
}

// CompleteToolExecResult resolves a pending delegated tool call. False means
// no call is waiting under this id, or the nonce does not match the one
// broadcast with it.
func CompleteToolExecResult(toolCallID, nonce string, res acp.ToolExecutionResult) bool {
	toolExecWaitsMu.Lock()
	ws := toolExecWaits[toolCallID]
	for i, w := range ws {
		if w.nonce != nonce {
			continue
		}
		rest := append(ws[:i], ws[i+1:]...)
		if len(rest) == 0 {
			delete(toolExecWaits, toolCallID)
		} else {
			toolExecWaits[toolCallID] = rest
		}
		toolExecWaitsMu.Unlock()
		w.ch <- res
		return true
	}
	toolExecWaitsMu.Unlock()
	return false
}
