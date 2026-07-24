//go:build http

package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestDesignPlansCRUD(t *testing.T) {
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

	createBody, _ := json.Marshal(map[string]string{"slug": "demo", "content": plans.DefaultContent("demo", "Demo")})
	req := httptest.NewRequest(http.MethodPost, "/foxxycode/sessions/"+id+"/plans", bytes.NewReader(createBody))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/foxxycode/sessions/"+id+"/plans", nil)
	req.SetPathValue("id", id)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/foxxycode/sessions/"+id+"/plans/demo", nil)
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "demo")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}

	putBody, _ := json.Marshal(map[string]string{"body": "# Updated\n\nOnly body changed."})
	req = httptest.NewRequest(http.MethodPut, "/foxxycode/sessions/"+id+"/plans/demo", bytes.NewReader(putBody))
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "demo")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put body: %d %s", rec.Code, rec.Body.String())
	}
	got, err := plans.Read(filepath.Join(root, "sessions", id), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "# Updated\n\nOnly body changed." {
		t.Fatalf("body: %q", got.Body)
	}
	if got.Name != "Demo" {
		t.Fatalf("name should be preserved: %q", got.Name)
	}

	req = httptest.NewRequest(http.MethodDelete, "/foxxycode/sessions/"+id+"/plans/demo", nil)
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "demo")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestDesignPlanPutBodyBootstrapFromTranscript(t *testing.T) {
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
	st := mgr.SessionByID(id)
	if st == nil {
		t.Fatal("session missing")
	}
	bootstrap := plans.DefaultContent("orphan-plan", "Orphan plan")
	st.AppendPlanDocument(plans.Document{
		Slug:    "orphan-plan",
		Name:    "Orphan plan",
		Content: bootstrap,
		Body:    "# Draft\n",
	})

	putBody, _ := json.Marshal(map[string]string{
		"body":    "# Edited in markdown\n",
		"content": bootstrap,
	})
	req := httptest.NewRequest(http.MethodPut, "/foxxycode/sessions/"+id+"/plans/orphan-plan", bytes.NewReader(putBody))
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "orphan-plan")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put body bootstrap: %d %s", rec.Code, rec.Body.String())
	}
	sd := filepath.Join(root, "sessions", id)
	got, err := plans.Read(sd, "orphan-plan")
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "# Edited in markdown" {
		t.Fatalf("body: %q", got.Body)
	}
	msgs := mgr.SessionByID(id).GetMessages()
	var found bool
	for _, m := range msgs {
		if m.PlanDocument == nil || m.PlanDocument.Slug != "orphan-plan" {
			continue
		}
		found = true
		if m.PlanDocument.Body != "# Edited in markdown" {
			t.Fatalf("transcript body: %q", m.PlanDocument.Body)
		}
		if !strings.Contains(m.PlanDocument.Content, "name: Orphan plan") {
			t.Fatal("transcript content not updated")
		}
	}
	if !found {
		t.Fatal("plan_document row missing")
	}
}

// "Show in IDE" on the plan card: the SPA posts the slug and the server resolves
// the plan file path itself, then pushes an open_file event to the connected
// IntelliJ / VS Code plugin over the shared /foxxycode/ide/events hub.
func TestDesignPlanOpenInIDE(t *testing.T) {
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

	createBody, _ := json.Marshal(map[string]string{"slug": "demo", "content": plans.DefaultContent("demo", "Demo")})
	req := httptest.NewRequest(http.MethodPost, "/foxxycode/sessions/"+id+"/plans", bytes.NewReader(createBody))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	// A subscribed plugin must receive the event.
	sub := ideEvents.subscribe()
	defer ideEvents.unsubscribe(sub)

	req = httptest.NewRequest(http.MethodPost, "/foxxycode/sessions/"+id+"/plans/demo/open-in-ide", nil)
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "demo")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("open-in-ide: %d %s", rec.Code, rec.Body.String())
	}
	var parsed struct {
		Object    string `json:"object"`
		Path      string `json:"path"`
		Delivered bool   `json:"delivered"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	wantPath, err := plans.FilePath(filepath.Join(root, "sessions", id), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Object != "foxxycode.ide_open_file" {
		t.Fatalf("object: %q", parsed.Object)
	}
	if parsed.Path != wantPath {
		t.Fatalf("path: %q want %q", parsed.Path, wantPath)
	}
	if !parsed.Delivered {
		t.Fatal("delivered must be true while an IDE client is subscribed")
	}

	select {
	case ev := <-sub:
		if ev.Type != "open_file" {
			t.Fatalf("event type: %q", ev.Type)
		}
		if ev.Path != wantPath {
			t.Fatalf("event path: %q want %q", ev.Path, wantPath)
		}
		if ev.SessionID != id {
			t.Fatalf("event sessionId: %q want %q", ev.SessionID, id)
		}
	default:
		t.Fatal("no open_file event was broadcast")
	}

	// An unknown slug must not reach the IDE at all.
	req = httptest.NewRequest(http.MethodPost, "/foxxycode/sessions/"+id+"/plans/missing/open-in-ide", nil)
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "missing")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing plan: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case ev := <-sub:
		t.Fatalf("unexpected event for a missing plan: %+v", ev)
	default:
	}
}
