//go:build !scheduler

package scheduler

import (
	"context"
	"log/slog"

	"github.com/EvilFreelancer/coddy-agent/internal/config"
)

// Start is a no-op when built without the scheduler tag.
func Start(_ context.Context, _ *config.Config, _ *slog.Logger, _ string) {}
