package config

import (
	"flag"
	"testing"
)

func TestPlanNoSelfRunDefaultsToOff(t *testing.T) {
	var tools Tools
	if err := tools.Validate(); err != nil {
		t.Fatal(err)
	}
	if tools.PlanNoSelfRunEnabled() {
		t.Fatal("plan_no_self_run must default to false")
	}
}

func TestPlanNoSelfRunHonoursExplicitConfigValue(t *testing.T) {
	on := true
	off := false
	if got := (&Tools{PlanNoSelfRun: &on}).PlanNoSelfRunEnabled(); !got {
		t.Error("explicit true must enable the guard")
	}
	if got := (&Tools{PlanNoSelfRun: &off}).PlanNoSelfRunEnabled(); got {
		t.Error("explicit false must disable the guard")
	}
}

func TestApplyPlanNoSelfRunFlagOnlyOverridesWhenPassed(t *testing.T) {
	off := false
	cfg := &Config{Tools: Tools{PlanNoSelfRun: &off}}

	untouched := flag.NewFlagSet("http", flag.ContinueOnError)
	val := untouched.Bool(PlanNoSelfRunFlagName, false, "")
	if err := untouched.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	ApplyPlanNoSelfRunFlag(untouched, cfg, val)
	if cfg.Tools.PlanNoSelfRunEnabled() {
		t.Fatal("config value must survive when the flag was not passed")
	}

	passed := flag.NewFlagSet("http", flag.ContinueOnError)
	val2 := passed.Bool(PlanNoSelfRunFlagName, false, "")
	if err := passed.Parse([]string{"-" + PlanNoSelfRunFlagName}); err != nil {
		t.Fatal(err)
	}
	ApplyPlanNoSelfRunFlag(passed, cfg, val2)
	if !cfg.Tools.PlanNoSelfRunEnabled() {
		t.Fatal("an explicitly passed flag must override the config value")
	}
}

func TestPlanNoSelfRunSurvivesJSONRoundTrip(t *testing.T) {
	on := true
	cfg := &Config{Tools: Tools{PermissionMode: PermModeAsk, PlanNoSelfRun: &on}}
	back := JSONDTOToConfig(ConfigToJSONDTO(cfg), Paths{})
	if !back.Tools.PlanNoSelfRunEnabled() {
		t.Fatal("plan_no_self_run lost through the JSON config DTO")
	}
}
