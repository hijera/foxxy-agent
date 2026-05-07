//go:build scheduler

package tools

import (
	"github.com/EvilFreelancer/coddy-agent/external/scheduler/tools"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
)

func registerSchedulerTools(r *Registry, cfg *config.Config) {
	if cfg == nil || !cfg.SchedulerEffectiveEnabled() {
		return
	}
	schedtools.RegisterTools(r.Register, cfg)
}
