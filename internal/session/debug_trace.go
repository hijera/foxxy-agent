package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// debugTraceFileName is the JSONL append-only file holding a session's debug-trace
// events, one DebugEvent per line. Written only while the diagnostics layer is on
// (debug.enabled); surfaced read-only through GET /foxxycode/sessions/{id}/debug.
const debugTraceFileName = "debug_trace.jsonl"

// DebugEvent is one structured trace record emitted by the agent loop: turn
// boundaries, LLM request/response metadata, and tool start/finish. The raw LLM
// HTTP bodies are logged separately to the process log; this carries lightweight
// metadata suitable for a UI timeline and the debug endpoint.
type DebugEvent struct {
	Turn   int                    `json:"turn"`
	Phase  string                 `json:"phase"` // turn_start|llm_request|llm_response|tool_start|tool_finish
	Title  string                 `json:"title,omitempty"`
	Detail string                 `json:"detail,omitempty"`
	At     string                 `json:"at"` // RFC3339 UTC
	Meta   map[string]interface{} `json:"meta,omitempty"`
}

func debugTracePath(sessionDir string) (string, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return "", fmt.Errorf("session directory is empty")
	}
	return filepath.Join(sessionDir, debugTraceFileName), nil
}

// AppendDebugEvent appends one debug trace event to the session bundle as a JSONL line.
// It is a best-effort trace: a write error is returned but callers typically only log it,
// since tracing must never break an agent turn.
func AppendDebugEvent(sessionDir string, ev DebugEvent) (string, error) {
	path, err := debugTracePath(sessionDir)
	if err != nil {
		return "", err
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return path, err
	}
	line = append(line, '\n')
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return path, err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(line); err != nil {
		return path, err
	}
	return path, nil
}

// ReadDebugTrace reads the full debug trace for a session. A missing file is reported
// as an empty slice and no error (the endpoint treats it as "no events collected").
func ReadDebugTrace(sessionDir string) ([]DebugEvent, error) {
	path, err := debugTracePath(sessionDir)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []DebugEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var ev DebugEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			// Skip malformed lines rather than failing the whole read.
			continue
		}
		out = append(out, ev)
	}
	return out, sc.Err()
}
