//go:build http

package httpserver

import (
	"context"
	"fmt"
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
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// TestInspectExportArtifacts is a manual smoke test that spins up a real HTTP
// server, seeds a rich transcript, and writes the four export artifacts to the
// test's temp dir so a developer can open them and eyeball the formatting. It
// is skipped under -short because it exists for human inspection, not CI.
func TestInspectExportArtifacts(t *testing.T) {
	if testing.Short() {
		t.Skip("manual inspection smoke test")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sessRoot := filepath.Join(root, "sessions")
	for _, d := range []string{filepath.Join(home, "memory"), sessRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, &session.FileStore{Root: sessRoot})
	srv := New(cfg, mgr, slog.Default(), root)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Drain()

	sn, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(sn.SessionID)
	st.SetTitlePinned("Rich Markdown Chat")

	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "Can you explain how exports work and give me a code example?"})

	// Build the assistant content from plain strings so backtick fenced code
	// blocks and inline code render in the markdown without escaping issues.
	bt := "`"
	fence := bt + bt + bt
	content := "## How exports work\n\n" +
		"Exports render the **dialogue** — your questions and my answers — into a *portable* document. Tool calls and system rows are omitted so the result reads as a conversation.\n\n" +
		"Key points:\n\n" +
		"- Markdown formatting is preserved across formats.\n" +
		"- Headings, lists, and " + bt + "inline code" + bt + " all render.\n" +
		"- Code blocks keep their original indentation.\n\n" +
		"Here is a Go example:\n\n" +
		fence + "go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"exported!\")\n}\n" + fence + "\n\n" +
		"> Tip: pick JSON if you want to re-import the transcript elsewhere."
	st.AddMessage(llm.Message{
		Role:      llm.RoleAssistant,
		Reasoning: "The user wants both an explanation and a code sample. I should structure the answer with a heading, a short paragraph, a bullet list, and a fenced code block to cover the common markdown constructs.",
		Content:   content,
	})

	for _, fmtName := range []string{"json", "html", "pdf", "docx"} {
		out := filepath.Join(root, "rich-chat."+fmtName)
		body, err := exportViaHTTP(t, ts.URL, sn.SessionID, fmtName)
		if err != nil {
			t.Fatalf("export %s: %v", fmtName, err)
		}
		if err := os.WriteFile(out, body, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d bytes)", out, len(body))
		// When FOXXYCODE_EXPORT_INSPECT_DIR is set, also copy artifacts there so
		// a developer can open them in their real viewer (the temp dir is
		// removed when the test ends).
		if dest := os.Getenv("FOXXYCODE_EXPORT_INSPECT_DIR"); dest != "" {
			if err := os.MkdirAll(dest, 0o755); err == nil {
				_ = os.WriteFile(filepath.Join(dest, "rich-chat."+fmtName), body, 0o644)
			}
		}
	}
}

// exportViaHTTP calls the live export endpoint and returns the body bytes.
func exportViaHTTP(t *testing.T, base, sid, format string) ([]byte, error) {
	t.Helper()
	u := base + "/foxxycode/sessions/" + url.PathEscape(sid) + "/export?format=" + url.QueryEscape(format)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-FoxxyCode-Session-ID", sid)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d for %s", res.StatusCode, format)
	}
	return io.ReadAll(res.Body)
}
