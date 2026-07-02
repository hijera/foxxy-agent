package session

import (
	"github.com/hijera/foxxy-agent/internal/config"
	"github.com/hijera/foxxy-agent/internal/rules"
)

// DiscoverRules loads project rules from cwd using config.
func DiscoverRules(cfg *config.Config, cwd string) []*rules.Rule {
	if cfg == nil || !cfg.Rules.AutoDiscoverEnabled() {
		return nil
	}
	cat, err := rules.DefaultFactory().Discover(cwd, rules.ParseSystems(cfg.Rules.Systems))
	if err != nil {
		return nil
	}
	return cat
}
