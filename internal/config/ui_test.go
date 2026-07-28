package config

import "testing"

func TestUIConfigValidate(t *testing.T) {
	tests := []struct {
		locale string
		ok     bool
	}{
		{"", true},
		{"en", true},
		{"ru", true},
		{"de", false},
	}
	for _, tc := range tests {
		u := UIConfig{Locale: tc.locale}
		u.Normalize()
		err := u.Validate()
		if tc.ok && err != nil {
			t.Fatalf("locale %q: unexpected error: %v", tc.locale, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("locale %q: expected error", tc.locale)
		}
	}
}

func TestUIConfigSendModeValidate(t *testing.T) {
	tests := []struct {
		sendMode string
		ok       bool
	}{
		{"enter", true},
		{"ctrl_enter", true},
		{"off", true},
		{"CTRL_ENTER", true}, // normalized to lower-case
		{"shift", false},
	}
	for _, tc := range tests {
		u := UIConfig{SendMode: tc.sendMode}
		u.Normalize()
		err := u.Validate()
		if tc.ok && err != nil {
			t.Fatalf("send_mode %q: unexpected error: %v", tc.sendMode, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("send_mode %q: expected error", tc.sendMode)
		}
	}
}

func TestUIConfigSendModeDefault(t *testing.T) {
	u := UIConfig{SendMode: "  "}
	u.Normalize()
	if u.SendMode != UISendModeEnter {
		t.Fatalf("empty send_mode should normalize to %q, got %q", UISendModeEnter, u.SendMode)
	}
}

func TestUIConfigStatusLineDefault(t *testing.T) {
	var u UIConfig
	if !u.IsStatusLineEnabled() {
		t.Fatal("unset status_line should default to enabled")
	}
	off := false
	u.StatusLine = &off
	if u.IsStatusLineEnabled() {
		t.Fatal("status_line=false should disable the status line")
	}
	on := true
	u.StatusLine = &on
	if !u.IsStatusLineEnabled() {
		t.Fatal("status_line=true should enable the status line")
	}
}

func TestConfigJSONRoundTripUIStatusLine(t *testing.T) {
	paths := Paths{Home: t.TempDir(), CWD: t.TempDir()}
	off := false
	j := ConfigJSON{UI: UIJSON{StatusLine: &off}}
	cfg := JSONDTOToConfig(&j, paths)
	if cfg.UI.StatusLine == nil || *cfg.UI.StatusLine {
		t.Fatalf("got status_line %v", cfg.UI.StatusLine)
	}
	out := ConfigToJSONDTO(cfg)
	if out.UI.StatusLine == nil || *out.UI.StatusLine {
		t.Fatalf("dto status_line %v", out.UI.StatusLine)
	}

	// An absent key must round-trip as absent, not as an explicit false.
	empty := ConfigJSON{}
	if got := ConfigToJSONDTO(JSONDTOToConfig(&empty, paths)).UI.StatusLine; got != nil {
		t.Fatalf("absent status_line should stay nil, got %v", *got)
	}
}

func TestConfigJSONRoundTripUILocale(t *testing.T) {
	paths := Paths{Home: t.TempDir(), CWD: t.TempDir()}
	j := ConfigJSON{UI: UIJSON{Locale: "ru", SendMode: "ctrl_enter"}}
	cfg := JSONDTOToConfig(&j, paths)
	if cfg.UI.Locale != "ru" {
		t.Fatalf("got locale %q", cfg.UI.Locale)
	}
	if cfg.UI.SendMode != "ctrl_enter" {
		t.Fatalf("got send_mode %q", cfg.UI.SendMode)
	}
	out := ConfigToJSONDTO(cfg)
	if out.UI.Locale != "ru" {
		t.Fatalf("dto locale %q", out.UI.Locale)
	}
	if out.UI.SendMode != "ctrl_enter" {
		t.Fatalf("dto send_mode %q", out.UI.SendMode)
	}
}
