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
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/plans"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
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
	req := httptest.NewRequest(http.MethodPost, "/coddy/sessions/"+id+"/plans", bytes.NewReader(createBody))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/coddy/sessions/"+id+"/plans", nil)
	req.SetPathValue("id", id)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/coddy/sessions/"+id+"/plans/demo", nil)
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "demo")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}

	putBody, _ := json.Marshal(map[string]string{"body": "# Updated\n\nOnly body changed."})
	req = httptest.NewRequest(http.MethodPut, "/coddy/sessions/"+id+"/plans/demo", bytes.NewReader(putBody))
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

	req = httptest.NewRequest(http.MethodDelete, "/coddy/sessions/"+id+"/plans/demo", nil)
	req.SetPathValue("id", id)
	req.SetPathValue("slug", "demo")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
}
