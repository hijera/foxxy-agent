//go:build !scheduler

package tools

import "github.com/EvilFreelancer/coddy-agent/internal/config"

func registerSchedulerTools(_ *Registry, _ *config.Config) {}
