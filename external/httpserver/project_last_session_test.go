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
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/project"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type lastSessionDTO struct {
	Object    string `json:"object"`
	Path      string `json:"path"`
	SessionID string `json:"session_id"`
}

type lastSessionFixture struct {
	srv     *Server
	mgr     *session.Manager
	ts      *httptest.Server
	store   *session.FileStore
	project string
}

func newLastSessionServer(t *testing.T) *lastSessionFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sessRoot := filepath.Join(root, "sessions")
	proj := filepath.Join(root, "proj")
	for _, d := range []string{home, sessRoot, proj} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: proj},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "ok", nil
	}
	store := &session.FileStore{Root: sessRoot}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), proj, store)
	srv := New(cfg, mgr, slog.Default(), proj)
	t.Cleanup(srv.Drain)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &lastSessionFixture{srv: srv, mgr: mgr, ts: ts, store: store, project: proj}
}

// attachStoreAt wires a project store whose current project is dir.
func (f *lastSessionFixture) attachStoreAt(t *testing.T, home, dir string) *project.Store {
	t.Helper()
	ps, err := project.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.SetCurrent(dir); err != nil {
		t.Fatal(err)
	}
	f.srv.AttachProjectStore(ps)
	return ps
}

// persistSession creates a session rooted at cwd and writes it to disk.
func (f *lastSessionFixture) persistSession(t *testing.T, cwd string) string {
	t.Helper()
	sn, err := f.mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	st := f.mgr.SessionByID(sn.SessionID)
	if st == nil {
		t.Fatalf("session %s missing after create", sn.SessionID)
	}
	if err := f.store.Save(st); err != nil {
		t.Fatal(err)
	}
	if !f.store.HasPersistedSnapshot(sn.SessionID) {
		t.Fatalf("session %s not persisted", sn.SessionID)
	}
	return sn.SessionID
}

func (f *lastSessionFixture) getLastSession(t *testing.T) lastSessionDTO {
	t.Helper()
	res, err := http.Get(f.ts.URL + "/foxxycode/project/last-session")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET last-session status %d", res.StatusCode)
	}
	var dto lastSessionDTO
	if err := json.NewDecoder(res.Body).Decode(&dto); err != nil {
		t.Fatal(err)
	}
	return dto
}

