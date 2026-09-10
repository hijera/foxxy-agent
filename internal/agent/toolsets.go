package agent

import (
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
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
	// A planner fans out investigation the same way Claude Code's Explore
	// subagent does; the child of a plan-mode parent is forced into plan mode.
	"spawn_agent",
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

// askToolNames is the fixed allowlist for ask mode: repository reads and web
// research only. No shell, no plan tools, no config tools, and no MCP tools
// (react.go never appends MCP definitions in this mode).
var askToolNames = []string{
	"read",
	"keep_result",
	"glob",
	"grep",
	"print_tree",
	"websearch",
	"webfetch",
	"question",
	// Read-only: lets the assistant pull a catalogued skill's instructions when
	// skills.auto_discovery is on (the tool is only registered when enabled).
	"load_skill",
}

// ToolSetForMode returns the tool allowlist for the session mode. Agent mode is unrestricted.
// Debug mode is likewise unrestricted (full tool access, matching kilocode's Debug mode); its
// behaviour is driven entirely by the debug.md system prompt, so it intentionally falls through
// to the nil (unrestricted) return below instead of getting a named allowlist.
// noSelfRun mirrors tools.plan_no_self_run: in plan mode it removes plan_exit, so the model
// cannot switch the session to agent mode and start implementing on its own.
func ToolSetForMode(mode string, noSelfRun bool) ToolSet {
	var names []string
	switch mode {
	case "plan":
		out := make(ToolSet, 0, len(planToolNames))
		for _, n := range planToolNames {
			if noSelfRun && n == PlanExitToolName {
				continue
			}
			out = append(out, n)
		}
		return out
	case "docs":
		names = docsToolNames
	case "ask":
		names = askToolNames
	default:
		return nil
	}
	out := make(ToolSet, len(names))
	copy(out, names)
	return out
}

// askToolSet is the ask allowlist as a ToolSet, built once for the per-call check.
var askToolSet = ToolSet(askToolNames)

// modeMaySpawn reports whether a turn admitted in mode may delegate to a
// subagent. The fork has five modes where upstream has three: agent, plan and
// debug (unrestricted like agent) may spawn; ask and docs are read-only
// surfaces that delegate nothing, so spawn_agent is neither offered nor
// honoured there.
func modeMaySpawn(mode string) bool {
	switch mode {
	case "agent", "plan", "debug":
		return true
	default:
		return false
	}
}

// ModeAllowsMCPTools reports whether external MCP tools are exposed in a mode.
// Docs mode keeps a closed, documentation-only mutation surface and ask mode
// is read-only by construction; MCP servers do not expose enforceable
// read-only guarantees, so neither mode offers their tools.
func ModeAllowsMCPTools(mode string) bool {
	return mode != "docs" && mode != "ask"
}

// toolCallRefusedByMode reports whether a tool call must be refused at execution
// time in the given mode, with the refusal text returned as the tool result.
// Definition filtering already hides restricted tools from the LLM, but a model
// can still replay a call from earlier history (recorded in another mode), so
// ask mode re-checks its allowlist here, and plan mode does the same under
// tools.plan_no_self_run. MCP tool names (server__tool) are not in the ask
// allowlist and are refused the same way; plan mode keeps them reachable.
func toolCallRefusedByMode(mode, name string, noSelfRun bool) (string, bool) {
	name = strings.TrimSpace(name)
	switch mode {
	case "ask":
		if name == "" || askToolSet.Allows(name) {
			return "", false
		}
		return fmt.Sprintf("error: tool %q is not available in Ask mode because it is not read-only", name), true
	case "plan":
		if !noSelfRun || name == "" || strings.Contains(name, "__") {
			return "", false
		}
		if ToolSetForMode(mode, noSelfRun).Allows(name) {
			return "", false
		}
		return fmt.Sprintf("error: tool %q is not available in %s mode; "+
			"the user starts the implementation from the plan card", name, mode), true
	default:
		return "", false
	}
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
