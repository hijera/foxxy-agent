//go:build http

package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// newRewindUndoServer serves a session with two stored turns behind a runner
// that records the prompt and then waits until the turn is cancelled, so a
// test can undo while the edited turn is still running.
func newRewindUndoServer(t *testing.T) (*httptest.Server, *session.Manager, string, chan struct{}) {
	t.Helper()
	root := t.TempDir()
	started := make(chan struct{}, 4)
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		var text string
		for _, b := range prompt {
			text += b.Text
		}
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: text})
		started <- struct{}{}
		<-ctx.Done()
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "partial"})
		return string(acp.StopReasonCancelled), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: filepath.Join(root, "home"), CWD: root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, store)
	srv := New(cfg, mgr, slog.Default(), root)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		srv.Drain()
	})
	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: root})
	if err != nil {
		t.Fatalf("session new: %v", err)
	}
	// A turn a failing test left running would keep the server from closing.
	t.Cleanup(func() { mgr.HandleSessionCancel(acp.SessionCancelParams{SessionID: res.SessionID}) })
	st := mgr.SessionByID(res.SessionID)
	for i := 0; i < 2; i++ {
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("ask %d", i)})
		st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("answer %d", i)})
	}
	if err := store.Save(st); err != nil {
		t.Fatalf("save: %v", err)
	}
	return ts, mgr, res.SessionID, started
}

func postJSON(t *testing.T, url string, payload interface{}) (int, map[string]interface{}) {
	t.Helper()
	var buf []byte
	if payload != nil {
		buf, _ = json.Marshal(payload)
	}
	res, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	var body map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body
}

func startBlockedRewindPrompt(t *testing.T, ts *httptest.Server, sid, text string, started <-chan struct{}) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf, _ := json.Marshal(map[string]interface{}{"model": "agent", "input": text, "stream": false})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/responses", bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-FoxxyCode-Session-ID", sid)
		if res, err := http.DefaultClient.Do(req); err == nil {
			_ = res.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not start")
	}
	return done
}

func TestRewindUndoStopsTheEditedTurn(t *testing.T) {
	ts, mgr, sid, started := newRewindUndoServer(t)
	path := ts.URL + "/foxxycode/sessions/" + sid
	if status, body := postJSON(t, path+"/rewind", map[string]int{"userMessageIndex": 1}); status != http.StatusOK {
		t.Fatalf("rewind: %d %v", status, body)
	}
	id := undoSnapshotID(t, ts.Client(), path+"/messages")
	done := startBlockedRewindPrompt(t, ts, sid, "edited", started)
	status, body := postJSON(t, path+"/rewind/undo", map[string]string{"snapshotId": id})
	if status != http.StatusOK {
		t.Fatalf("undo: %d %v", status, body)
	}
	if body["object"] != "foxxycode.session_rewind_undone" || body["sessionId"] != sid {
		t.Fatalf("unexpected undo body: %v", body)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the edited turn kept running after the undo")
	}
	msgs := mgr.SessionByID(sid).GetMessages()
	want := []string{"ask 0", "answer 0", "ask 1", "answer 1"}
	if len(msgs) != len(want) {
		t.Fatalf("expected the original history, got %+v", msgs)
	}
	for i, m := range msgs {
		if m.Content != want[i] {
			t.Fatalf("history[%d] = %q, want %q (all: %+v)", i, m.Content, want[i], msgs)
		}
	}
}

func TestRewindUndoWithNothingToUndo(t *testing.T) {
	ts, _, sid, _ := newRewindUndoServer(t)
	status, body := postJSON(t, ts.URL+"/foxxycode/sessions/"+sid+"/rewind/undo", map[string]string{"snapshotId": "expired"})
	if status != http.StatusConflict {
		t.Fatalf("undo without a rewind: want 409, got %d %v", status, body)
	}
}

func TestRewindUndoRejectsBadSessionID(t *testing.T) {
	ts, _, _, _ := newRewindUndoServer(t)
	status, _ := postJSON(t, ts.URL+"/foxxycode/sessions/..%2Fetc/rewind/undo", map[string]string{"snapshotId": "expired"})
	if status != http.StatusBadRequest && status != http.StatusNotFound {
		t.Fatalf("want 400 or 404 for a traversal id, got %d", status)
	}
}

