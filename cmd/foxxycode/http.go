//go:build http

package main

import (
	"github.com/hijera/foxxycode-agent/external/httpserver"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
)

// httpAvailable reports whether `foxxycode http` is compiled in. Specs gate their
// @http scenarios on it instead of failing where the surface does not exist.
const httpAvailable = true

func runHTTP(args []string) error {
	return httpserver.Run(args, httpserver.CommandDeps{
		NewServerRef: func(pp **acp.Server, cfg *config.Config, live func() *config.Config) acp.UpdateSender {
			return &serverRef{p: pp, cfg: cfg, live: live}
		},
		EnsureHome:  ensureFoxxyCodeHomeLayout,
		OpenStore:   openSessionStore,
		CheckOutput: configTestOutput,
	})
}
