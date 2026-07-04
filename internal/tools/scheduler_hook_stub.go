//go:build !scheduler

package tools

import "github.com/hijera/foxxycode-agent/internal/config"

func registerSchedulerTools(_ *Registry, _ *config.Config) {}
