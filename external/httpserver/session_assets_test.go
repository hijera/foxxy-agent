//go:build http

package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestFoxxycodeSessionAssetGet(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sessRoot := filepath.Join(root, "sessions")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Paths:     config.Paths{Home: home, CWD: "/tmp"},
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}
	store := &session.FileStore{Root: sessRoot}
	sid := "sess_assets"
	sd, err := store.EnsureLayout(sid)
	if err != nil {
		t.Fatal(err)
	}
	st := &session.State{ID: sid, CWD: "/tmp", Mode: session.ModeAgent, SessionDir: sd}
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}

	// A minimal PNG (signature bytes are enough to serve).
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01}
	assetsDir := session.AssetsPath(sd)
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "browser_1.png"), pngBytes, 0o444); err != nil {
		t.Fatal(err)
	}

	mgr := session.NewManager(cfg, noopSender{}, func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}, slog.Default(), "/tmp", store)
	srv := New(cfg, mgr, slog.Default(), "/tmp")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	do := func(name string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet,
			ts.URL+"/foxxycode/sessions/"+url.PathEscape(sid)+"/assets/"+name, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-FoxxyCode-Session-ID", sid)
		res, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Happy path: serves the file with an image content type.
	res := do("browser_1.png")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET asset status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if len(body) != len(pngBytes) {
		t.Errorf("body length = %d, want %d", len(body), len(pngBytes))
	}

	// Missing file: 404.
	res = do("does_not_exist.png")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("missing asset status = %d, want 404", res.StatusCode)
	}

	// Path traversal: rejected (the URL-encoded name never resolves outside assets).
	res = do("..%2f..%2fstate.json")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusNotFound {
		t.Errorf("traversal status = %d, want 400 or 404", res.StatusCode)
	}
}
