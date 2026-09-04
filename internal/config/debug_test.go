package config

import (
	"flag"
	"testing"
)

func TestDebugEffectiveCapture(t *testing.T) {
	falseVal := false
	trueVal := true
	cases := []struct {
		name string
		d    Debug
		want bool
	}{
		{name: "disabled", d: Debug{Enabled: false}, want: false},
		{name: "enabled capture nil defaults on", d: Debug{Enabled: true}, want: true},
		{name: "enabled capture true", d: Debug{Enabled: true, CaptureLLM: &trueVal}, want: true},
		{name: "enabled capture false opts out", d: Debug{Enabled: true, CaptureLLM: &falseVal}, want: false},
		{name: "disabled ignores capture", d: Debug{Enabled: false, CaptureLLM: &trueVal}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.EffectiveCapture(); got != tc.want {
				t.Errorf("EffectiveCapture() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDebugValidateApplyDefaultsNoop(t *testing.T) {
	d := Debug{Enabled: true}
	if err := d.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	d.ApplyDefaults() // must not panic or reset CaptureLLM semantics
	if !d.Effective() {
		t.Error("Effective should mirror Enabled")
	}
}

func TestApplyDebugFlagOnlyWhenPassed(t *testing.T) {
	cases := []struct {
		name   string
		passed bool
		value  bool
		preSet bool
		wantOn bool
	}{
		{name: "flag passed true forces on", passed: true, value: true, wantOn: true},
		{name: "flag passed false does not disable", passed: true, value: false, preSet: true, wantOn: true},
		{name: "flag omitted leaves config false", passed: false, wantOn: false},
		{name: "flag omitted preserves config true", passed: false, preSet: true, wantOn: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			v := fs.Bool(DebugFlagName, false, "")
			cfg := &Config{}
			cfg.Debug.Enabled = tc.preSet
			if err := fs.Parse(argsFor(tc.passed, tc.value)); err != nil {
				t.Fatal(err)
			}
			*v = tc.value
			ApplyDebugFlag(fs, cfg, v)
			if cfg.Debug.Enabled != tc.wantOn {
				t.Errorf("Debug.Enabled = %v, want %v", cfg.Debug.Enabled, tc.wantOn)
			}
		})
	}
}

func argsFor(passed bool, value bool) []string {
	if !passed {
		return nil
	}
	if value {
		return []string{"-" + DebugFlagName}
	}
	return []string{"-" + DebugFlagName + "=false"}
}
