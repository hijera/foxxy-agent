//go:build gateway || gateway.telegram

package gateway

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/hijera/foxxycode-agent/external/gateway/telegram"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/logger"
	"github.com/hijera/foxxycode-agent/internal/session"
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
			// The standalone `foxxycode gateway` command has no turn mirror: it is
			// the one process, and nothing else is watching its turns.
			bot := telegram.New(&cfg.Gateways.Telegram, mgr, defaultCWD,
				logger.Component(log, logger.ComponentGatewayTelegram), storePath, nil)
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
