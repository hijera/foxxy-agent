//go:build !scheduler

package tools

import "github.com/hijera/foxxy-agent/internal/config"

func registerSchedulerTools(_ *Registry, _ *config.Config) {}
