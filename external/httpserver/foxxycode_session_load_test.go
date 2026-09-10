//go:build http

package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// countingSender counts the available-commands update that loadSessionFromDisk emits once per
// load, so a test can tell how many times a session was actually read off disk.
type countingSender struct {
	noopSender
	loads atomic.Int64
}

func (c *countingSender) SendSessionUpdate(_ string, u interface{}) error {
	if _, ok := u.(acp.AvailableCommandsUpdate); ok {
		c.loads.Add(1)
	}
	return nil
}

// A panel opening a chat fires every per-session route at once. Each one used to run a full
// load from disk, and each load closed the previous one's state - so the boot fan-out fought
// itself while holding all of the webview's connections, and the turn's POST never got one.
func TestConcurrentPerSessionRoutesLoadTheSessionOnce(t *testing.T) {
	cwd := t.TempDir()
	root := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(root, "sessions")}
	if err := os.MkdirAll(store.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Agent:  config.Agent{Model: "openai/gpt-4o"},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o"}},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	sender := &countingSender{}
	mgr := session.NewManager(cfg, sender, runner, slog.Default(), cwd, store)

	res, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	sid := res.SessionID
	// Drop it from memory: this is the cold session a freshly started backend is asked about.
	mgr.ForgetLiveSession(sid)
	sender.loads.Store(0)

	srv := New(cfg, mgr, slog.Default(), cwd)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	paths := []string{
		"/foxxycode/sessions/" + sid + "/messages",
		"/foxxycode/sessions/" + sid + "/tool-calls",
		"/foxxycode/sessions/" + sid + "/stats",
		"/foxxycode/sessions/" + sid + "/branches",
		"/foxxycode/sessions/" + sid + "/activity",
		"/foxxycode/sessions/" + sid + "/plans",
		"/foxxycode/sessions/" + sid + "/messages",
		"/foxxycode/sessions/" + sid + "/stats",
	}
	codes := make([]int, len(paths))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			<-start
			resp, err := http.Get(ts.URL + p)
			if err != nil {
				codes[i] = -1
				return
			}
			defer func() { _ = resp.Body.Close() }()
			codes[i] = resp.StatusCode
		}(i, p)
	}
	close(start)
	wg.Wait()

	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("%s: status %d", paths[i], code)
		}
	}
	if got := sender.loads.Load(); got != 1 {
		t.Fatalf("expected the session to be read from disk once, got %d", got)
	}
}
