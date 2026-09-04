package agent

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/mcp"
)

// Debug mode mirrors Agent mode for tool access: it is unrestricted, MCP tools
// are exposed, and no tool call is refused at execution time. Its behaviour is
// driven entirely by the debug.md system prompt, so there is no named allowlist.
func TestDebugModeIsUnrestricted(t *testing.T) {
	if set := ToolSetForMode("debug", false); !set.Unrestricted() {
		t.Errorf("debug mode should be unrestricted (nil set), got %v", set)
	}
	// Every background tool, including reap, is reachable — same as agent.
	for _, name := range append(backgroundObserveTools(), "background_reap") {
		if !ToolSetForMode("debug", false).Allows(name) {
			t.Errorf("debug mode should allow %s (unrestricted)", name)
		}
	}
}

func TestDebugModeAllowsMCPTools(t *testing.T) {
	if !ModeAllowsMCPTools("debug") {
		t.Error("debug mode should expose MCP tools")
	}
	// Any MCP tool is allowed (no read-only annotation requirement, unlike ask).
	tool := mcp.ToolInfo{Name: "server__do_stuff", ReadOnly: false}
	if !MCPToolAllowedForMode("debug", false, tool) {
		t.Error("debug mode should allow a non-read-only MCP tool")
	}
}

// Debug mode is not an enforced mode, so hallucinated tool names are passed to
// the registry (which returns "unknown tool") rather than refused by the mode
// boundary — identical to agent mode.
func TestDebugModeNeverRefusesToolCalls(t *testing.T) {
	for _, name := range []string{"write", "edit", "apply_patch", "run_command", "server__tool"} {
		if toolCallRefusedByMode("debug", name, false) {
			t.Errorf("debug mode should not refuse %s", name)
		}
	}
}
