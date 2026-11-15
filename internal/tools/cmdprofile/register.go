// Package cmdprofile registers the operator-declared command profiles from the
// commands: config section as builtin tools (cmd_<name>). The profile shape
// and every safety rule live in internal/cmdprofile; this package only bridges
// them into the tool registry.
package cmdprofile

import (
	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// Specs returns a deep copy of the config-declared profiles (nil-safe).
func Specs(cfg *config.Config) []cmdprofile.ProfileSpec {
	if cfg == nil {
		return nil
	}
	return cfg.CommandProfileSpecs()
}

// RegisterBuiltins registers one tool per declared profile. Unlike the svn
// family, a profile registers even when its binary is missing: the operator
// declared it explicitly, so a call must answer with install guidance rather
// than with "unknown tool". Invalid entries are skipped defensively — the
// config loader already rejects them, but a registry rebuild must never panic
// on a config that arrived by another path.
func RegisterBuiltins(add func(*tooling.Tool), cfg *config.Config) {
	for _, spec := range Specs(cfg) {
		if err := spec.Validate(); err != nil {
			continue
		}
		add(Tool(spec))
	}
}
