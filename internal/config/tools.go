package config

import (
	"flag"
	"fmt"
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

	// AskDisableExtendedTools keeps Ask mode on the basic repository-reading
	// surface. When false (the default), Ask also receives read-only shell, web,
	// annotated MCP, and scheduler inspection tools.
	AskDisableExtendedTools bool `yaml:"ask_disable_extended_tools"`

	// OutputLimits caps how many lines each tool result or error contributes to
	// the LLM context. Positive limits also activate a hard byte safety ceiling.
	OutputLimits ToolOutputLimits `yaml:"output_limits"`
}

const (
	OutputLimitDefaultRead          = 1000
	OutputLimitDefaultGrep          = 200
	OutputLimitDefaultGlob          = 300
	OutputLimitDefaultPrintTree     = 400
	OutputLimitDefaultRunCommand    = 500
	OutputLimitDefaultSSHRunCommand = 500
	OutputLimitDefaultWebFetch      = 800
	OutputLimitDefaultWebSearch     = 200
	OutputLimitDefaultDefault       = 1000
)

// ToolOutputLimits is the YAML tools.output_limits section. Pointer fields
// preserve the difference between an omitted value and an explicit zero.
type ToolOutputLimits struct {
	Read          *int `yaml:"read"`
	Grep          *int `yaml:"grep"`
	Glob          *int `yaml:"glob"`
	PrintTree     *int `yaml:"print_tree"`
	RunCommand    *int `yaml:"run_command"`
	SSHRunCommand *int `yaml:"ssh_run_command"`
	WebFetch      *int `yaml:"webfetch"`
	WebSearch     *int `yaml:"websearch"`
	Default       *int `yaml:"default"`
}

const outputLimitDefaultKey = ""

// MaxLines returns the effective line ceiling for a tool. Zero disables both
// line and byte limiting.
func (l *ToolOutputLimits) MaxLines(tool string) int {
	if l == nil {
		return 0
	}
	pick := func(p *int, def int) int {
		if p != nil {
			return *p
		}
		return def
	}
	switch tool {
	case "read":
		return pick(l.Read, OutputLimitDefaultRead)
	case "grep":
		return pick(l.Grep, OutputLimitDefaultGrep)
	case "glob":
		return pick(l.Glob, OutputLimitDefaultGlob)
	case "print_tree":
		return pick(l.PrintTree, OutputLimitDefaultPrintTree)
	case "run_command":
		return pick(l.RunCommand, OutputLimitDefaultRunCommand)
	case "ssh_run_command":
		return pick(l.SSHRunCommand, OutputLimitDefaultSSHRunCommand)
	case "webfetch":
		return pick(l.WebFetch, OutputLimitDefaultWebFetch)
	case "websearch":
		return pick(l.WebSearch, OutputLimitDefaultWebSearch)
	default:
		return pick(l.Default, OutputLimitDefaultDefault)
	}
}

// AsMap materializes effective per-tool ceilings for the execution layer.
func (l *ToolOutputLimits) AsMap() map[string]int {
	names := []string{
		"read", "grep", "glob", "print_tree",
		"run_command", "ssh_run_command", "webfetch", "websearch",
	}
	out := make(map[string]int, len(names)+1)
	for _, name := range names {
		out[name] = l.MaxLines(name)
	}
	out[outputLimitDefaultKey] = l.MaxLines(outputLimitDefaultKey)
	return out
}

func (l *ToolOutputLimits) validate() error {
	for name, p := range map[string]*int{
		"read": l.Read, "grep": l.Grep, "glob": l.Glob, "print_tree": l.PrintTree,
		"run_command": l.RunCommand, "ssh_run_command": l.SSHRunCommand,
		"webfetch": l.WebFetch, "websearch": l.WebSearch, "default": l.Default,
	} {
		if p != nil && *p < 0 {
			return fmt.Errorf("tools.output_limits.%s: must be >= 0", name)
		}
	}
	return nil
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
	if err := c.OutputLimits.validate(); err != nil {
		return err
	}
	return nil
}
