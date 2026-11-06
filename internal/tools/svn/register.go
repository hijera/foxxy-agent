package svn

import (
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svn"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func ReadOnlyToolNames() []string { return svn.ReadOnlyToolNames() }
func ToolNames() []string         { return svn.ToolNames() }

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
