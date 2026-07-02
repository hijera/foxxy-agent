//go:build scheduler

package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/hijera/foxxy-agent/external/scheduler/service"
	"github.com/hijera/foxxy-agent/external/scheduler/storage"
	"github.com/hijera/foxxy-agent/internal/config"
)

func init() {
	schedservice.LaunchManualJob = launchManualScheduledJob
}

func launchManualScheduledJob(ctx context.Context, cfg *config.Config, log *slog.Logger, processCWD, absJob string, fm *storage.JobFrontmatter, instruction string) error {
	if fm == nil {
		return nil
	}
	return RunJobFile(ctx, cfg, log, processCWD, absJob, time.Time{}, false, fm, instruction)
}
