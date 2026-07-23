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
	"glob",
	"grep",
	"print_tree",
	"websearch",
	"webfetch",
	"run_command",
	"question",
	"plan_write",
	"plan_list",
	"plan_read",
	// Lets the model finish planning and start the implementation itself. Dropped when
	// tools.plan_no_self_run is on, so only the user can launch from the plan card.
	PlanExitToolName,
	// Read-only: lets the planner pull a catalogued skill's instructions when
	// skills.auto_discovery is on (the tool is only registered when enabled).
	"load_skill",
}

var docsToolNames = []string{
	"read",
	"glob",
	"grep",
	"websearch",
	"webfetch",
	"question",
	"docs_write",
	"docs_edit",
}

// ToolSetForMode returns the tool allowlist for the session mode. Agent mode is unrestricted.
// noSelfRun mirrors tools.plan_no_self_run: in plan mode it removes plan_exit, so the model
// cannot switch the session to agent mode and start implementing on its own.
func ToolSetForMode(mode string, noSelfRun bool) ToolSet {
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
	return nil
}

// ModeAllowsMCPTools reports whether external MCP tools are exposed in a mode.
// Docs mode keeps a closed, documentation-only mutation surface because MCP
// servers do not currently expose enforceable read-only guarantees.
func ModeAllowsMCPTools(mode string) bool {
	return mode != "docs"
}

// toolCallRefusedByMode reports whether a tool call must be refused instead of executed.
// Filtering the definitions sent to the model is not enough on its own: a model can still
// emit a call for a tool it was never offered (hallucination, or a name carried over from
// an earlier agent-mode turn), and the registry would happily run it. Only the
// tools.plan_no_self_run guard turns this on, so default behaviour is unchanged.
// MCP tools (server__tool) are exempt wherever the mode exposes them.
func toolCallRefusedByMode(mode, toolName string, noSelfRun bool) bool {
	if !noSelfRun || mode != "plan" {
		return false
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return false
	}
	if strings.Contains(name, "__") && ModeAllowsMCPTools(mode) {
		return false
	}
	return !ToolSetForMode(mode, noSelfRun).Allows(name)
}

// modeToolRefusalMessage is the tool result handed back to the model for a refused call.
func modeToolRefusalMessage(mode, toolName string) string {
	return fmt.Sprintf("error: tool %q is not available in %s mode; "+
		"the user starts the implementation from the plan card", toolName, mode)
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
