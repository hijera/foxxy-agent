package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/permission"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// ResumeAfterPermission executes a tool call that was approved via POST /permission after the HTTP
// stream ended or the server restarted, then continues the ReAct loop from persisted messages.
func (a *Agent) ResumeAfterPermission(ctx context.Context, toolCallID string, perm *acp.PermissionResult) (string, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return "", fmt.Errorf("toolCallId is required")
	}
	if perm == nil {
		return "", fmt.Errorf("permission result is nil")
	}
	tc, err := a.findPendingToolCall(toolCallID)
	if err != nil {
		// The gate is over - most often the prompt timed out and the turn moved
		// on. Drop the persisted record so a stale one from before bridge.go
		// learned to clear it does not keep being resurrected.
		if sd := strings.TrimSpace(a.state.GetPersistedSessionDir()); sd != "" {
			_ = session.ClearPendingPermission(sd)
		}
		return "", err
	}
	mode := a.state.GetMode()
	sd := strings.TrimSpace(a.state.GetPersistedSessionDir())
	toolEnv := a.buildToolEnv(mode, sd)
	if !permission.Approved(perm) {
		// A refusal needs nothing from the bundle: the gate is cleared and
		// the denial recorded before anything is read, so an unreadable
		// arguments file cannot keep a refused call pending.
		if sd != "" {
			_ = session.ClearPendingPermission(sd)
		}
		toolResultMsg := llm.Message{
			Role:       llm.RoleTool,
			Content:    permissionDeniedResult(perm),
			ToolCallID: tc.ID,
		}
		a.state.AddMessage(toolResultMsg)
		if sd != "" {
			_ = session.WriteToolCallResult(sd, tc.ID, toolResultMsg.Content)
			_ = session.MarkToolCallFinished(sd, tc.ID, tc.Name, toolKind(tc.Name), "cancelled")
		}
		return a.continueReAct(ctx, mode, toolEnv)
	}
	// The history holds the arguments the model produced; the bundle holds
	// the arguments the prompt showed, after any PreToolUse rewrite (it is
	// written when the call starts and again after a rewrite, or the call is
	// cancelled before the prompt). The approval binds to the latter, so
	// those are what runs and what an allow-always grant is recorded
	// against; a bundle that cannot produce them fails closed, and the gate
	// stays for a retry.
	if sd != "" {
		shown, err := session.ReadToolCallArgs(sd, tc.ID)
		if err != nil {
			return "", fmt.Errorf("resume tool call %s: the approved arguments could not be read: %w", tc.ID, err)
		}
		if strings.TrimSpace(shown) == "" && strings.TrimSpace(tc.InputJSON) != "" {
			return "", fmt.Errorf("resume tool call %s: the approved arguments are missing from the bundle", tc.ID)
		}
		tc.InputJSON = shown
	}
	// A call the current mode refuses (a pending agent-mode write approved
	// after switching to ask) must not leave an "allow always" grant behind:
	// the grant would outlive the refusal and apply once the mode changes back.
	_, refusedByMode := toolCallRefusedByMode(mode, tc.Name, a.cfg.Tools.PlanNoSelfRunEnabled())
	if st := sessionStatePtr(a.state); st != nil && !refusedByMode {
		permission.RecordAllowAlways(st, tc.Name, tc.InputJSON, toolEnv.CWD, perm)
	}
	if sd != "" {
		_ = session.ClearPendingPermission(sd)
	}
	result, execErr := a.executeToolCall(ctx, tc, toolEnv, mode, a.state.GetID(), true, 0)
	var toolResultMsg llm.Message
	if execErr != nil {
		toolResultMsg = llm.Message{
			Role:       llm.RoleTool,
			Content:    fmt.Sprintf("error: %v", execErr),
			ToolCallID: tc.ID,
		}
	} else {
		toolResultMsg = llm.Message{
			Role:       llm.RoleTool,
			Content:    result,
			ToolCallID: tc.ID,
		}
	}
	a.state.AddMessage(toolResultMsg)
	return a.continueReAct(ctx, mode, toolEnv)
}

