package config

import (
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

// validateCommandProfiles normalizes and validates the commands: section. The
// rules themselves live in internal/cmdprofile — the same type carries the
// YAML config shape and the portable Mini App document shape, so config and
// documents cannot drift apart.
func validateCommandProfiles(cfg *Config) error {
	seen := make(map[string]int, len(cfg.Commands))
	for index := range cfg.Commands {
		profile := &cfg.Commands[index]
		profile.Name = strings.ToLower(strings.TrimSpace(profile.Name))
		profile.Binary = strings.TrimSpace(profile.Binary)
		profile.Permission = strings.ToLower(strings.TrimSpace(profile.Permission))
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("commands[%d]: %w", index, err)
		}
		if previous, duplicate := seen[profile.Name]; duplicate {
			return fmt.Errorf("commands[%d]: duplicate profile name %q (already declared at commands[%d])", index, profile.Name, previous)
		}
		seen[profile.Name] = index
	}
	return nil
}

// CommandProfileSpecs returns a deep copy of the declared profiles, so callers
// can hold them across a config reload without sharing state.
func (c *Config) CommandProfileSpecs() []cmdprofile.ProfileSpec {
	if c == nil {
		return nil
	}
	return cmdprofile.CloneSpecs(c.Commands)
}
