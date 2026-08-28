package logger

import (
	"io"
	"log/slog"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// newHandler builds the slog.Handler matching cfg.Format. The level comes from lv,
// a shared *slog.LevelVar, so the caller can change verbosity at runtime via lv.Set
// (used by the debug.enabled toggle through PUT /foxxycode/config) without
// rebuilding the handler or the logger.
func newHandler(w io.Writer, cfg config.Logger, lv *slog.LevelVar) slog.Handler {
	opts := &slog.HandlerOptions{Level: lv}
	if cfg.Format == config.LogFormatJSON {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

func levelOf(name string) slog.Level {
	switch name {
	case config.LogLevelDebug:
		return slog.LevelDebug
	case config.LogLevelWarn:
		return slog.LevelWarn
	case config.LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLevelVar returns a *slog.LevelVar set to the named level (debug|info|warn|error,
// default info). Because *slog.LevelVar implements slog.Leveler, handing it to a handler
// lets the level change later with Set.
func NewLevelVar(level string) *slog.LevelVar {
	lv := new(slog.LevelVar)
	lv.Set(levelOf(level))
	return lv
}

// EffectiveLevel returns the slog.Level the process logger should use: debug when the
// diagnostics master switch is on (the --debug CLI flag / debug.enabled), otherwise the
// configured level name.
func EffectiveLevel(debugEnabled bool, cfgLevel string) slog.Level {
	if debugEnabled {
		return slog.LevelDebug
	}
	return levelOf(cfgLevel)
}
