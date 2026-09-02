package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendAndReadDebugTrace(t *testing.T) {
	dir := t.TempDir()

	evs := []DebugEvent{
		{Turn: 0, Phase: "turn_start", Title: "agent", At: "2038-01-19T03:14:07Z", Meta: map[string]interface{}{"tools": 12}},
		{Turn: 0, Phase: "llm_response", Detail: "ok", At: "2038-01-19T03:14:10Z", Meta: map[string]interface{}{"out": 42}},
		{Turn: 1, Phase: "tool_start", Title: "read", At: "2038-01-19T03:14:11Z"},
	}
	for _, ev := range evs {
		if _, err := AppendDebugEvent(dir, ev); err != nil {
			t.Fatalf("AppendDebugEvent: %v", err)
		}
	}

	got, err := ReadDebugTrace(dir)
	if err != nil {
		t.Fatalf("ReadDebugTrace: %v", err)
	}
	if len(got) != len(evs) {
		t.Fatalf("got %d events, want %d", len(got), len(evs))
	}
	if got[0].Phase != "turn_start" || got[0].Meta["tools"] != float64(12) {
		t.Errorf("first event mismatch: %+v", got[0])
	}
	if got[2].Title != "read" {
		t.Errorf("third event title = %q, want read", got[2].Title)
	}
}

func TestReadDebugTraceMissingFileIsEmpty(t *testing.T) {
	got, err := ReadDebugTrace(t.TempDir())
	if err != nil {
		t.Fatalf("ReadDebugTrace on empty dir: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil slice for missing file, got %v", got)
	}
}

func TestAppendDebugTraceEmptyDirErrors(t *testing.T) {
	if _, err := AppendDebugEvent("", DebugEvent{}); err == nil {
		t.Error("expected error on empty session dir")
	}
}

func TestReadDebugTraceSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	// Write one good line and one malformed line directly to the JSONL file.
	path := filepath.Join(dir, debugTraceFileName)
	good := `{"turn":0,"phase":"turn_start","at":"2038-01-19T03:14:07Z"}` + "\n"
	bad := `{"turn":1,"phase":` + "\n" // truncated JSON
	if err := os.WriteFile(path, []byte(good+bad+good), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDebugTrace(dir)
	if err != nil {
		t.Fatalf("ReadDebugTrace: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 valid events (malformed skipped), got %d", len(got))
	}
}
