//go:build gateway || gateway.telegram

package gateway

import (
	"context"
	"log/slog"

	"github.com/EvilFreelancer/coddy-agent/external/gateway/telegram"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
)

// Start builds all enabled gateway adapters and runs the hub. Blocks until ctx is cancelled.
func Start(ctx context.Context, cfg *config.Config, mgr *session.Manager, log *slog.Logger, defaultCWD string) {
	var adapters []Adapter

	if cfg.Gateways.Telegram.Enabled {
		bot := telegram.New(&cfg.Gateways.Telegram, mgr, defaultCWD, log)
		adapters = append(adapters, bot)
	}

	if len(adapters) == 0 {
		log.Warn("gateway: no adapters enabled; set gateways.telegram.enabled: true in config")
		return
	}

	hub := NewHub(log, adapters...)
	hub.Start(ctx)
}
