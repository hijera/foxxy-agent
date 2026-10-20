package svn

// ReadOnlyToolNames are the svn tools that never change state. They are the
// read-only Subversion inspection contributed to plan mode, mirroring the
// read-only git commands the planner can already run through run_command.
// Registered only when vcs.svn is enabled and a client is installed; an
// unregistered name simply never appears in the definitions.
func ReadOnlyToolNames() []string {
	return []string{"svn_info", "svn_status", "svn_diff", "svn_log", "svn_list"}
}

// mutatingToolNames are the svn tools that rewrite the working copy. It is its
// own list rather than a slice of ToolNames, so ToolPolicy classifies a name by
// membership and cannot be shifted onto the wrong tools by an edit to either
// list.
func mutatingToolNames() []string {
	return []string{
		"svn_add", "svn_revert", "svn_resolve",
		"svn_update", "svn_commit", "svn_switch", "svn_merge", "svn_checkout",
	}
}

// ToolNames returns every svn tool name, read-only ones first.
func ToolNames() []string {
	return append(ReadOnlyToolNames(), mutatingToolNames()...)
}

// ToolsForMode contributes only the existing plan inspection allowlist.
func ToolsForMode(mode string) []string {
	if mode == "plan" {
		return ReadOnlyToolNames()
	}
	return nil
}
