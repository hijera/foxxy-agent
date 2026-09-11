package agent

import (
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/permission"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// permissionRequired decides whether a tool call must be confirmed by the
// operator before it runs. registryRequires is the tool registry's own
// RequiresPermission flag; the built-in command and write tools override it
// with mode-specific policy:
//
//   - bypass never prompts.
//   - run_command consults the session command grants in every other mode;
//     accept_edits deliberately behaves like ask, because auto-approving shell
//     commands is not part of that mode's contract.
//   - filesystem writes are auto-approved by accept_edits (the mode's whole
//     point); ask consults the session write grants.
//   - config writes always prompt outside bypass: committing or rolling back
//     the agent's own configuration can start MCP processes and change the
//     permission policy itself.
func permissionRequired(registryRequires bool, tc llm.ToolCall, env *tools.Env, sessCmdGrants, sessWriteGrants []string) bool {
	switch {
	case tc.Name == "run_command":
		if env.PermissionMode == config.PermModeBypass {
			return false
		}
		cmd := permission.ExtractRunCommand(tc.InputJSON)
		return !permission.CommandAllowedWithSession(env, sessCmdGrants, cmd)
	case configWriteTool(tc.Name):
		return env.PermissionMode != config.PermModeBypass
	case filesystemWriteTool(tc.Name):
		if env.PermissionMode == config.PermModeBypass || env.PermissionMode == config.PermModeAcceptEdits {
			return false
		}
		keys := permission.WriteGrantKeys(tc.Name, tc.InputJSON, env.CWD)
		return !permission.AllWriteKeysGranted(sessWriteGrants, keys)
	default:
		return registryRequires
	}
}
