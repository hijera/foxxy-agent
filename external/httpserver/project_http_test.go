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

func newProjectTestServer(t *testing.T) (*Server, *session.Manager, *httptest.Server, string) {
	t.Helper()
	defaultCWD := t.TempDir()
	cfg := &config.Config{
		Agent:  config.Agent{Model: "openai/gpt-4o"},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "ok", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), defaultCWD, nil)
	srv := New(cfg, mgr, slog.Default(), defaultCWD)
	t.Cleanup(srv.Drain)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, mgr, ts, defaultCWD
}

func attachProjectStore(t *testing.T, srv *Server) *project.Store {
	t.Helper()
	ps, err := project.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv.AttachProjectStore(ps)
	return ps
}

type projectDTO struct {
	Object       string `json:"object"`
	Path         string `json:"path"`
	Source       string `json:"source"`
	NativePicker bool   `json:"native_picker"`
}

func getProjectDTO(t *testing.T, ts *httptest.Server) projectDTO {
	t.Helper()
	res, err := http.Get(ts.URL + "/foxxycode/project")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /foxxycode/project status %d", res.StatusCode)
	}
	var dto projectDTO
	if err := json.NewDecoder(res.Body).Decode(&dto); err != nil {
		t.Fatal(err)
	}
	return dto
}

func TestProjectGetDefault(t *testing.T) {
	_, _, ts, defaultCWD := newProjectTestServer(t)

	dto := getProjectDTO(t, ts)
	if dto.Object != "foxxycode.project" {
		t.Fatalf("object = %q", dto.Object)
	}
	if dto.Path != defaultCWD {
		t.Fatalf("path = %q, want default %q", dto.Path, defaultCWD)
	}
	if dto.Source != "default" {
		t.Fatalf("source = %q, want default", dto.Source)
	}
	if dto.NativePicker {
		t.Fatal("native_picker = true without a picker hook")
	}
}

func TestProjectPutSetsCurrentAndNewSessionsInheritIt(t *testing.T) {
	srv, mgr, ts, defaultCWD := newProjectTestServer(t)
	attachProjectStore(t, srv)
	proj := t.TempDir()

	// Session created before the switch keeps the default cwd.
	pre, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/foxxycode/project",
		strings.NewReader(`{"path":`+jsonQuote(proj)+`}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT status %d", res.StatusCode)
	}
	var dto projectDTO
	if err := json.NewDecoder(res.Body).Decode(&dto); err != nil {
		t.Fatal(err)
	}
	if dto.Path != proj || dto.Source != "project" {
		t.Fatalf("PUT response = %+v, want path %q source project", dto, proj)
	}

	// New implicit HTTP session inherits the project cwd.
	rres, err := http.Post(ts.URL+"/v1/responses", "application/json",
		strings.NewReader(`{"model":"agent","input":"hi","stream":false}`))
	if err != nil {
		t.Fatal(err)
	}
	defer rres.Body.Close()
	if rres.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/responses status %d", rres.StatusCode)
	}
	sid := strings.TrimSpace(rres.Header.Get("X-FoxxyCode-Session-ID"))
	if sid == "" {
		t.Fatal("no X-FoxxyCode-Session-ID header on new session")
	}
	st := mgr.SessionByID(sid)
	if st == nil {
		t.Fatal("new session missing")
	}
	if got := st.GetCWD(); got != proj {
		t.Fatalf("new session cwd = %q, want project %q", got, proj)
	}

	// Pre-existing session is untouched.
	if got := mgr.SessionByID(pre.SessionID).GetCWD(); got != defaultCWD {
		t.Fatalf("old session cwd = %q, want %q", got, defaultCWD)
	}
}

func TestProjectPutValidation(t *testing.T) {
	srv, _, ts, _ := newProjectTestServer(t)

	// No store attached: 503.
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/foxxycode/project", strings.NewReader(`{"path":"x"}`))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("PUT without store: status %d, want 503", res.StatusCode)
	}

	attachProjectStore(t, srv)
	base := t.TempDir()
	file := filepath.Join(base, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"path":""}`, `{"path":` + jsonQuote(filepath.Join(base, "nope")) + `}`, `{"path":` + jsonQuote(file) + `}`, `not json`} {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/foxxycode/project", strings.NewReader(bad))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("PUT %s: status %d, want 400", bad, res.StatusCode)
		}
	}
}

