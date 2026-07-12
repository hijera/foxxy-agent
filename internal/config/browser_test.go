package config_test

import (
	"encoding/json"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestBrowserDefaults(t *testing.T) {
	var c config.BrowserConfig
	c.ApplyDefaults()

	if c.Enabled {
		t.Error("browser should default to disabled")
	}
	if !c.HeadlessEnabled() {
		t.Error("browser should default to headless")
	}
	if c.TimeoutSeconds != config.BrowserDefaultTimeoutSeconds {
		t.Errorf("TimeoutSeconds = %d, want %d", c.TimeoutSeconds, config.BrowserDefaultTimeoutSeconds)
	}
}

func TestBrowserExplicitHeadlessFalsePreserved(t *testing.T) {
	off := false
	c := config.BrowserConfig{Headless: &off}
	c.ApplyDefaults()
	if c.HeadlessEnabled() {
		t.Error("explicit browser.headless=false must be preserved")
	}
}

func TestBrowserJSONRoundTrip(t *testing.T) {
	on := true
	src := config.BrowserConfig{
		Enabled:        true,
		Headless:       &on,
		ExecutablePath: "/usr/bin/chromium",
		TimeoutSeconds: 45,
	}
	cfg := &config.Config{Browser: src}
	dto := config.ConfigToJSONDTO(cfg)
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var j config.ConfigJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	back := config.JSONDTOToConfig(&j, config.Paths{})
	if !back.Browser.Enabled {
		t.Error("Enabled lost in round-trip")
	}
	if !back.Browser.HeadlessEnabled() {
		t.Error("Headless lost in round-trip")
	}
	if back.Browser.ExecutablePath != "/usr/bin/chromium" {
		t.Errorf("ExecutablePath = %q", back.Browser.ExecutablePath)
	}
	if back.Browser.TimeoutSeconds != 45 {
		t.Errorf("TimeoutSeconds = %d, want 45", back.Browser.TimeoutSeconds)
	}
}

func TestBrowserSchemaCoversConfigJSON(t *testing.T) {
	// The UI schema must expose the browser section (parity check that also guards drift).
	if err := config.UISchemaCoversConfigJSONFields(); err != nil {
		t.Fatalf("UISchemaCoversConfigJSONFields: %v", err)
	}
	doc := config.UISchemaMap()
	props, ok := doc["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema missing properties")
	}
	if _, ok := props["browser"].(map[string]interface{}); !ok {
		t.Fatal("schema missing browser section")
	}
}
