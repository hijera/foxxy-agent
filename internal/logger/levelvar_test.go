package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// TestNewReturnsLevelVar proves the handler level can be flipped at runtime via the
// returned *slog.LevelVar without rebuilding the logger — the contract ReplaceConfig
// relies on for the debug.enabled toggle.
func TestNewReturnsLevelVar(t *testing.T) {
	var buf bytes.Buffer
	lv := NewLevelVar(config.LogLevelWarn)
	handler := newHandler(&buf, config.Logger{Level: config.LogLevelWarn, Format: config.LogFormatText}, lv)
	log := slog.New(handler)

	// At warn level, debug and info are suppressed.
	log.Debug("hidden-debug")
	log.Info("hidden-info")
	log.Warn("visible-warn")
	if strings.Contains(buf.String(), "hidden") {
		t.Fatalf("warn level should suppress debug/info, got: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "visible-warn") {
		t.Fatalf("warn should be emitted, got: %q", buf.String())
	}

	// Flip to debug at runtime: now a debug line appears.
	buf.Reset()
	lv.Set(slog.LevelDebug)
	log.Debug("now-visible-debug")
	if !strings.Contains(buf.String(), "now-visible-debug") {
		t.Fatalf("after lv.Set(debug), debug should be emitted, got: %q", buf.String())
	}
}

func TestEffectiveLevel(t *testing.T) {
	if got := EffectiveLevel(true, config.LogLevelInfo); got != slog.LevelDebug {
		t.Errorf("EffectiveLevel(true,...) = %v, want debug", got)
	}
	if got := EffectiveLevel(false, config.LogLevelWarn); got != slog.LevelWarn {
		t.Errorf("EffectiveLevel(false,warn) = %v, want warn", got)
	}
	if got := EffectiveLevel(false, ""); got != slog.LevelInfo {
		t.Errorf("EffectiveLevel(false,\"\") = %v, want info (default)", got)
	}
}
