//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func writeInWorkspace(t *testing.T, cwd, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cwd, rel), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func changedPaths(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	files, _ := body["files"].([]interface{})
	var out []string
	for _, f := range files {
		row, _ := f.(map[string]interface{})
		p, _ := row["path"].(string)
		out = append(out, filepath.ToSlash(p))
	}
	return out
}

// waitForSessionChanges reads frames until the one that says the session's
// change set settled, and fails the test if none arrives.
func waitForSessionChanges(t *testing.T, frames <-chan []byte, sessionID string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case f := <-frames:
			s := string(f)
			if strings.HasPrefix(s, "event: session_changes\n") && strings.Contains(s, `"sessionId":"`+sessionID+`"`) {
				return
			}
		case <-deadline:
			t.Fatal("no session_changes event for the session")
		}
	}
}

func TestSessionChangesFrameShape(t *testing.T) {
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	frame := string(sessionChangesFrame("sess_x", at))
	if !strings.HasPrefix(frame, "event: session_changes\ndata: ") || !strings.HasSuffix(frame, "\n\n") {
		t.Fatalf("frame %q", frame)
	}
	var body map[string]interface{}
	data := strings.TrimSuffix(strings.TrimPrefix(frame, "event: session_changes\ndata: "), "\n\n")
	if err := json.Unmarshal([]byte(data), &body); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	if body["object"] != "foxxycode.session_changes" || body["sessionId"] != "sess_x" || body["at"] != "2026-09-27T12:00:00Z" {
		t.Fatalf("body %v", body)
	}
}

// Opened while a turn runs, the card lists the finished turns and what the
// running one has already written; the last-turn scope is the running turn.
func TestSessionChangesIncludeARunningTurn(t *testing.T) {
	e := newChangesEnv(t)
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "earlier.txt", After: wsFile("x\n")})
	live := e.srv.beginLiveTurn(e.id, e.cwd, session.TakeWorkspaceSnapshot(e.cwd))
	defer e.srv.endLiveTurn(e.id, live)
	writeInWorkspace(t, e.cwd, "fresh.txt", "new\n")

	got := changedPaths(t, e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes"))
	if strings.Join(got, ",") != "earlier.txt,fresh.txt" {
		t.Fatalf("session scope %v", got)
	}
	got = changedPaths(t, e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes?scope=turn"))
	if strings.Join(got, ",") != "fresh.txt" {
		t.Fatalf("turn scope %v", got)
	}
	rec := e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes/file?path=fresh.txt")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "+new") {
		t.Fatalf("detail %d: %s", rec.Code, rec.Body.String())
	}
}

// The event is what tells the card a finished turn can be read, so it must
// come after the diff is on disk - and the live entry is retired by then, or
// the card would keep comparing against a snapshot that is over.
func TestSessionChangesAnnouncedAfterTheTurnIsStored(t *testing.T) {
	e := newChangesEnv(t)
	frames, unsubscribe := e.srv.events.subscribe()
	defer unsubscribe()
	st := e.srv.mgr.SessionByID(e.id)
	if st == nil {
		t.Fatal("session not live")
	}
	before := session.TakeWorkspaceSnapshot(e.cwd)
	live := e.srv.beginLiveTurn(e.id, e.cwd, before)
	writeInWorkspace(t, e.cwd, "fresh.txt", "new\n")
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edit"})

	e.srv.captureAndStoreTurnDiff(st, before, live)
	waitForSessionChanges(t, frames, e.id)

	turns, err := session.ListStoredTurnDiffs(e.sessionDir)
	if err != nil || len(turns) == 0 {
		t.Fatalf("the event came before the diff was stored: %v %v", turns, err)
	}
	if _, running := e.srv.liveTurnDiff(e.id); running {
		t.Fatal("the live entry outlived the stored diff")
	}
	got := changedPaths(t, e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes"))
	if strings.Join(got, ",") != "fresh.txt" {
		t.Fatalf("after the turn %v", got)
	}
}

// A turn that wrote files and then failed - the provider dropped the answer -
// still changed the workspace; the card and the rollback have to know.
func TestSessionChangesKeepWhatAFailedTurnWrote(t *testing.T) {
	runner := func(_ context.Context, st *session.State, _ []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edit it"})
		if err := os.WriteFile(filepath.Join(st.GetCWD(), "half.txt"), []byte("written\n"), 0o644); err != nil {
			return "", err
		}
		return "", errors.New("provider went away")
	}
	e := newChangesEnvWithRunner(t, runner)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"model":"agent","input":"edit it","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", e.id)
	rec := httptest.NewRecorder()
	e.srv.mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("the failing turn answered 200: %s", rec.Body.String())
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		turns, err := session.ListStoredTurnDiffs(e.sessionDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the failed turn's diff was never stored")
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := changedPaths(t, e.do(t, http.MethodGet, "/foxxycode/sessions/"+e.id+"/changes"))
	if strings.Join(got, ",") != "half.txt" {
		t.Fatalf("changes %v", got)
	}
}

// A turn that never reached the agent - the session was busy - did nothing, so
// nothing is stored for it: a diff taken now would file whatever else moved in
// the folder under the previous turn's number. The card still hears that the
// set is settled, or it would wait for an event that is not coming.
func TestSessionChangesSkipATurnThatNeverRan(t *testing.T) {
	e := newChangesEnv(t)
	frames, unsubscribe := e.srv.events.subscribe()
	defer unsubscribe()
	st := e.srv.mgr.SessionByID(e.id)
	before := session.TakeWorkspaceSnapshot(e.cwd)
	live := e.srv.beginLiveTurn(e.id, e.cwd, before)
	turns := session.TurnNumber(st.GetMessages())
	writeInWorkspace(t, e.cwd, "stray.txt", "someone else\n")

	e.srv.settleTurnDiff(st, before, live, turns, session.ErrSessionTurnBusy)
	waitForSessionChanges(t, frames, e.id)

	stored, err := session.ListStoredTurnDiffs(e.sessionDir)
	if err != nil || len(stored) != 0 {
		t.Fatalf("a turn that never ran stored %v (%v)", stored, err)
	}
	if _, running := e.srv.liveTurnDiff(e.id); running {
		t.Fatal("the live entry of a turn that never ran was kept")
	}
}

// Another window showing the same session learns about a rollback too.
func TestSessionChangesRevertAnnouncesTheChange(t *testing.T) {
	e := newChangesEnv(t)
	frames, unsubscribe := e.srv.events.subscribe()
	defer unsubscribe()
	writeInWorkspace(t, e.cwd, "a.txt", "new\n")
	e.storeTurn(t, 1, session.WorkspaceChange{Path: "a.txt", Before: wsFile("old\n"), After: wsFile("new\n")})

	rec := e.do(t, http.MethodPost, "/foxxycode/sessions/"+e.id+"/changes/revert")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	waitForSessionChanges(t, frames, e.id)
}
