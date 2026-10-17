package svn

// ReadOnlyToolNames are the svn tools that never change state; they are the ones
// contributed explicitly to plan mode.
func ReadOnlyToolNames() []string {
	return []string{"svn_info", "svn_status", "svn_diff", "svn_log", "svn_list"}
}

// ToolNames returns every svn tool name, read-only ones first.
func ToolNames() []string {
	return append(ReadOnlyToolNames(),
		"svn_add", "svn_revert", "svn_resolve",
		"svn_update", "svn_commit", "svn_switch", "svn_merge", "svn_checkout",
	)
}

// ToolsForMode contributes only the existing plan inspection allowlist.
func ToolsForMode(mode string) []string {
	if mode == "plan" {
		return ReadOnlyToolNames()
	}
	return nil
}
