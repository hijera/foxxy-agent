//go:build browser

package browser

import (
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// RegisterBuiltins adds the interactive browser tools to a registry when the
// browser config is enabled. A single Manager (one Chrome per session) backs all
// of them.
func RegisterBuiltins(add func(*tooling.Tool), cfg *config.Config) {
	if cfg == nil || !cfg.Browser.Enabled {
		return
	}
	bc := &cfg.Browser
	m := NewManager(bc)
	add(m.NavigateTool())
	add(m.ClickTool())
	add(m.FillTool())
	add(m.HoverTool())
	add(m.ScrollTool())
	add(m.ScreenshotTool())
	add(m.EvaluateTool())
	add(m.CloseTool())
}

// ToolNames returns the tool names the browser subsystem registers. Used by tests
// and by mode/toolset wiring.
func ToolNames() []string {
	return []string{
		"foxxycode_browser_navigate",
		"foxxycode_browser_click",
		"foxxycode_browser_fill",
		"foxxycode_browser_hover",
		"foxxycode_browser_scroll",
		"foxxycode_browser_screenshot",
		"foxxycode_browser_evaluate",
		"foxxycode_browser_close",
	}
}