func (a *Agent) findPendingToolCall(toolCallID string) (llm.ToolCall, error) {
	msgs := a.state.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == llm.RoleTool && strings.TrimSpace(m.ToolCallID) == toolCallID {
			return llm.ToolCall{}, fmt.Errorf("tool call %s already has a result", toolCallID)
		}
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != llm.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(tc.ID) != toolCallID {
				continue
			}
			// A session written before partial persists stopped carrying tool calls
			// can hold one whose arguments were cut mid-write. The SPA restores a
			// permission prompt for it, so approving would run the tool on truncated
			// input; refuse with something the operator can act on instead of a
			// per-tool unmarshal error.
			if args := strings.TrimSpace(tc.InputJSON); args != "" && !json.Valid([]byte(args)) {
				return llm.ToolCall{}, fmt.Errorf(
					"tool call %s has incomplete arguments (the response was cut off before it finished writing them); ask again instead of resuming it", toolCallID)
			}
			return tc, nil
		}
	}
	return llm.ToolCall{}, fmt.Errorf("tool call %s not found in session history", toolCallID)
}

func (a *Agent) buildToolEnv(mode, sessionDir string) *tools.Env {
	env := &tools.Env{
		CWD:              a.state.GetCWD(),
		PermissionMode:   effectivePermMode(a.state, a.cfg),
		CommandAllowlist: a.cfg.Tools.CommandAllowlist,
		SessionID:        a.state.GetID(),
		SessionDir:       sessionDir,
		ArchiveActiveMarkdown: func() error {
			if sessionDir == "" {
				return nil
			}
			return session.ArchiveActiveTodo(sessionDir)
		},
		WriteArchivedPlanMarkdown: func(md string) (string, error) {
			if sessionDir == "" {
				return "", nil
			}
			return session.WritePlanArchivedMarkdown(sessionDir, md)
		},
		Sender:         a.server,
		GetPlan:        a.state.GetPlan,
		SetPlan:        a.state.SetPlan,
		SetSessionMode: a.setSessionModeAnnounced,
		PersistPlanDocument: func(doc plans.Document) {
			a.state.AppendPlanDocument(doc)
		},
		OutputLineLimits:  a.cfg.Tools.OutputLimits.AsMap(),
		Background:        a.backgroundPool(sessionDir),
		BackgroundEnabled: a.cfg.Tools.Background.ResolvedEnabled(),
	}
	a.applySubagentEnv(env, mode)
	a.wireFileEditHook(env)
	return env
}

// backgroundPool returns the process-wide task pool, telling it where this
// session persists so a task started from here mirrors its output into the
// session bundle.
func (a *Agent) backgroundPool(sessionDir string) *bgtask.Pool {
	pool := bgtask.Default()
	pool.SetConfig(backgroundConfig(a.cfg))
	if strings.TrimSpace(sessionDir) != "" {
		pool.SetSessionDir(a.state.GetID(), sessionDir)
	}
	return pool
}

// backgroundConfig translates the operator's YAML into the pool's bounds.
func backgroundConfig(cfg *config.Config) bgtask.Config {
	if cfg == nil {
		return bgtask.Config{}
	}
	resolved := cfg.Tools.Background.Resolved()
	return bgtask.Config{
		MaxConcurrent:         resolved.MaxConcurrent,
		DefaultTimeoutSeconds: resolved.DefaultTimeoutSeconds,
		MaxTimeoutSeconds:     resolved.MaxTimeoutSeconds,
		OutputBufferBytes:     resolved.OutputBufferBytes,
	}
}

// continueReAct runs the ReAct loop using messages already on the session (no new user turn).
func (a *Agent) continueReAct(ctx context.Context, mode string, toolEnv *tools.Env) (string, error) {
	userText := lastUserText(a.state.GetMessages())
	contextFiles := extractContextFiles(nil)
	activeSkills := FilterSkillsForContext(a.state.GetSkills(), contextFiles)
	toolDefs := a.currentToolDefinitions(mode)
	transport, err := a.getProvider(mode)
	if err != nil {
		return string(acp.StopReasonRefused), fmt.Errorf("no LLM configured: %w", err)
	}
	messages := a.buildMessages(a.buildSystemPrompt(mode, activeSkills, toolDefs, userText, contextFiles))
	maxTurns := a.cfg.Agent.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 30
	}
	sd := strings.TrimSpace(a.state.GetPersistedSessionDir())
	toolEnv.SendDesignPlanUpdate = func(doc plans.Document) {
		tools.SendDesignPlanUpdate(toolEnv, doc)
	}
	return a.runReActLoop(ctx, mode, messages, toolDefs, transport, toolEnv, sd, userText, contextFiles, activeSkills, maxTurns, false)
}

func lastUserText(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}
