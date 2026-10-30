package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	toolfs "github.com/hijera/foxxycode-agent/internal/tools/fs"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// toolDelegateTimeout bounds how long a delegated tool call may sit with a
// connected client before the agent runs it locally. File edits round-trip in
// well under this; a client executing long work must answer sooner or accept
// the local fallback.
const toolDelegateTimeout = 60 * time.Second

// tryDelegateToolCall offers a tool call to a connected client (an IDE plugin
// executing it through the editor). handled is false when the call must run
// locally: no delegate on this surface, the client declined the call, or the
// delegate failed before taking it. A turn cancelled while the delegate was
// deciding is not a fallback - the call ends cancelled like any other.
func (a *Agent) tryDelegateToolCall(ctx context.Context, tc llm.ToolCall, env *tooling.Env, sessionID string) (handled bool, result string, err error) {
	ex, ok := a.server.(acp.ToolExecutor)
	if !ok || ex == nil {
		return false, "", nil
	}
	// A subagent's calls stay local: its transcript is private to the run and
	// the surface of the parent must not execute work named by the child.
	if a.subagent != nil {
		return false, "", nil
	}
	// The edit preview is computed before the client runs the call: afterwards
	// the disk may already hold the new content and the transforms would read
	// it as their input. On success it stands in for the notifyFileEdit the
	// executing tool would have fired, so the transcript row and the
	// edit_applied event are the same either way.
	cwd := ""
	if env != nil {
		cwd = env.CWD
	}
	absPath, before, after, isEdit, _ := toolfs.EditPreview(tc.Name, tc.InputJSON, cwd)

	dctx, cancel := context.WithTimeout(ctx, toolDelegateTimeout)
	defer cancel()
	res, derr := ex.ExecuteToolCall(dctx, acp.ToolExecutionRequest{
		SessionID:  sessionID,
		ToolCallID: strings.TrimSpace(tc.ID),
		ToolName:   tc.Name,
		ArgsJSON:   tc.InputJSON,
	})
	if derr != nil {
		if ctx.Err() != nil {
			return true, "", ctx.Err()
		}
		return false, "", nil
	}
	if res.Status != acp.ToolExecHandled {
		return false, "", nil
	}
	if strings.TrimSpace(res.Err) != "" {
		return true, "", errors.New(res.Err)
	}
	if isEdit {
		toolfs.NotifyFileEdit(env, tc.Name, absPath, before, after)
	}
	return true, res.Output, nil
}
