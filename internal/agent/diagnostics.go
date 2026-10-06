package agent

import (
	"context"
	"log/slog"
	"time"
)

// debugStage marks work before the first-token timer, which can otherwise look like a network stall.
func (a *Agent) debugStage(ctx context.Context, stage string) func() {
	if a.log == nil || !a.log.Enabled(ctx, slog.LevelDebug) {
		return func() {}
	}
	started := time.Now()
	log := a.log.With("session", a.state.GetID(), "stage", stage)
	log.DebugContext(ctx, "agent.stage.start")
	return func() { log.DebugContext(ctx, "agent.stage.end", "elapsed_ms", time.Since(started).Milliseconds()) }
}
