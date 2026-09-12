package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// captureTrace redirects the package trace writer for one test. It lives in the
// test file so the production path keeps its allocation-free lazy resolver;
// consuming traceOnce here stops that resolver from replacing the buffer.
func captureTrace(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	traceMu.Lock()
	prev := traceOut
	traceOut = buf
	traceOnce.Do(func() {})
	traceMu.Unlock()
	t.Cleanup(func() {
		traceMu.Lock()
		traceOut = prev
		traceMu.Unlock()
	})
	return buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestStreamEndVerdictNamesEachSilentEnding(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		done       bool
		stopReason string
		want       string
	}{
		{"completed answer", nil, true, "end_turn", "complete"},
		{"cut off at max_tokens", nil, true, "max_tokens", "truncated-by-max-tokens"},
		{"abandoned before any terminal marker", nil, false, "", "cut-before-terminal-marker"},
		{"finish_reason but no [DONE]", nil, false, "end_turn", "finish-reason-without-done"},
		{"[DONE] but no finish_reason", nil, true, "", "done-without-finish-reason"},
		{"transport failure", io.ErrUnexpectedEOF, false, "", "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := streamEndVerdict(tc.err, tc.done, tc.stopReason); got != tc.want {
				t.Errorf("streamEndVerdict = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOpenTraceWriterResolvesDestination(t *testing.T) {
	if w := openTraceWriter(""); w != nil {
		t.Error("an unset variable must leave tracing off")
	}
	if w := openTraceWriter("  "); w != nil {
		t.Error("a blank variable must leave tracing off")
	}
	for _, raw := range []string{"1", "true", "YES", "on", "stderr"} {
		if w := openTraceWriter(raw); w == nil {
			t.Errorf("%q must enable tracing", raw)
		}
	}
	path := t.TempDir() + "/trace.log"
	w := openTraceWriter(path)
	if w == nil {
		t.Fatal("a path must enable tracing")
	}
	if c, ok := w.(io.Closer); ok {
		_ = c.Close()
	}
}

// streamTraceServer replays a scripted SSE body, so the trace can be asserted
// against a stream whose ending is known exactly.
func streamTraceServer(t *testing.T, body string) *openAIProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return newOpenAIProvider("kimi-k2.6", "test-key", srv.URL, srv.Client(), 8192, 0.2, "")
}

func TestStreamTraceRecordsMaxTokensTruncation(t *testing.T) {
	buf := captureTrace(t)

	frame := func(delta, finish string) string {
		fr := "null"
		if finish != "" {
			fr = fmt.Sprintf("%q", finish)
		}
		return fmt.Sprintf(
			`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":%q},"finish_reason":%s}]}`+"\n\n",
			delta, fr)
	}
	body := frame("The plan ", "") + frame("begins with", "length") + "data: [DONE]\n\n"

	p := streamTraceServer(t, body)
	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if resp.StopReason != "max_tokens" {
		t.Fatalf("stop reason = %q, want max_tokens", resp.StopReason)
	}

	got := buf.String()
	for _, want := range []string{
		"llm-trace",
		"model=kimi-k2.6",
		"max_tokens=8192",
		`finish_reason="length"`,
		"verdict=truncated-by-max-tokens",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("trace missing %q\n--- trace ---\n%s", want, got)
		}
	}
}

// One tool call can span hundreds of argument frames. Only the frame that names
// the call is worth a trace line; logging each argument chunk buries the decisive
// ones - a real turn produced 1105 frame lines out of 1143 before this rule.
// (That these frames reach the caller as Progress at all is pinned separately in
// stream_progress_test.go.)
func TestStreamTraceLogsOneLinePerToolCallNotPerArgumentFrame(t *testing.T) {
	buf := captureTrace(t)

	first := `{"index":0,"id":"call_1","function":{"name":"list_dir","arguments":"{\"path\":"}}`
	rest := `{"index":0,"function":{"arguments":"\".\"}"}}`
	frame := func(tc string) string {
		return `data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[` +
			tc + `]}}]}` + "\n\n"
	}
	body := frame(first) + frame(rest) +
		`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` +
		"\n\n" + "data: [DONE]\n\n"

	p := streamTraceServer(t, body)
	resp, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].InputJSON != `{"path":"."}` {
		t.Fatalf("tool call was not assembled correctly: %+v", resp.ToolCalls)
	}

	trace := buf.String()
	if n := strings.Count(trace, " frame "); n > 2 {
		t.Errorf("trace logged %d frame lines for one tool call:\n%s", n, trace)
	}
	if !strings.Contains(trace, `tool_call="list_dir"`) {
		t.Errorf("trace does not name the tool call:\n%s", trace)
	}
}

func TestStreamTraceStaysSilentWhenDisabled(t *testing.T) {
	buf := captureTrace(t)
	traceMu.Lock()
	traceOut = nil
	traceMu.Unlock()

	p := streamTraceServer(t, `data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	if _, err := p.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, func(StreamChunk) {}); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if buf.String() != "" {
		t.Errorf("tracing is off, yet something was written: %q", buf.String())
	}
}
