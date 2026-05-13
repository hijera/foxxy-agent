//go:build http

package main

import (
	"github.com/EvilFreelancer/coddy-agent/external/httpserver"
	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
)

func runHTTP(args []string) error {
	return httpserver.Run(args, httpserver.CommandDeps{
		NewServerRef: func(pp **acp.Server, cfg *config.Config, live func() *config.Config) acp.UpdateSender {
			return &serverRef{p: pp, cfg: cfg, live: live}
		},
		EnsureHome: ensureCoddyHomeLayout,
		OpenStore:  openSessionStore,
	})
}
