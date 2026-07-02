//go:build http && !ui

package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxy-agent/internal/acp"
	"github.com/hijera/foxxy-agent/internal/config"
	"github.com/hijera/foxxy-agent/internal/session"
)

func TestHTTPWithoutEmbeddedUIServesPlain404OnRoot(t *testing.T) {
	cfg := &config.Config{
		Agent: config.Agent{Model: "openai/gpt-4o"},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), t.TempDir(), nil)
	srv := New(cfg, mgr, slog.Default(), t.TempDir())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); !strings.Contains(got, "without the embedded web UI") {
		t.Fatalf("unexpected body %q", got)
	}
}
