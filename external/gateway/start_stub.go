//go:build !(gateway || gateway.telegram)

// Package gateway is a no-op when built without the gateway or gateway.telegram tag.
package gateway

import (
	"context"
	"log/slog"

	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
)

// Start is a no-op when built without messenger gateway tags.
func Start(_ context.Context, _ *config.Config, _ *session.Manager, _ *slog.Logger, _ string) {}
