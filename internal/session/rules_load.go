package session

import (
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/rules"
)

// DiscoverRules loads the rules that apply to a session: the operator's own
// ${FOXXYCODE_HOME}/rules (from cfg.Paths.Home) and the rule folders of the
// workspace at cwd.
func DiscoverRules(cfg *config.Config, cwd string) []*rules.Rule {
	if cfg == nil || !cfg.Rules.AutoDiscoverEnabled() {
		return nil
	}
	cat, err := rules.DefaultFactory(cfg.Paths.Home).Discover(cwd, rules.ParseSystems(cfg.Rules.Systems))
	if err != nil {
		return nil
	}
	return cat
}
