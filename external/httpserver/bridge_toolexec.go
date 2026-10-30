//go:build http

package httpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	toolfs "github.com/hijera/foxxycode-agent/internal/tools/fs"
)

// ExecuteToolCall implements acp.ToolExecutor: offers the call to connected
// IDE clients as a tool_execute event on /foxxycode/ide/events and waits for
// POST /foxxycode/ide/tool-result. Returns not_handled when no client declared
// the tool, so the agent runs the call locally.
func (s *Sender) ExecuteToolCall(ctx context.Context, req acp.ToolExecutionRequest) (acp.ToolExecutionResult, error) {
	notHandled := acp.ToolExecutionResult{Status: acp.ToolExecNotHandled}
	sid := strings.TrimSpace(req.SessionID)
	tcid := strings.TrimSpace(req.ToolCallID)
	name := strings.TrimSpace(req.ToolName)
	if sid == "" || tcid == "" || name == "" {
		return notHandled, fmt.Errorf("sessionId, toolCallId and toolName are required")
	}
	if !ideEvents.HandlesTool(name) {
		return notHandled, nil
	}
	nonce, err := newIDENonce()
	if err != nil {
		return notHandled, err
	}
	// Registered before the broadcast, so a result that arrives the instant a
	// client learns of the request already has somewhere to land.
	ch := registerToolExecWait(tcid, nonce)
	defer unregisterToolExecWait(tcid)

	ev := ideEvent{
		Type:       "tool_execute",
		ToolCallID: tcid,
		SessionID:  sid,
		ToolName:   name,
		ArgsJSON:   req.ArgsJSON,
		Nonce:      nonce,
	}
	// Filesystem write tools carry the resolved diff, so the client applies
	// exactly what the local write would have produced (and can refuse on a
	// buffer that no longer matches `before`).
	s.mu.Lock()
	cwd := s.cwd
	sd := s.sessionDir
	s.mu.Unlock()
	if absPath, before, after, ok, perr := toolfs.EditPreview(name, req.ArgsJSON, cwd, sd); ok && perr == nil {
		ev.Path = absPath
		ev.Before = string(before)
		ev.After = string(after)
	}
	ideEvents.Broadcast(ev)

	select {
	case res := <-ch:
		return res, nil
	case <-ctx.Done():
		return notHandled, ctx.Err()
	}
}
