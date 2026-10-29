package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestUpstreamStatus: the status a provider answered with is read from the
// typed errors only, so a digit inside a message never passes for one.
func TestUpstreamStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"openai 400", retryHTTPError(t, "openai", 400, nil), 400},
		{"anthropic 529", retryHTTPError(t, "anthropic", 529, nil), 529},
		{"stream error after text", fmt.Errorf("openai stream: %w", &streamServerError{code: 500, emitted: true}), 500},
		{"quota reset", &QuotaResetError{Cause: retryHTTPError(t, "openai", 429, nil)}, 429},
		{"plain text", errors.New("dial failed after 500 ms"), 0},
		{"devin 503", &devinAPIError{status: 503}, 503},
		{"nil", nil, 0},
	}
	for _, c := range cases {
		if got := UpstreamStatus(c.err); got != c.want {
			t.Errorf("%s: UpstreamStatus = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestUpstreamRetryAfter(t *testing.T) {
	d, ok := UpstreamRetryAfter(retryHTTPError(t, "openai", 429, map[string]string{"Retry-After": "7"}))
	if !ok || d != 7*time.Second {
		t.Fatalf("UpstreamRetryAfter = %v, %v; want 7s", d, ok)
	}
	if d, ok := UpstreamRetryAfter(&devinAPIError{status: 429, retryAfter: 7 * time.Second}); !ok || d != 7*time.Second {
		t.Fatalf("Devin Retry-After = %v, %v", d, ok)
	}
	reset := &QuotaResetError{Delay: 90 * time.Second, Cause: errors.New("limit")}
	if d, ok := UpstreamRetryAfter(reset); !ok || d != 90*time.Second {
		t.Fatalf("UpstreamRetryAfter(quota reset) = %v, %v; want 90s", d, ok)
	}
	if _, ok := UpstreamRetryAfter(retryHTTPError(t, "openai", 400, nil)); ok {
		t.Fatal("a 400 without a hint named a pause")
	}
}

// TestIsTransientProviderError: what the agent may run again after a pause -
// a lane failure, whether or not text had already streamed - and what it
// must not: a request the provider refused, a cancel, a limit with a reset.
func TestIsTransientProviderError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"server error after text", fmt.Errorf("openai stream: %w", &streamServerError{code: 500, msg: "litellm.MidStreamFallbackError", emitted: true}), true},
		{"server error before text", fmt.Errorf("openai stream: %w", &streamServerError{code: 502}), true},
		{"truncated after text", fmt.Errorf("openai stream: %w", &streamTruncatedError{emitted: true}), true},
		{"truncated inside a JSON event after text", fmt.Errorf("codex stream: %w", &streamTruncatedError{emitted: true, cause: jsonCutError()}), true},
		{"truncated inside a JSON event before text", fmt.Errorf("codex stream: %w", &streamTruncatedError{cause: jsonCutError()}), true},
		{"JSON syntax error in a transport wrapper", fmt.Errorf("codex stream: %w", &streamTransportError{cause: jsonCutError(), emitted: true}), false},
		{"undecodable frame with a status in its text", fmt.Errorf("openai stream: %w", &streamUndecodableError{snippet: "<html><title>502 Bad Gateway</title></html>"}), false},
		{"connection reset after text", fmt.Errorf("openai stream: %w", &streamTransportError{cause: syscall.ECONNRESET, emitted: true}), true},
		{"Winsock connection reset after text", fmt.Errorf("openai stream: %w", &streamTransportError{cause: os.NewSyscallError("wsarecv", syscall.Errno(10054)), emitted: true}), true},
		{"stalled", &streamStalledError{idle: time.Minute}, true},
		{"upstream 503", retryHTTPError(t, "openai", 503, nil), true},
		{"upstream 429", retryHTTPError(t, "openai", 429, nil), false},
		{"limit in the stream", fmt.Errorf("openai stream: %w", &streamServerError{code: 429, emitted: true}), false},
		{"bad request", retryHTTPError(t, "openai", 400, nil), false},
		{"refusal after text", fmt.Errorf("openai stream: %w", &streamServerError{code: 400, emitted: true}), false},
		{"codex failure after text", fmt.Errorf("codex stream: %w", &streamServerError{msg: "content policy", emitted: true}), false},
		{"transport error that is not transient", fmt.Errorf("openai stream: %w", &streamTransportError{cause: errors.New("x509: certificate signed by unknown authority"), emitted: true}), false},
		{"cancelled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"quota reset", &QuotaResetError{Cause: retryHTTPError(t, "openai", 429, nil)}, false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := IsTransientProviderError(c.err); got != c.want {
			t.Errorf("%s: IsTransientProviderError = %v, want %v", c.name, got, c.want)
		}
	}
}
