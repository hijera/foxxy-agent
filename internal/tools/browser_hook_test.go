//go:build browser

package tools

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestBrowserToolsRegisteredWhenEnabled(t *testing.T) {
	r := NewRegistryFor(&config.Config{Browser: config.BrowserConfig{Enabled: true}})
	for _, name := range []string{
		"foxxycode_browser_navigate",
		"foxxycode_browser_navigate_back",
		"foxxycode_browser_click",
		"foxxycode_browser_press_key",
		"foxxycode_browser_wait_for",
		"foxxycode_browser_tabs",
		"foxxycode_browser_select_option",
		"foxxycode_browser_handle_dialog",
		"foxxycode_browser_upload_file",
		"foxxycode_browser_resize",
		"foxxycode_browser_drag",
		"foxxycode_browser_fill_form",
		"foxxycode_browser_find",
		"foxxycode_browser_network",
		"foxxycode_browser_route",
		"foxxycode_browser_storage_set",
		"foxxycode_browser_screenshot",
		"foxxycode_browser_close",
	} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("tool %q not registered when browser is enabled", name)
		}
	}
}

func TestBrowserToolsAbsentWhenDisabled(t *testing.T) {
	r := NewRegistryFor(&config.Config{Browser: config.BrowserConfig{Enabled: false}})
	if _, ok := r.Get("foxxycode_browser_navigate"); ok {
		t.Error("browser tools must not register when browser.enable is false")
	}
}
