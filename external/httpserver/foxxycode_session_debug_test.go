//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestSessionDebugEndpoint(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "p1", Type: "openai", APIKey: "k"}},
		Models:    []config.ModelEntry{{Model: "p1/gpt-4o"}},
		Agent:     config.Agent{Model: "p1/gpt-4o"},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), t.TempDir(), store)
	srv := New(cfg, mgr, slog.Default(), t.TempDir())

	newRes, err := mgr.HandleSessionNew(t.Context(), acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := newRes.SessionID

	// Before any trace is written, events is null.
	rec := getDebug(t, srv, id)
	var body struct {
		Object   string            `json:"object"`
		Events   []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode empty trace: %v: %s", err, rec.Body.String())
	}
	if body.Object != "foxxycode.session_debug" || body.Events != nil {
		t.Fatalf("empty trace: object=%q events=%v", body.Object, body.Events)
	}

	// Write a trace directly into the session bundle, then read it back.
	dir := filepath.Join(store.Root, id)
	trace := `{"turn":0,"phase":"turn_start","title":"agent","at":"2038-01-19T03:14:07Z","meta":{"tools":12}}` + "\n" +
		`{"turn":0,"phase":"llm_response","at":"2038-01-19T03:14:08Z","meta":{"stop_reason":"end_turn","output_tokens":7}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "debug_trace.jsonl"), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}

	rec = getDebug(t, srv, id)
	var body2 struct {
		Object string                   `json:"object"`
		Events []map[string]interface{} `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body2); err != nil {
		t.Fatalf("decode trace: %v: %s", err, rec.Body.String())
	}
	if body2.Object != "foxxycode.session_debug" || len(body2.Events) != 2 {
		t.Fatalf("trace: object=%q events=%d", body2.Object, len(body2.Events))
	}
	if body2.Events[0]["phase"] != "turn_start" || body2.Events[1]["phase"] != "llm_response" {
		t.Errorf("unexpected events order: %+v", body2.Events)
	}
}

func getDebug(t *testing.T, srv *Server, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/foxxycode/sessions/"+id+"/debug", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /debug: %d %s", rec.Code, rec.Body.String())
	}
	return rec
}
