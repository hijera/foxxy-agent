//go:build desktop

package main

import (
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/desktop"
)

func runDesktop(args []string) error {
	return desktop.Run(desktop.Options{
		Args:            args,
		EnsureHome:      ensureFoxxyCodeHomeLayout,
		BootstrapConfig: bootstrapExampleConfig,
		OpenStore:       openSessionStore,
		NewServerRef: func(pp **acp.Server, cfg *config.Config, live func() *config.Config) acp.UpdateSender {
			return &serverRef{p: pp, cfg: cfg, live: live}
		},
	})
}

func defaultRun() error {
	return runDesktop(nil)
}
