//go:build http

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/idecopy"
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
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", res.StatusCode)
	}

	snap := ideenv.Get()
	if snap.ActiveFile != "/ws/a.go" || len(snap.OpenFiles) != 2 {
		t.Fatalf("snapshot not stored: %+v", snap)
	}
}

func TestFoxxyCodeIdeEditorStateStoresSelectionAndOffersCandidate(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	t.Cleanup(idecopy.Reset)
	idecopy.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"openFiles":["/ws/a.go"],"activeFile":"/ws/a.go","selection":{"file":"/ws/a.go","startLine":21,"endLine":31,"text":"x := 1\ny := 2"}}`
	for i := 0; i < 2; i++ {
		res, err := http.Post(ts.URL+"/foxxycode/ide/editor-state", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("status %d, want 204", res.StatusCode)
		}
	}

	snap := ideenv.Get()
	if snap.Selection == nil || snap.Selection.File != "/ws/a.go" || snap.Selection.StartLine != 21 || snap.Selection.EndLine != 31 {
		t.Fatalf("selection not stored: %+v", snap.Selection)
	}
	cands := idecopy.Candidates()
	if len(cands) != 1 {
		t.Fatalf("want 1 copy candidate (deduped), got %d: %+v", len(cands), cands)
	}
	if cands[0].Kind != idecopy.KindFile || cands[0].PathAbs != "/ws/a.go" || cands[0].StartLine != 21 {
		t.Fatalf("candidate %+v", cands[0])
	}
}

func TestFoxxyCodeIdeEditorStateClearsSelection(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	t.Cleanup(idecopy.Reset)
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ideenv.Set(nil, "", &ideenv.Selection{File: "/ws/a.go", StartLine: 1, EndLine: 1, Text: "x"})
	res, err := http.Post(ts.URL+"/foxxycode/ide/editor-state", "application/json",
		strings.NewReader(`{"openFiles":["/ws/a.go"],"activeFile":"/ws/a.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if snap := ideenv.Get(); snap.Selection != nil {
		t.Fatalf("selection not cleared: %+v", snap.Selection)
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
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}