func TestProjectRecentList(t *testing.T) {
	srv, _, ts, _ := newProjectTestServer(t)
	attachProjectStore(t, srv)
	base := t.TempDir()
	a := filepath.Join(base, "alpha")
	b := filepath.Join(base, "beta")
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{a, b} {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/foxxycode/project", strings.NewReader(`{"path":`+jsonQuote(p)+`}`))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("PUT %s: status %d", p, res.StatusCode)
		}
	}
	if err := os.RemoveAll(a); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(ts.URL + "/foxxycode/projects/recent")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET recent status %d", res.StatusCode)
	}
	var out struct {
		Object string `json:"object"`
		Data   []struct {
			Path         string `json:"path"`
			Name         string `json:"name"`
			LastOpenedAt string `json:"last_opened_at"`
			Exists       bool   `json:"exists"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "list" || len(out.Data) != 2 {
		t.Fatalf("recent = %+v", out)
	}
	if out.Data[0].Path != b || out.Data[0].Name != "beta" || !out.Data[0].Exists {
		t.Fatalf("recent[0] = %+v, want beta exists", out.Data[0])
	}
	if out.Data[1].Path != a || out.Data[1].Exists {
		t.Fatalf("recent[1] = %+v, want alpha missing", out.Data[1])
	}
}

func TestProjectPickFolder(t *testing.T) {
	srv, _, ts, _ := newProjectTestServer(t)
	attachProjectStore(t, srv)

	// No hook: 501.
	res, err := http.Post(ts.URL+"/foxxycode/project/pick-folder", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("pick without hook: status %d, want 501", res.StatusCode)
	}

	picked := t.TempDir()
	srv.SetFolderPicker(func(ctx context.Context) (string, bool, error) {
		return picked, false, nil
	})

	dto := getProjectDTO(t, ts)
	if !dto.NativePicker {
		t.Fatal("native_picker = false with a picker hook set")
	}

	res2, err := http.Post(ts.URL+"/foxxycode/project/pick-folder", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("pick status %d", res2.StatusCode)
	}
	var pick struct {
		Object    string `json:"object"`
		Cancelled bool   `json:"cancelled"`
		Path      string `json:"path"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&pick); err != nil {
		t.Fatal(err)
	}
	if pick.Object != "foxxycode.project_pick" || pick.Cancelled || pick.Path != picked {
		t.Fatalf("pick = %+v, want path %q", pick, picked)
	}

	// Picking must not change the current project by itself.
	if got := getProjectDTO(t, ts); got.Source != "default" {
		t.Fatalf("source after pick = %q, want default (pick must not set)", got.Source)
	}

	// Cancelled dialog.
	srv.SetFolderPicker(func(ctx context.Context) (string, bool, error) {
		return "", true, nil
	})
	res3, err := http.Post(ts.URL+"/foxxycode/project/pick-folder", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if err := json.NewDecoder(res3.Body).Decode(&pick); err != nil {
		t.Fatal(err)
	}
	if !pick.Cancelled || pick.Path != "" {
		t.Fatalf("cancelled pick = %+v", pick)
	}
}

func TestProjectPickFolderBusy(t *testing.T) {
	srv, _, ts, _ := newProjectTestServer(t)
	release := make(chan struct{})
	started := make(chan struct{})
	srv.SetFolderPicker(func(ctx context.Context) (string, bool, error) {
		close(started)
		<-release
		return "", true, nil
	})

	errCh := make(chan error, 1)
	go func() {
		res, err := http.Post(ts.URL+"/foxxycode/project/pick-folder", "application/json", nil)
		if err == nil {
			_ = res.Body.Close()
		}
		errCh <- err
	}()
	<-started

	res, err := http.Post(ts.URL+"/foxxycode/project/pick-folder", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("concurrent pick status %d, want 409", res.StatusCode)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
