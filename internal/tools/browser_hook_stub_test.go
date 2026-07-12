//go:build !browser

package tools

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// Without the browser build tag the registration hook is a no-op even when the
// config enables the browser, so no browser tools reach the registry.
func TestBrowserToolsNoopWithoutBuildTag(t *testing.T) {
	r := NewRegistryFor(&config.Config{Browser: config.BrowserConfig{Enabled: true}})
	if _, ok := r.Get("foxxycode_browser_navigate"); ok {
		t.Error("browser tools must not register without the browser build tag")
	}
}
