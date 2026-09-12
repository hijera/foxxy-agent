package llm

import (
	"log/slog"
	"sync/atomic"
)

// debugCapture gates raw LLM HTTP request/response logging (see debug_transport.go).
// It is a package-level switch because the *http.Client is built once per provider
// and has no handle back to config: the CLI flags and PUT /foxxycode/config flip
// this atomically at startup and on reload instead.
var debugCapture atomic.Bool

// debugLog receives the raw LLM HTTP excerpts. It defaults to slog.Default() so the
// transport works without wiring; entry points call SetDebugLogger with the process
// logger so dumps land in the rotating file alongside other logs.
var debugLog atomic.Pointer[slog.Logger]

// SetDebugCapture enables or disables raw LLM HTTP body logging process-wide.
// Called from the entry points after the logger is built and from the HTTP
// server's ReplaceConfig when debug.enabled is toggled at runtime.
func SetDebugCapture(on bool) { debugCapture.Store(on) }

// DebugCaptureEnabled reports whether raw LLM HTTP bodies are being logged.
func DebugCaptureEnabled() bool { return debugCapture.Load() }

// SetDebugLogger routes raw LLM HTTP dumps to the given logger. Pass the process
// logger so the excerpts are written to the same rotating output as other logs.
func SetDebugLogger(l *slog.Logger) { debugLog.Store(l) }

// debugLogger returns the configured logger, falling back to slog.Default().
func debugLogger() *slog.Logger {
	if l := debugLog.Load(); l != nil {
		return l
	}
	return slog.Default()
}
