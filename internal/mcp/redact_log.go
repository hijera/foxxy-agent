package mcp

import (
	"context"
	"log/slog"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// redactingHandler also protects diagnostics emitted inside a transport,
// including a failed HTTP attempt whose legacy-SSE fallback then succeeds.
type redactingHandler struct {
	slog.Handler
	server config.MCPServerConfig
	cwd    string
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(record.Time, record.Level, RedactValues(h.server, h.cwd, record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		safe.AddAttrs(h.attr(attr))
		return true
	})
	return h.Handler.Handle(ctx, safe)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	safe := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		safe[i] = h.attr(attr)
	}
	h.Handler = h.Handler.WithAttrs(safe)
	return h
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	h.Handler = h.Handler.WithGroup(name)
	return h
}

func (h redactingHandler) attr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	switch attr.Value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(RedactValues(h.server, h.cwd, attr.Value.String()))
	case slog.KindAny:
		if err, ok := attr.Value.Any().(error); ok {
			attr.Value = slog.AnyValue(redactError(h.server, h.cwd, err))
		}
	case slog.KindGroup:
		group := attr.Value.Group()
		safe := make([]slog.Attr, len(group))
		for i, child := range group {
			safe[i] = h.attr(child)
		}
		attr.Value = slog.GroupValue(safe...)
	}
	return attr
}
