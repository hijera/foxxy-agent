//go:build http && (unix || windows)

package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func postQueue(t *testing.T, ts *httptest.Server, id, text string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/foxxycode/sessions/"+id+"/queue", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FoxxyCode-Session-ID", id)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&payload)
	return res.StatusCode, payload.Error.Code
}

// Two IDE windows are two backends over one home, and the turn runs in only one of
// them. The other has no queue for that turn - it lives in the process that runs
// it - and the prompt a client falls back to on no_active_turn would be refused as
// busy after waiting for the lock. So this server says busy straight away, and
// the client keeps what the operator wrote.
func TestQueuePostIsBusyWhileAnotherBackendRunsTheTurn(t *testing.T) {
	mgr, srv, sessRoot := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	created, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	id := created.SessionID

	// The second backend: its own manager over the same sessions root.
	cfg := &config.Config{
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	other := session.NewManager(cfg, noopSender{}, nil, slog.Default(), "/tmp", &session.FileStore{Root: sessRoot})
	st, err := other.EnsureHTTPSession(context.Background(), id, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	held, err := other.AcquireComposerTurnLock(id, st)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	unlock := func() { once.Do(held) }
	// Registered after the temp dirs, so it runs before they are removed: Windows
	// refuses to delete a lock file somebody still holds.
	t.Cleanup(unlock)

	status, code := postQueue(t, ts, id, "also check the Windows path")
	if status != http.StatusConflict || code != "session_busy" {
		t.Fatalf("queue while another backend runs the turn = %d %q, want 409 session_busy", status, code)
	}

	unlock()
	status, code = postQueue(t, ts, id, "also check the Windows path")
	if status != http.StatusConflict || code != "no_active_turn" {
		t.Fatalf("queue with no turn anywhere = %d %q, want 409 no_active_turn", status, code)
	}
}

// A client re-attaching to a running turn trims the transcript back to the prompt
// the turn started from and lets the relay replay the rest. Follow-ups the turn
// read are in the transcript too, so the rows say which user messages they are.
func TestMessagesMarkTheFollowUpsATurnRead(t *testing.T) {
	mgr, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	created, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(created.SessionID)
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "start the work"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "working"})
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "check the tests too", Queued: true})

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/foxxycode/sessions/"+created.SessionID+"/messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-FoxxyCode-Session-ID", created.SessionID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var body struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 3 {
		t.Fatalf("got %d rows", len(body.Messages))
	}
	if _, marked := body.Messages[0]["queued"]; marked {
		t.Errorf("the prompt row is marked queued: %v", body.Messages[0])
	}
	if body.Messages[2]["queued"] != true {
		t.Errorf("the follow-up row is not marked queued: %v", body.Messages[2])
	}
}
