package llm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticsDisabled(t *testing.T) {
	var output bytes.Buffer
	ctx := WithDiagnostics(context.Background(), slog.New(slog.NewTextHandler(&output, nil)))
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.test", nil)
	_, _ = diagnosticMiddleware(r, func(got *http.Request) (*http.Response, error) {
		if got != r {
			t.Fatal("disabled diagnostics changed request")
		}
		return nil, context.Canceled
	})
	if output.Len() != 0 {
		t.Fatal("diagnostics logged at info level")
	}
}

func TestDiagnosticsDoesNotLogSensitiveErrorOrURL(t *testing.T) {
	var output bytes.Buffer
	ctx := WithDiagnostics(context.Background(), slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://user:secret@example.test/private-key?token=secret", nil)
	wantErr := errors.New("secret error response with credentials")
	_, gotErr := diagnosticMiddleware(r, func(*http.Request) (*http.Response, error) { return nil, wantErr })
	if gotErr != wantErr {
		t.Fatal("middleware changed error")
	}
	for _, secret := range []string{"secret", "private-key", "user:", "token="} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("sensitive value in log: %s", output.String())
		}
	}
}

func TestDiagnosticsShowsRetry(t *testing.T) {
	// The SDK's own retries are disabled (WithMaxRetries(0)); the retry the
	// diagnostics see is the resilient wrapper's around the same transport.
	for _, kind := range []string{"openai", "anthropic"} {
		t.Run(kind, func(t *testing.T) {
			var output bytes.Buffer
			ctx := WithDiagnostics(context.Background(), slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					w.Header().Set("Retry-After", "0.001")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"error":{"message":"secret-error","type":"overloaded_error"}}`)
					return
				}
				if kind == "openai" {
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
				} else {
					_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`)
				}
			}))
			defer srv.Close()
			p, err := NewProvider(ProviderInput{Type: kind, Model: "test", BaseURL: srv.URL, APIKey: "secret-key"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Complete(ctx, nil, nil); err != nil {
				t.Fatal(err)
			}
			if calls != 2 || !strings.Contains(output.String(), "llm.retry.wait") || !strings.Contains(output.String(), "status=503") {
				t.Fatalf("retry missing: calls=%d log=%s", calls, output.String())
			}
		})
	}
}

func TestDiagnosticsShowsOuterRetry(t *testing.T) {
	var output bytes.Buffer
	ctx := WithDiagnostics(context.Background(), slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	calls := 0
	p := wrapResilient(&stubProvider{completeFn: func(context.Context, []Message, []ToolDefinition) (*Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("503 secret-provider-error")
		}
		return &Response{}, nil
	}}, ResilientOptions{RetryBase: time.Millisecond})
	if _, err := p.Complete(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "llm.retry.wait") || !strings.Contains(output.String(), "attempt=2") || strings.Contains(output.String(), "secret-provider-error") {
		t.Fatalf("unexpected retry log: %s", output.String())
	}
}

func TestDiagnosticsPreservesTLSStreamCancellation(t *testing.T) {
	var output bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = WithDiagnostics(ctx, slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := diagnosticMiddleware(req, srv.Client().Do)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	cancel()
	_, err = io.ReadAll(resp.Body)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("stream cancellation changed: %v", err)
	}
	for _, event := range []string{"llm.http.tls_start", "llm.http.tls_done", "llm.http.headers", "canceled=true"} {
		if !strings.Contains(output.String(), event) {
			t.Fatalf("missing %s in %s", event, output.String())
		}
	}
}