func undoSnapshotID(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	res, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var body struct {
		RewindUndo struct {
			SnapshotID string `json:"snapshotId"`
		} `json:"rewindUndo"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("messages: status %d, decode error %v", res.StatusCode, err)
	}
	if body.RewindUndo.SnapshotID == "" {
		t.Fatal("missing rewind undo snapshot ID")
	}
	return body.RewindUndo.SnapshotID
}

func TestRewindUndoAcrossAlreadyLoadedBackends(t *testing.T) {
	first, manager, sid, _ := newRewindUndoServer(t)
	cwd := manager.SessionByID(sid).GetCWD()
	second := newBackendWindow(t, cwd, manager.FileStore().Root)
	if _, err := second.mgr.LoadPersistedSession(context.Background(), sid, cwd); err != nil {
		t.Fatal(err)
	}
	path := "/foxxycode/sessions/" + sid
	if status, body := postJSON(t, first.URL+path+"/rewind", map[string]int{"userMessageIndex": 0}); status != http.StatusOK {
		t.Fatalf("rewind: %d %v", status, body)
	}
	snapshotID := undoSnapshotID(t, second.ts.Client(), second.ts.URL+path+"/messages")
	if status, body := postJSON(t, second.ts.URL+path+"/rewind/undo", map[string]string{"snapshotId": snapshotID}); status != http.StatusOK {
		t.Fatalf("undo through second backend: %d %v", status, body)
	}
	if got := second.mgr.SessionByID(sid).GetMessages(); len(got) != 4 || got[2].Content != "ask 1" {
		t.Fatalf("expected the original history, got %v", got)
	}
}

func TestRewindUndoRejectsSupersededSnapshot(t *testing.T) {
	ts, _, sid, _ := newRewindUndoServer(t)
	path := ts.URL + "/foxxycode/sessions/" + sid
	if status, body := postJSON(t, path+"/rewind", map[string]int{"userMessageIndex": 1}); status != http.StatusOK {
		t.Fatalf("first rewind: %d %v", status, body)
	}
	oldID := undoSnapshotID(t, ts.Client(), path+"/messages")
	if status, body := postJSON(t, path+"/rewind", map[string]int{"userMessageIndex": 0}); status != http.StatusOK {
		t.Fatalf("second rewind: %d %v", status, body)
	}
	if status, body := postJSON(t, path+"/rewind/undo", map[string]string{"snapshotId": oldID}); status != http.StatusConflict {
		t.Fatalf("undo of superseded snapshot: %d %v", status, body)
	}
	if newID := undoSnapshotID(t, ts.Client(), path+"/messages"); newID == oldID {
		t.Fatal("rewind reused its predecessor's identity")
	}
}

func TestRewindUndoDoesNotCancelFollowUpTurn(t *testing.T) {
	ts, mgr, sid, started := newRewindUndoServer(t)
	path := ts.URL + "/foxxycode/sessions/" + sid
	if status, body := postJSON(t, path+"/rewind", map[string]int{"userMessageIndex": 1}); status != http.StatusOK {
		t.Fatalf("rewind: %d %v", status, body)
	}
	id := undoSnapshotID(t, ts.Client(), path+"/messages")
	done := startBlockedRewindPrompt(t, ts, sid, "edited", started)
	mgr.HandleSessionCancel(acp.SessionCancelParams{SessionID: sid})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the edited turn did not stop")
	}
	followUp := startBlockedRewindPrompt(t, ts, sid, "follow-up", started)
	if status, body := postJSON(t, path+"/rewind/undo", map[string]string{"snapshotId": id}); status != http.StatusConflict {
		t.Fatalf("undo after a follow-up: %d %v", status, body)
	}
	select {
	case <-followUp:
		t.Fatal("a refused undo cancelled the follow-up")
	default:
	}
}

func TestRewindUndoRequiresJSONContentType(t *testing.T) {
	ts, _, sid, _ := newRewindUndoServer(t)
	res, err := ts.Client().Post(ts.URL+"/foxxycode/sessions/"+sid+"/rewind/undo", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", res.StatusCode)
	}
}

func TestRewindUndoCancelsEditedTurnOnAnotherBackend(t *testing.T) {
	first, manager, sid, started := newRewindUndoServer(t)
	cwd := manager.SessionByID(sid).GetCWD()
	second := newBackendWindow(t, cwd, manager.FileStore().Root)
	if _, err := second.mgr.LoadPersistedSession(context.Background(), sid, cwd); err != nil {
		t.Fatal(err)
	}
	path := "/foxxycode/sessions/" + sid
	if status, body := postJSON(t, first.URL+path+"/rewind", map[string]int{"userMessageIndex": 1}); status != http.StatusOK {
		t.Fatalf("rewind: %d %v", status, body)
	}
	snapshotID := undoSnapshotID(t, second.ts.Client(), second.ts.URL+path+"/messages")
	done := startBlockedRewindPrompt(t, first, sid, "edited", started)
	status, body := postJSON(t, second.ts.URL+path+"/rewind/undo", map[string]string{"snapshotId": snapshotID})
	if status != http.StatusOK {
		t.Fatalf("undo through second backend: %d %v", status, body)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the edited turn on the first backend did not stop")
	}
	if got := second.mgr.SessionByID(sid).GetMessages(); len(got) != 4 || got[2].Content != "ask 1" {
		t.Fatalf("expected the original history, got %v", got)
	}
}