func (f *lastSessionFixture) putLastSession(t *testing.T, id string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, f.ts.URL+"/foxxycode/project/last-session",
		strings.NewReader(`{"session_id":`+jsonQuote(id)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	return res.StatusCode
}

func TestProjectLastSessionEmptyByDefault(t *testing.T) {
	f := newLastSessionServer(t)
	f.attachStoreAt(t, t.TempDir(), f.project)

	dto := f.getLastSession(t)
	if dto.Object != "foxxycode.project_last_session" {
		t.Fatalf("object = %q", dto.Object)
	}
	if dto.Path != f.project {
		t.Fatalf("path = %q, want project %q", dto.Path, f.project)
	}
	if dto.SessionID != "" {
		t.Fatalf("session_id = %q, want empty", dto.SessionID)
	}
}

func TestProjectLastSessionRoundTrip(t *testing.T) {
	f := newLastSessionServer(t)
	f.attachStoreAt(t, t.TempDir(), f.project)
	sid := f.persistSession(t, f.project)

	if code := f.putLastSession(t, sid); code != http.StatusOK {
		t.Fatalf("PUT status %d, want 200", code)
	}
	if got := f.getLastSession(t).SessionID; got != sid {
		t.Fatalf("session_id = %q, want %q", got, sid)
	}
}

func TestProjectLastSessionAcceptsSessionInSubdirectory(t *testing.T) {
	f := newLastSessionServer(t)
	f.attachStoreAt(t, t.TempDir(), f.project)
	// A linked worktree lives under the project root and stays in scope,
	// matching the History cwd filter (session.CWDInScope).
	sub := filepath.Join(f.project, "wt", "feature")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	sid := f.persistSession(t, sub)

	if code := f.putLastSession(t, sid); code != http.StatusOK {
		t.Fatalf("PUT status %d, want 200", code)
	}
	if got := f.getLastSession(t).SessionID; got != sid {
		t.Fatalf("session_id = %q, want %q", got, sid)
	}
}

func TestProjectLastSessionDroppedWhenSessionDeleted(t *testing.T) {
	f := newLastSessionServer(t)
	f.attachStoreAt(t, t.TempDir(), f.project)
	sid := f.persistSession(t, f.project)
	if code := f.putLastSession(t, sid); code != http.StatusOK {
		t.Fatalf("PUT status %d", code)
	}

	if err := os.RemoveAll(f.store.SessionPath(sid)); err != nil {
		t.Fatal(err)
	}
	if got := f.getLastSession(t).SessionID; got != "" {
		t.Fatalf("session_id = %q after delete, want empty", got)
	}
}

func TestProjectLastSessionDroppedWhenOutOfProjectScope(t *testing.T) {
	f := newLastSessionServer(t)
	home := t.TempDir()
	f.attachStoreAt(t, home, f.project)
	other := t.TempDir()
	sid := f.persistSession(t, other)
	if code := f.putLastSession(t, sid); code != http.StatusOK {
		t.Fatalf("PUT status %d", code)
	}

	// Recorded while the session lived elsewhere: never route back to it.
	if got := f.getLastSession(t).SessionID; got != "" {
		t.Fatalf("session_id = %q for out-of-scope session, want empty", got)
	}
}

func TestProjectLastSessionClearedByEmptyPut(t *testing.T) {
	f := newLastSessionServer(t)
	f.attachStoreAt(t, t.TempDir(), f.project)
	sid := f.persistSession(t, f.project)
	if code := f.putLastSession(t, sid); code != http.StatusOK {
		t.Fatalf("PUT status %d", code)
	}

	if code := f.putLastSession(t, ""); code != http.StatusOK {
		t.Fatalf("PUT clear status %d, want 200", code)
	}
	if got := f.getLastSession(t).SessionID; got != "" {
		t.Fatalf("session_id = %q after clear, want empty", got)
	}
}

func TestProjectLastSessionPutValidation(t *testing.T) {
	f := newLastSessionServer(t)
	f.attachStoreAt(t, t.TempDir(), f.project)

	for _, bad := range []string{"../escape", "sess/../..", "with space"} {
		if code := f.putLastSession(t, bad); code != http.StatusBadRequest {
			t.Fatalf("PUT %q: status %d, want 400", bad, code)
		}
	}
	req, _ := http.NewRequest(http.MethodPut, f.ts.URL+"/foxxycode/project/last-session",
		strings.NewReader("not json"))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT invalid JSON: status %d, want 400", res.StatusCode)
	}
}

func TestProjectLastSessionWithoutProjectStore(t *testing.T) {
	f := newLastSessionServer(t)

	// GET degrades to "no last session" so the SPA simply shows the hero.
	dto := f.getLastSession(t)
	if dto.SessionID != "" {
		t.Fatalf("session_id = %q without a project store, want empty", dto.SessionID)
	}
	if dto.Path != f.project {
		t.Fatalf("path = %q, want default cwd %q", dto.Path, f.project)
	}
	if code := f.putLastSession(t, "sess_abc"); code != http.StatusServiceUnavailable {
		t.Fatalf("PUT without store: status %d, want 503", code)
	}
}

func TestProjectLastSessionPersistsAcrossRestart(t *testing.T) {
	// The IDE plugins bind a fresh random port on every launch, so the record
	// must come back from ~/.foxxycode/projects.json, not browser storage.
	f := newLastSessionServer(t)
	home := t.TempDir()
	f.attachStoreAt(t, home, f.project)
	sid := f.persistSession(t, f.project)
	if code := f.putLastSession(t, sid); code != http.StatusOK {
		t.Fatalf("PUT status %d", code)
	}

	reopened, err := project.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	f.srv.AttachProjectStore(reopened)
	if got := f.getLastSession(t).SessionID; got != sid {
		t.Fatalf("session_id = %q after reopening the store, want %q", got, sid)
	}
}
