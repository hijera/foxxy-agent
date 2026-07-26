package svn

import (
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// ReadOnlyToolNames are the svn tools that never change state; they are the ones
// offered in plan and ask mode.
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

// Enabled reports whether the svn tools should be registered: Subversion support
// has to be on in the config and an svn client has to be installed. Both are
// checked when the registry is built, which happens once per prompt turn, so
// unchecking the setting removes the tools from the very next turn.
func Enabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.VCS.SVN.SVNEnabled() {
		return false
	}
	return svnws.Available(OptionsFor(cfg))
}

// RegisterBuiltins adds the Subversion tools to a registry when Enabled.
func RegisterBuiltins(add func(*tooling.Tool), cfg *config.Config) {
	if !Enabled(cfg) {
		return
	}
	c := newClient(cfg)
	for _, ctor := range []func() *tooling.Tool{
		c.InfoTool,
		c.StatusTool,
		c.DiffTool,
		c.LogTool,
		c.ListTool,
		c.AddTool,
		c.RevertTool,
		c.ResolveTool,
		c.UpdateTool,
		c.CommitTool,
		c.SwitchTool,
		c.MergeTool,
		c.CheckoutTool,
	} {
		add(ctor())
	}
}
