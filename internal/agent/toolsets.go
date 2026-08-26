package agent

import (
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mcp"
)

// ToolSet is an allowlist of tool names passed to the LLM. Empty or nil means unrestricted
// (all definitions from the registry, and MCP tools when the agent wires them in).
type ToolSet []string

// PlanExitToolName leaves plan mode and switches the session to agent mode. It is the
// only plan-mode tool the model can use to start executing on its own, so the
// tools.plan_no_self_run guard drops it from the set.
const PlanExitToolName = "plan_exit"

// planToolNames is the fixed allowlist for plan mode (read-only registry builtins plus shell).
// MCP server tools are appended separately in react.go (same as agent mode).
var planToolNames = []string{
	"read",
	"keep_result",
	"glob",
	"grep",
	"print_tree",
	"websearch",
	"webfetch",
	"run_command",
	// Background execution is available in plan mode for the same reason
	// run_command is: a planner investigating a repo should not have to sit
	// through a slow read-only command, and the pool tools only observe and
	// terminate work the planner started itself. background_reap is left out:
	// it kills process groups this session never started.
	"background_list",
	"background_output",
	"background_wait",
	"background_stop",
	"question",
	"config_get",
	// Read-only view of staged config commands; staging and committing stay
	// agent-mode-only. docs and ask stay out entirely: they are narrower than
	// plan and never touch the agent's own configuration.
	"config_changes",
	"plan_write",
	"plan_list",
	"plan_read",
	// Lets the model finish planning and start the implementation itself. Dropped when
	// tools.plan_no_self_run is on, so only the user can launch from the plan card.
	PlanExitToolName,
	// Read-only: lets the planner pull a catalogued skill's instructions when
	// skills.auto_discovery is on (the tool is only registered when enabled).
	"load_skill",
	// Read-only Subversion inspection, mirroring the read-only git commands the
	// planner can already run through run_command. Registered only when
	// vcs.svn is enabled and a client is installed; an unregistered name simply
	// never appears in the definitions.
	"svn_info",
	"svn_status",
	"svn_diff",
	"svn_log",
	"svn_list",
}

var docsToolNames = []string{
	"read",
	"keep_result",
	"glob",
	"grep",
	"websearch",
	"webfetch",
	"question",
	"docs_write",
	"docs_edit",
}

var askBasicToolNames = []string{
	"read",
	"keep_result",
	"glob",
	"grep",
	"print_tree",
	"question",
	"load_skill",
}

var askExtendedToolNames = []string{
	"run_command",
	// Ask already grants run_command here, so a backgrounded one is reachable
	// and has to stay observable. Same omission as plan mode: no background_reap.
	"background_list",
	"background_output",
	"background_wait",
	"background_stop",
	"websearch",
	"webfetch",
	"foxxycode_scheduler_jobs_list",
	"foxxycode_scheduler_job_get",
	"foxxycode_scheduler_job_runs",
	"svn_info",
	"svn_status",
	"svn_diff",
	"svn_log",
	"svn_list",
}

// ToolSetForMode returns the tool allowlist for the session mode. Agent mode is unrestricted.
// noSelfRun mirrors tools.plan_no_self_run: in plan mode it removes plan_exit, so the model
// cannot switch the session to agent mode and start implementing on its own. The optional
// askBasicOnly value mirrors tools.ask_disable_extended_tools.
func ToolSetForMode(mode string, noSelfRun bool, askBasicOnly ...bool) ToolSet {
	if mode == "plan" {
		out := make(ToolSet, 0, len(planToolNames))
		for _, n := range planToolNames {
			if noSelfRun && n == PlanExitToolName {
				continue
			}
			out = append(out, n)
		}
		return out
	}
	if mode == "docs" {
		out := make(ToolSet, len(docsToolNames))
		copy(out, docsToolNames)
		return out
	}
	if mode == "ask" {
		out := make(ToolSet, len(askBasicToolNames))
		copy(out, askBasicToolNames)
		if !optionalBool(askBasicOnly) {
			out = append(out, askExtendedToolNames...)
		}
		return out
	}
	return nil
}

// ModeAllowsMCPTools reports whether external MCP tools are exposed in a mode.
// Docs mode keeps a closed, documentation-only mutation surface because MCP
// servers do not currently expose enforceable read-only guarantees. Ask mode
// accepts only tools explicitly annotated read-only unless its extended tools
// are disabled entirely.
func ModeAllowsMCPTools(mode string, askBasicOnly ...bool) bool {
	if mode == "docs" {
		return false
	}
	return mode != "ask" || !optionalBool(askBasicOnly)
}

// MCPToolAllowedForMode applies the mode boundary to one MCP tool definition.
// Ask requires the standard readOnlyHint annotation; absence is treated as unsafe.
func MCPToolAllowedForMode(mode string, askBasicOnly bool, tool mcp.ToolInfo) bool {
	if !ModeAllowsMCPTools(mode, askBasicOnly) {
		return false
	}
	return mode != "ask" || tool.ReadOnly
}

// toolCallRefusedByMode reports whether a tool call must be refused instead of executed.
// Filtering the definitions sent to the model is not enough on its own: a model can still
// emit a call for a tool it was never offered (hallucination, or a name carried over from
// an earlier turn), and the registry would happily run it. Plan enables this
// boundary through tools.plan_no_self_run; Ask always enforces it. MCP tools
// (server__tool) pass through here only when the mode exposes them and Ask
// validates their read-only annotation separately.
func toolCallRefusedByMode(mode, toolName string, noSelfRun bool, askBasicOnly ...bool) bool {
	enforce := mode == "ask" || (mode == "plan" && noSelfRun)
	if !enforce {
		return false
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return false
	}
	if strings.Contains(name, "__") && ModeAllowsMCPTools(mode, askBasicOnly...) {
		return false
	}
	return !ToolSetForMode(mode, noSelfRun, askBasicOnly...).Allows(name)
}

// modeToolRefusalMessage is the tool result handed back to the model for a refused call.
func modeToolRefusalMessage(mode, toolName string) string {
	if mode == "ask" {
		return fmt.Sprintf("error: tool %q is not available in Ask mode because it is not read-only", toolName)
	}
	return fmt.Sprintf("error: tool %q is not available in %s mode; "+
		"the user starts the implementation from the plan card", toolName, mode)
}

func optionalBool(values []bool) bool {
	return len(values) > 0 && values[0]
}

// Unrestricted reports whether the set imposes no name filter.
func (s ToolSet) Unrestricted() bool {
	return len(s) == 0
}

// Allows reports whether name is permitted by this set. Unrestricted sets allow every name.
func (s ToolSet) Allows(name string) bool {
	if s.Unrestricted() {
		return true
	}
	for _, n := range s {
		if n == name {
			return true
		}
	}
	return false
}

// FilterToolDefinitions keeps definitions whose names are allowed by set.
func FilterToolDefinitions(defs []llm.ToolDefinition, set ToolSet) []llm.ToolDefinition {
	if set.Unrestricted() {
		return defs
	}
	var out []llm.ToolDefinition
	for i := range defs {
		if set.Allows(defs[i].Name) {
			out = append(out, defs[i])
		}
	}
	return out
}
