package llm

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TraceEnvVar names the environment variable that turns frame-level streaming
// traces on. "1", "true", "yes", "on" and "stderr" write to standard error;
// any other non-empty value is taken as a file path, opened in append mode.
const TraceEnvVar = "FOXXYCODE_LLM_TRACE"

// Frame-level tracing of the OpenAI-compatible SSE path.
//
// A turn that ends early is indistinguishable from a completed one once the
// frames are gone: a gateway that stops generating mid-answer, a model that
// exhausts max_tokens, and a model that genuinely finished all arrive as the
// same "the stream ended" observation at the call site. The trace keeps the
// evidence - what the terminal frame said, how long the gaps between frames
// were, and how many tokens each channel actually carried - so an operator can
// tell those apart after the fact instead of guessing.

var (
	traceOnce sync.Once
	traceOut  io.Writer
	traceMu   sync.Mutex
)

// traceWriter resolves the trace destination once per process. A file that
// cannot be opened degrades to stderr rather than disabling the trace: the
// operator asked for it, so failing quietly would be the worst outcome.
func traceWriter() io.Writer {
	traceOnce.Do(func() { traceOut = openTraceWriter(os.Getenv(TraceEnvVar)) })
	traceMu.Lock()
	defer traceMu.Unlock()
	return traceOut
}

// openTraceWriter resolves the raw environment value to a destination; nil
// leaves tracing off.
func openTraceWriter(raw string) io.Writer {
	raw = strings.TrimSpace(raw)
	switch strings.ToLower(raw) {
	case "":
		return nil
	case "1", "true", "yes", "on", "stderr":
		return os.Stderr
	}
	f, err := os.OpenFile(raw, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm trace: cannot open %s: %v; tracing to stderr\n", raw, err)
		return os.Stderr
	}
	return f
}

// streamTrace records one streamed request. A nil *streamTrace is a working
// no-op, so the call sites carry no enabled/disabled branching of their own.
type streamTrace struct {
	w     io.Writer
	model string
	id    string
	start time.Time
	last  time.Time

	frames     int
	contentLen int
	reasonLen  int
	toolFrames int
	maxGap     time.Duration
	firstToken time.Duration
}

// newStreamTrace returns nil unless tracing is enabled, which is the common case.
func newStreamTrace(model string) *streamTrace {
	w := traceWriter()
	if w == nil {
		return nil
	}
	now := time.Now()
	return &streamTrace{
		w:     w,
		model: model,
		id:    strconv.FormatInt(now.UnixNano()%1e9, 36),
		start: now,
		last:  now,
	}
}

func (t *streamTrace) write(event string, extra string) {
	if t == nil {
		return
	}
	line := fmt.Sprintf("llm-trace %s model=%s t=%dms %s%s\n",
		t.id, t.model, time.Since(t.start).Milliseconds(), event, extra)
	traceMu.Lock()
	_, _ = io.WriteString(t.w, line)
	traceMu.Unlock()
}

// request records the outgoing call before any frame arrives, so a request that
// never produces one is still visible in the trace.
func (t *streamTrace) request(messages, tools int, maxTokens int, reasoningEffort string) {
	if t == nil {
		return
	}
	t.write("request", fmt.Sprintf(" messages=%d tools=%d max_tokens=%d reasoning_effort=%q",
		messages, tools, maxTokens, reasoningEffort))
}

// delta records a content/reasoning/tool frame. The gap since the previous
// frame is the part that matters for a stall: the payload sizes only say how
// much of the budget each channel consumed. newToolCall carries the function
// name when this frame starts a call, and is empty for the argument chunks
// that follow.
func (t *streamTrace) delta(content, reasoning, newToolCall, finishReason string, toolCallDeltas int) {
	if t == nil {
		return
	}
	now := time.Now()
	gap := now.Sub(t.last)
	t.last = now
	t.frames++
	if gap > t.maxGap {
		t.maxGap = gap
	}
	if t.firstToken == 0 && (content != "" || reasoning != "") {
		t.firstToken = now.Sub(t.start)
	}
	t.contentLen += len(content)
	t.reasonLen += len(reasoning)
	if toolCallDeltas > 0 {
		t.toolFrames++
	}
	// A frame earns a line only when it carries a decision: a finish_reason, the
	// start of a tool call, or a gap long enough to look like a stall. One tool
	// call can span hundreds of argument frames, and logging each would bury the
	// decisive lines - the end record already reports how many there were.
	if finishReason == "" && newToolCall == "" && gap < time.Second {
		return
	}
	t.write("frame", fmt.Sprintf(" n=%d gap=%dms content=%d reasoning=%d tool_call=%q finish_reason=%q",
		t.frames, gap.Milliseconds(), len(content), len(reasoning), newToolCall, finishReason))
}

// marker records a non-delta frame: the [DONE] terminator, a usage-only frame,
// or an in-band error object.
func (t *streamTrace) marker(kind string, detail string) {
	if t == nil {
		return
	}
	now := time.Now()
	gap := now.Sub(t.last)
	t.last = now
	t.frames++
	t.write("marker", fmt.Sprintf(" kind=%s gap=%dms detail=%s", kind, gap.Milliseconds(), traceSnippet(detail)))
}

// end records how the stream terminated. verdict is the classification the
// caller reached, which is exactly the fact a silent stop hides.
func (t *streamTrace) end(verdict string, stopReason string, done bool, err error, inputTokens, outputTokens int) {
	if t == nil {
		return
	}
	errText := "none"
	if err != nil {
		errText = traceSnippet(err.Error())
	}
	t.write("end", fmt.Sprintf(
		" verdict=%s stop_reason=%q done=%t frames=%d content=%d reasoning=%d tool_frames=%d"+
			" first_token=%dms max_gap=%dms in_tokens=%d out_tokens=%d error=%s",
		verdict, stopReason, done, t.frames, t.contentLen, t.reasonLen, t.toolFrames,
		t.firstToken.Milliseconds(), t.maxGap.Milliseconds(), inputTokens, outputTokens, errText))
}

// streamEndVerdict names the terminal condition of a stream in the operator's
// terms. The three silent endings - a completed answer, an answer cut off at
// max_tokens, and an answer the server abandoned after sending a terminal
// marker - reach the call site as the same successful return, so the verdict is
// the only place the difference is recorded.
func streamEndVerdict(streamErr error, done bool, stopReason string) string {
	switch {
	case streamErr != nil:
		return "error"
	case stopReason == "max_tokens":
		return "truncated-by-max-tokens"
	case !done && stopReason == "":
		return "cut-before-terminal-marker"
	case !done:
		return "finish-reason-without-done"
	case stopReason == "":
		return "done-without-finish-reason"
	default:
		return "complete"
	}
}

// traceSnippet keeps a trace line to one line of bounded length: raw payloads
// may be long and may contain newlines that would break line-oriented reading.
func traceSnippet(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	const max = 240
	if len(s) > max {
		s = s[:max] + "…"
	}
	return strconv.Quote(s)
}
