//go:build scheduler

package tools

import (
	"github.com/hijera/foxxycode-agent/external/scheduler/tools"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func registerSchedulerTools(r *Registry, cfg *config.Config) {
	if cfg == nil || !cfg.SchedulerEffectiveEnabled() {
		return
	}
	schedtools.RegisterTools(r.Register, cfg)
}
