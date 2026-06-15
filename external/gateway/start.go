//go:build gateway || gateway.telegram

package gateway

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/EvilFreelancer/coddy-agent/external/gateway/telegram"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
)

// Start builds all enabled gateway adapters and runs the hub. Blocks until ctx is cancelled.
func Start(ctx context.Context, cfg *config.Config, mgr *session.Manager, log *slog.Logger, defaultCWD string) {
	var adapters []Adapter

	if cfg.Gateways.Telegram.Enabled {
		if cfg.Gateways.Telegram.EffectiveToken() == "" {
			log.Warn("gateway: telegram enabled but no token; set gateways.telegram.token or the " +
				config.TelegramBotTokenEnvVar + " environment variable")
		} else {
			storePath := filepath.Join(cfg.ResolvedSessionsRoot(), "gateway_sessions.json")
			bot := telegram.New(&cfg.Gateways.Telegram, mgr, defaultCWD, log, storePath)
			adapters = append(adapters, bot)
		}
	}

	if len(adapters) == 0 {
		log.Warn("gateway: no adapters enabled; set gateways.telegram.enabled: true in config")
		return
	}

	hub := NewHub(log, adapters...)
	hub.Start(ctx)
}
