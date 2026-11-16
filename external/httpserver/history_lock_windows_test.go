//go:build http && windows

package httpserver

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestHistoryListDoesNotPublishPartialResultsDuringMetadataLock(t *testing.T) {
	home := t.TempDir()
	store := &session.FileStore{Root: filepath.Join(home, "sessions")}
	dir, err := store.EnsureLayout("sess_locked_history")
	if err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(dir, "session.json")
	if err := os.WriteFile(metaPath, []byte(`{"version":1,"id":"sess_locked_history","cwd":"C:/plain-folder"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.Paths{Home: home}}
	mgr := session.NewManager(cfg, noopSender{}, nil, slog.Default(), home, store)
	server := New(cfg, mgr, slog.Default(), home)
	path, err := syscall.UTF16PtrFromString(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != syscall.InvalidHandle {
			if err := syscall.CloseHandle(handle); err != nil {
				t.Error(err)
			}
		}
	}()
	read := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/foxxycode/sessions", nil))
		return response
	}
	response := read()
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "list failed") {
		t.Fatalf("busy metadata: status=%d body=%s", response.Code, response.Body.String())
	}
	if err := syscall.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = syscall.InvalidHandle
	response = read()
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "sess_locked_history") {
		t.Fatalf("released metadata: status=%d body=%s", response.Code, response.Body.String())
	}
}
