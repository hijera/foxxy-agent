//go:build http

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/ideenv"
)

func TestFoxxyCodeIdeEditorStateStoresSnapshot(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"openFiles":["/ws/a.go","/ws/b.go"],"activeFile":"/ws/a.go"}`
	res, err := http.Post(ts.URL+"/foxxycode/ide/editor-state", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", res.StatusCode)
	}

	snap := ideenv.Get()
	if snap.ActiveFile != "/ws/a.go" || len(snap.OpenFiles) != 2 {
		t.Fatalf("snapshot not stored: %+v", snap)
	}
}

func TestFoxxyCodeIdeEditorStateRejectsBadJSON(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/foxxycode/ide/editor-state", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}
