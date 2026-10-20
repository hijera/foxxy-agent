package llm

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A peer that resets the connection after part of an answer went out is a
// transient failure on every platform, so the agent carries the answer on.
// Linux reports ECONNRESET ("connection reset by peer"); Windows reports
// WSAECONNRESET ("An existing connection was forcibly closed by the remote
// host"), which syscall.ECONNRESET does not match there. Found live in the
// IDE: on Windows the turn ended with an LLM error and lost the text.
func TestConnectionResetMidAnswerIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		frame := `data: {"id":"c1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"half an ans"}}]}` + "\n\n"
		_, _ = fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", len(frame), frame)
		_ = rw.Flush()
		time.Sleep(100 * time.Millisecond)
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetLinger(0) // close with RST, not FIN
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	p, err := NewProvider(ProviderInput{
		Type:          "openai",
		Model:         "test-model",
		BaseURL:       srv.URL,
		RetryMax:      1,
		RetryBase:     time.Millisecond,
		RetryMaxDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var got strings.Builder
	_, err = p.Stream(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil, func(c StreamChunk) {
		got.WriteString(c.TextDelta)
	})
	if err == nil {
		t.Fatal("the stream succeeded, want the reset")
	}
	if got.String() != "half an ans" {
		t.Fatalf("delivered %q before the reset, want the first frame", got.String())
	}
	if !IsTransientProviderError(err) {
		t.Fatalf("a connection reset mid-answer is not transient: %v", err)
	}
}
