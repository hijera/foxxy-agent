//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/idecopy"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
)

type pasteClassifyResult struct {
	Kind         string `json:"kind"`
	PathRel      string `json:"pathRel"`
	StartLine    int    `json:"startLine"`
	EndLine      int    `json:"endLine"`
	TerminalName string `json:"terminalName"`
}

func classify(t *testing.T, url, text string) pasteClassifyResult {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	res, err := http.Post(url+"/foxxycode/ide/paste-classify", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
	var out pasteClassifyResult
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// testCwdAbsPath returns an absolute path under the test server's default cwd
// ("/tmp" in config; drive-qualified on Windows) joined with rel.
func testCwdAbsPath(t *testing.T, rel string) string {
	t.Helper()
	abs, err := filepath.Abs("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(abs, rel)
}

func TestPasteClassifyMatchesFileCandidateAcrossLineEndings(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	t.Cleanup(ideenv.Reset)
	idecopy.Reset()
	ideenv.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	idecopy.Offer(idecopy.Candidate{
		Kind: idecopy.KindFile, PathAbs: testCwdAbsPath(t, "Dockerfile"),
		StartLine: 21, EndLine: 31, Text: "FROM x\r\nRUN y\r\n",
	})
	got := classify(t, ts.URL, "FROM x\nRUN y")
	if got.Kind != "file" || got.PathRel != "Dockerfile" || got.StartLine != 21 || got.EndLine != 31 {
		t.Fatalf("got %+v", got)
	}
}

func TestPasteClassifyNoMatchIsNone(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	t.Cleanup(ideenv.Reset)
	t.Cleanup(ideterm.Reset)
	idecopy.Reset()
	ideenv.Reset()
	ideterm.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	if got := classify(t, ts.URL, "something\ncopied elsewhere"); got.Kind != "none" {
		t.Fatalf("got %+v", got)
	}
}

func TestPasteClassifyFallsBackToCurrentSelection(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	t.Cleanup(ideenv.Reset)
	idecopy.Reset()
	ideenv.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ideenv.Set(nil, "", &ideenv.Selection{
		File: testCwdAbsPath(t, "src/a.go"), StartLine: 5, EndLine: 6, Text: "alpha\nbeta",
	})
	got := classify(t, ts.URL, "alpha\nbeta")
	if got.Kind != "file" || got.PathRel != "src/a.go" || got.StartLine != 5 || got.EndLine != 6 {
		t.Fatalf("got %+v", got)
	}
}

func TestPasteClassifyPrefersCopyBufferOverSelection(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	t.Cleanup(ideenv.Reset)
	idecopy.Reset()
	ideenv.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	idecopy.Offer(idecopy.Candidate{
		Kind: idecopy.KindFile, PathAbs: testCwdAbsPath(t, "from-buffer.go"),
		StartLine: 1, EndLine: 2, Text: "same\ntext",
	})
	ideenv.Set(nil, "", &ideenv.Selection{
		File: testCwdAbsPath(t, "from-selection.go"), StartLine: 9, EndLine: 10, Text: "same\ntext",
	})
	got := classify(t, ts.URL, "same\ntext")
	if got.PathRel != "from-buffer.go" {
		t.Fatalf("got %+v", got)
	}
}

func TestPasteClassifyMatchesTerminalBuffer(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	t.Cleanup(ideenv.Reset)
	t.Cleanup(ideterm.Reset)
	idecopy.Reset()
	ideenv.Reset()
	ideterm.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ideterm.Set([]ideterm.Terminal{
		{ID: "1", Name: "dev", Output: "npm run dev\nserver ready on :3000\ndone", Active: true},
	})
	got := classify(t, ts.URL, "server ready on :3000\ndone")
	if got.Kind != "terminal" || got.TerminalName != "dev" {
		t.Fatalf("got %+v", got)
	}
}

func TestPasteClassifyTerminalNameWithSpacesIsBlank(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	t.Cleanup(idecopy.Reset)
	t.Cleanup(ideenv.Reset)
	idecopy.Reset()
	ideenv.Reset()
	ideterm.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ideterm.Set([]ideterm.Terminal{
		{ID: "1", Name: "dev server", Output: "compiled\nwith warnings", Active: true},
	})
	got := classify(t, ts.URL, "compiled\nwith warnings")
	if got.Kind != "terminal" || got.TerminalName != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestPasteClassifyRejectsWeakSignal(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	idecopy.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	idecopy.Offer(idecopy.Candidate{
		Kind: idecopy.KindFile, PathAbs: testCwdAbsPath(t, "a.go"), StartLine: 1, EndLine: 1, Text: "x := 1",
	})
	// Single-line text under 16 chars never chips.
	if got := classify(t, ts.URL, "x := 1"); got.Kind != "none" {
		t.Fatalf("got %+v", got)
	}
}

func TestPasteClassifyOutsideWorkspaceIsNone(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	idecopy.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	idecopy.Offer(idecopy.Candidate{
		Kind: idecopy.KindFile, PathAbs: "/elsewhere/secret.go", StartLine: 1, EndLine: 2, Text: "top\nsecret",
	})
	if got := classify(t, ts.URL, "top\nsecret"); got.Kind != "none" {
		t.Fatalf("got %+v", got)
	}
}
