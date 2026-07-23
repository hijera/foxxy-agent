package config

import (
	"flag"
	"strings"
)

// PlanNoSelfRunFlagName is the CLI flag (on `foxxycode acp` / `foxxycode http`) that
// overrides tools.plan_no_self_run. Editor plugins pass it so their panels default to
// the guarded behaviour while standalone runs keep the config value.
const PlanNoSelfRunFlagName = "plan-no-self-run"

// ApplyPlanNoSelfRunFlag overrides tools.plan_no_self_run only when the
// -plan-no-self-run flag was explicitly provided on fs; otherwise the config value
// (which defaults to false) is left untouched. Mirrors ApplySkillsAutoDiscoveryFlag.
func ApplyPlanNoSelfRunFlag(fs *flag.FlagSet, cfg *Config, val *bool) {
	if fs == nil || cfg == nil || val == nil {
		return
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == PlanNoSelfRunFlagName {
			v := *val
			cfg.Tools.PlanNoSelfRun = &v
		}
	})
}

// Permission mode constants for tools.permission_mode.
const (
	// PermModeAsk asks for user approval before each shell command and each file write.
	PermModeAsk = "ask"
	// PermModeAcceptEdits auto-approves file writes but still asks for shell commands.
	PermModeAcceptEdits = "accept_edits"
	// PermModeBypass skips all permission prompts (use only in fully trusted environments).
	PermModeBypass = "bypass"
)

// Tools is the YAML tools section (key tools).
type Tools struct {
	// PermissionMode controls when the agent asks for user approval before running tools.
	// Values: "ask" (default), "accept_edits", "bypass".
	PermissionMode   string   `yaml:"permission_mode"`
	CommandAllowlist []string `yaml:"command_allowlist"`

	// SSHConnectTimeout is the TCP dial timeout for SSH connections in seconds (default: 30).
	SSHConnectTimeout int `yaml:"ssh_connect_timeout"`

	// PlanNoSelfRun forbids the model from starting to execute a plan on its own. When
	// true, plan mode no longer offers plan_exit and the mode tool allowlist is enforced
	// at execution time, so a tool call outside it is refused instead of run. Defaults to
	// false; editor plugins turn it on via PlanNoSelfRunFlagName.
	PlanNoSelfRun *bool `yaml:"plan_no_self_run"`
}

// PlanNoSelfRunEnabled reports whether the model is barred from leaving plan mode itself.
func (c *Tools) PlanNoSelfRunEnabled() bool {
	return c.PlanNoSelfRun != nil && *c.PlanNoSelfRun
}

// ResolvedPermMode returns PermissionMode with a safe default of PermModeAsk.
func (c *Tools) ResolvedPermMode() string {
	switch c.PermissionMode {
	case PermModeAsk, PermModeAcceptEdits, PermModeBypass:
		return c.PermissionMode
	default:
		return PermModeAsk
	}
}

// Validate trims allowlist entries in place and normalises PermissionMode.
func (c *Tools) Validate() error {
	if c.PermissionMode == "" {
		c.PermissionMode = PermModeAsk
	}
	for i := range c.CommandAllowlist {
		c.CommandAllowlist[i] = strings.TrimSpace(c.CommandAllowlist[i])
	}
	if c.SSHConnectTimeout <= 0 {
		c.SSHConnectTimeout = 30
	}
	return nil
}
