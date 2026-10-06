//go:build http

package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/idecopy"
)

func TestFoxxyCodeIdeCopyBufferStoresFileCandidate(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	idecopy.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"kind":"file","path":"/ws/Dockerfile","startLine":21,"endLine":31,"text":"FROM x\nRUN y"}`
	res, err := http.Post(ts.URL+"/foxxycode/ide/copy-buffer", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", res.StatusCode)
	}
	cands := idecopy.Candidates()
	if len(cands) != 1 || cands[0].Kind != idecopy.KindFile || cands[0].PathAbs != "/ws/Dockerfile" ||
		cands[0].StartLine != 21 || cands[0].EndLine != 31 {
		t.Fatalf("candidates %+v", cands)
	}
}

func TestFoxxyCodeIdeCopyBufferStoresTerminalCandidate(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	idecopy.Reset()
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"kind":"terminal","terminalName":"dev","text":"npm run dev\nready"}`
	res, err := http.Post(ts.URL+"/foxxycode/ide/copy-buffer", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", res.StatusCode)
	}
	cands := idecopy.Candidates()
	if len(cands) != 1 || cands[0].Kind != idecopy.KindTerminal || cands[0].TerminalName != "dev" {
		t.Fatalf("candidates %+v", cands)
	}
}

func TestFoxxyCodeIdeCopyBufferRejectsBadInput(t *testing.T) {
	t.Cleanup(idecopy.Reset)
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, body := range []string{"not json", `{"kind":"weird","text":"x"}`} {
		res, err := http.Post(ts.URL+"/foxxycode/ide/copy-buffer", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%q: status %d, want 400", body, res.StatusCode)
		}
	}
}
