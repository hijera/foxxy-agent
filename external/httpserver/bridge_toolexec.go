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
	// A session that switched to another workspace (a worktree) must not have a
	// client attached to this checkout execute its calls: the paths would land in
	// the wrong tree. The session's cwd travels with the request for exactly this
	// comparison and for the client's own relative-path resolution.
	s.mu.Lock()
	cwd := s.cwd
	sd := s.sessionDir
	s.mu.Unlock()
	if reqCwd := strings.TrimSpace(req.Cwd); reqCwd != "" && reqCwd != cwd {
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
		Cwd:        cwd,
	}
	// Filesystem write tools carry the resolved diff, so the client applies
	// exactly what the local write would have produced (and can refuse on a
	// buffer that no longer matches `before`).
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
