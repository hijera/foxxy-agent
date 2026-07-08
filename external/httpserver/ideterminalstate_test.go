//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/ideterm"
)

func TestFoxxyCodeIdeTerminalStateStoresSnapshot(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"terminals":[{"id":"1","name":"zsh","output":"ok\n","active":true},{"id":"2","name":"dev"}]}`
	res, err := http.Post(ts.URL+"/foxxycode/ide/terminal-state", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", res.StatusCode)
	}

	snap := ideterm.Get()
	if len(snap.Terminals) != 2 || snap.Terminals[0].Name != "zsh" || !snap.Terminals[0].Active {
		t.Fatalf("snapshot not stored: %+v", snap)
	}
}

func TestFoxxyCodeIdeTerminalStateGetReturnsNames(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	ideterm.Set([]ideterm.Terminal{
		{ID: "1", Name: "zsh", Output: "secret output", Active: true},
		{ID: "2", Name: "dev server"},
	})
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/foxxycode/ide/terminal-state")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
	var got struct {
		Terminals []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Active bool   `json:"active"`
			Output string `json:"output"`
		} `json:"terminals"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Terminals) != 2 || got.Terminals[0].Name != "zsh" || !got.Terminals[0].Active {
		t.Fatalf("unexpected terminals: %+v", got.Terminals)
	}
	if got.Terminals[0].Output != "" {
		t.Fatalf("GET must not expose output, got %q", got.Terminals[0].Output)
	}
}

func TestFoxxyCodeIdeTerminalStateRejectsBadJSON(t *testing.T) {
	t.Cleanup(ideterm.Reset)
	_, srv, _ := testHTTPServerPersist(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/foxxycode/ide/terminal-state", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}
