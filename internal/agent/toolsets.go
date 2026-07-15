package agent

import (
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// ToolSet is an allowlist of tool names passed to the LLM. Empty or nil means unrestricted
// (all definitions from the registry, and MCP tools when the agent wires them in).
type ToolSet []string

// planToolNames is the fixed allowlist for plan mode (read-only registry builtins plus shell).
// MCP server tools are appended separately in react.go (same as agent mode).
var planToolNames = []string{
	"read",
	"glob",
	"grep",
	"websearch",
	"webfetch",
	"run_command",
	"question",
	"plan_write",
	"plan_list",
	"plan_read",
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
func ToolSetForMode(mode string) ToolSet {
	if mode == "plan" {
		out := make(ToolSet, len(planToolNames))
		copy(out, planToolNames)
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
