//go:build http

package main

import (
	"github.com/hijera/foxxycode-agent/external/httpserver"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func runHTTP(args []string) error {
	return httpserver.Run(args, httpserver.CommandDeps{
		NewServerRef: func(pp **acp.Server, cfg *config.Config, live func() *config.Config) acp.UpdateSender {
			return &serverRef{p: pp, cfg: cfg, live: live}
		},
		EnsureHome: ensureFoxxyCodeHomeLayout,
		OpenStore:  openSessionStore,
	})
}
