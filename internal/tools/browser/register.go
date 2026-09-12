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
	add(m.NavigateBackTool())
	add(m.ClickTool())
	add(m.FillTool())
	add(m.FillFormTool())
	add(m.PressKeyTool())
	add(m.SelectOptionTool())
	add(m.HoverTool())
	add(m.ScrollTool())
	add(m.DragTool())
	add(m.UploadFileTool())
	add(m.WaitForTool())
	add(m.HandleDialogTool())
	add(m.TabsTool())
	add(m.ResizeTool())
	add(m.ScreenshotTool())
	add(m.PageLogTool())
	add(m.ReadPageTool())
	add(m.FindTool())
	add(m.InspectTool())
	add(m.NetworkTool())
	add(m.RouteTool())
	add(m.StorageSetTool())
	add(m.EvaluateTool())
	add(m.CloseTool())
}

// ToolNames returns the tool names the browser subsystem registers. Used by tests
// and by mode/toolset wiring.
func ToolNames() []string {
	return []string{
		"foxxycode_browser_navigate",
		"foxxycode_browser_navigate_back",
		"foxxycode_browser_click",
		"foxxycode_browser_fill",
		"foxxycode_browser_fill_form",
		"foxxycode_browser_press_key",
		"foxxycode_browser_select_option",
		"foxxycode_browser_hover",
		"foxxycode_browser_scroll",
		"foxxycode_browser_drag",
		"foxxycode_browser_upload_file",
		"foxxycode_browser_wait_for",
		"foxxycode_browser_handle_dialog",
		"foxxycode_browser_tabs",
		"foxxycode_browser_resize",
		"foxxycode_browser_screenshot",
		"foxxycode_browser_page_log",
		"foxxycode_browser_read_page",
		"foxxycode_browser_find",
		"foxxycode_browser_inspect",
		"foxxycode_browser_network",
		"foxxycode_browser_route",
		"foxxycode_browser_storage_set",
		"foxxycode_browser_evaluate",
		"foxxycode_browser_close",
	}
}
