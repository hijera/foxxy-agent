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

func TestConfigJSONRoundTripUILocale(t *testing.T) {
	paths := Paths{Home: t.TempDir(), CWD: t.TempDir()}
	j := ConfigJSON{UI: UIJSON{Locale: "ru"}}
	cfg := JSONDTOToConfig(&j, paths)
	if cfg.UI.Locale != "ru" {
		t.Fatalf("got locale %q", cfg.UI.Locale)
	}
	out := ConfigToJSONDTO(cfg)
	if out.UI.Locale != "ru" {
		t.Fatalf("dto locale %q", out.UI.Locale)
	}
}
