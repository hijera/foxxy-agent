package llm

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type diagnosticContextKey struct{}

var diagnosticID atomic.Uint64

// WithDiagnostics carries the configured logger into SDK calls without changing global logging.
// Request bodies, headers, URL paths, query strings and raw errors are never logged here.
func WithDiagnostics(ctx context.Context, log *slog.Logger) context.Context {
	if log == nil || !log.Enabled(ctx, slog.LevelDebug) {
		return ctx
	}
	return context.WithValue(ctx, diagnosticContextKey{}, log)
}

func diagnosticLogger(ctx context.Context) *slog.Logger {
	log, _ := ctx.Value(diagnosticContextKey{}).(*slog.Logger)
	return log
}

func diagnosticEvent(ctx context.Context, event string, args ...any) {
	if log := diagnosticLogger(ctx); log != nil {
		log.DebugContext(ctx, event, args...)
	}
}

// startDiagnosticCall correlates SDK retries and outer retries for one provider operation.
func startDiagnosticCall(ctx context.Context, operation string) (context.Context, func()) {
	log := diagnosticLogger(ctx)
	if log == nil {
		return ctx, func() {}
	}
	started := time.Now()
	log = log.With("call_id", diagnosticID.Add(1), "operation", operation)
	ctx = context.WithValue(ctx, diagnosticContextKey{}, log)
	log.DebugContext(ctx, "llm.call.start")
	return ctx, func() { log.DebugContext(ctx, "llm.call.end", "elapsed_ms", time.Since(started).Milliseconds()) }
}

func diagnosticError(err error) []any {
	var ne net.Error
	return []any{
		"error_type", fmt.Sprintf("%T", err),
		"timeout", errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()),
		"canceled", errors.Is(err, context.Canceled),
	}
}

// diagnosticMiddleware runs once per SDK HTTP attempt, including SDK-managed retries.
// It leaves transport, proxy selection, timeouts, cancellation and SDK retry policy intact.
func diagnosticMiddleware(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	log := diagnosticLogger(req.Context())
	if log == nil {
		return next(req)
	}
	started := time.Now()
	log = log.With("http_id", diagnosticID.Add(1), "host", req.URL.Host, "scheme", req.URL.Scheme)
	emit := func(event string, args ...any) {
		log.DebugContext(req.Context(), "llm.http."+event, append([]any{"elapsed_ms", time.Since(started).Milliseconds()}, args...)...)
	}
	retry, _ := strconv.Atoi(req.Header.Get("X-Stainless-Retry-Count"))
	emit("start", "method", req.Method, "sdk_retry", retry)
	trace := &httptrace.ClientTrace{
		GetConn:           func(_ string) { emit("get_connection") },
		DNSStart:          func(_ httptrace.DNSStartInfo) { emit("dns_start") },
		DNSDone:           func(info httptrace.DNSDoneInfo) { emit("dns_done", diagnosticError(info.Err)...) },
		ConnectStart:      func(network, addr string) { emit("connect_start", "network", network, "address", addr) },
		ConnectDone:       func(_, _ string, err error) { emit("connect_done", diagnosticError(err)...) },
		TLSHandshakeStart: func() { emit("tls_start") },
		TLSHandshakeDone:  func(_ tls.ConnectionState, err error) { emit("tls_done", diagnosticError(err)...) },
		GotConn: func(info httptrace.GotConnInfo) {
			emit("connection", "reused", info.Reused, "idle_ms", info.IdleTime.Milliseconds())
		},
		WroteRequest:         func(info httptrace.WroteRequestInfo) { emit("request_written", diagnosticError(info.Err)...) },
		GotFirstResponseByte: func() { emit("first_response_byte") },
	}
	resp, err := next(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		emit("error", diagnosticError(err)...)
		return resp, err
	}
	emit("headers", "status", resp.StatusCode)
	if resp.Body != nil {
		resp.Body = &diagnosticBody{ReadCloser: resp.Body, emit: emit}
	}
	return resp, nil
}

type diagnosticBody struct {
	io.ReadCloser
	emit  func(string, ...any)
	first sync.Once
	end   sync.Once
}

func (b *diagnosticBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.first.Do(func() { b.emit("first_body_byte") })
	}
	if err != nil {
		b.end.Do(func() { b.emit("body_end", append([]any{"eof", errors.Is(err, io.EOF)}, diagnosticError(err)...)...) })
	}
	return n, err
}

func (b *diagnosticBody) Close() error {
	err := b.ReadCloser.Close()
	b.end.Do(func() { b.emit("body_closed", diagnosticError(err)...) })
	return err
}
