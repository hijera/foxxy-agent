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
		"foxxycode_browser_click",
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
		t.Error("browser tools must not register when browser.enabled is false")
	}
}
