package config_test

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestEffectiveLoopStuckAction(t *testing.T) {
	cases := []struct {
		name string
		set  string
		want string
	}{
		{name: "unset defaults to quarantine", set: "", want: config.AgentLoopStuckActionQuarantine},
		{name: "explicit quarantine", set: "quarantine", want: config.AgentLoopStuckActionQuarantine},
		{name: "explicit stop", set: "stop", want: config.AgentLoopStuckActionStop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := config.Agent{LoopStuckAction: tc.set}
			if got := a.EffectiveLoopStuckAction(); got != tc.want {
				t.Fatalf("EffectiveLoopStuckAction() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAgentValidateLoopStuckAction(t *testing.T) {
	for _, ok := range []string{"", config.AgentLoopStuckActionQuarantine, config.AgentLoopStuckActionStop} {
		a := config.Agent{Model: "m", LoopStuckAction: ok}
		if err := a.Validate(); err != nil {
			t.Fatalf("loop_stuck_action %q rejected: %v", ok, err)
		}
	}
	a := config.Agent{Model: "m", LoopStuckAction: "halt"}
	err := a.Validate()
	if err == nil {
		t.Fatal("an unknown action must be rejected at load time, not silently ignored")
	}
	if !strings.Contains(err.Error(), "loop_stuck_action") {
		t.Fatalf("error does not name the field: %v", err)
	}
}
