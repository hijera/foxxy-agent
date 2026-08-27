//go:build http

package httpserver

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
//
// The transcript deliberately covers every construct the renderers support —
// table, nested list, task list, ordered list, links, an embedded image, a
// thematic break, a multi-paragraph quote, strikethrough and an over-long code
// line — so a regression in any of them is visible in one look at the output.
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

	// A picture the export can actually embed, written into the session assets
	// directory the same way an upload would be.
	writeInspectAsset(t, st, "diagram.png")

	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "Can you explain how exports work and give me a code example?"})
	st.AddMessage(llm.Message{
		Role:      llm.RoleAssistant,
		Reasoning: "The user wants both an explanation and a code sample. I should structure the answer with a heading, a comparison table, a bullet list, and a fenced code block to cover the common markdown constructs.",
		Content:   inspectAssistantMarkdown(),
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

// inspectAssistantMarkdown builds the assistant turn from plain strings so
// backtick fenced code blocks and inline code render in the markdown without
// escaping issues.
func inspectAssistantMarkdown() string {
	bt := "`"
	fence := bt + bt + bt
	nl := "\n"

	return strings.Join([]string{
		"## How exports work",
		"",
		"Exports render the **dialogue** — your questions and my answers — into a *portable* " +
			"document. Tool calls and system rows are omitted so the result reads as a " +
			"conversation. See the [HTTP API docs](https://example.com/docs/http-api) for the " +
			"full contract.",
		"",
		"### Format comparison",
		"",
		"| Format | Editable | Styling | Best for |",
		"|--------|:--------:|:-------:|----------|",
		"| " + bt + "pdf" + bt + " | no | yes | printing, or sharing a fixed layout |",
		"| " + bt + "docx" + bt + " | **yes** | yes | handing the transcript to someone who will edit it |",
		"| " + bt + "html" + bt + " | yes | yes | reading in a browser, or archiving one self-contained file |",
		"| " + bt + "json" + bt + " | yes | ~~no~~ | re-importing the transcript into another tool |",
		"",
		"Key points:",
		"",
		"- Markdown formatting is preserved across formats.",
		"  - Nested items keep their level.",
		"    - Including a third one.",
		"- Headings, lists, and " + bt + "inline code" + bt + " all render.",
		"- Code blocks keep their original indentation.",
		"",
		"Checklist:",
		"",
		"- [x] Tables render as real tables",
		"- [x] Code is highlighted",
		"- [ ] Someone reviews the output",
		"",
		"Ordered steps:",
		"",
		"1. Pick a format from the menu.",
		"2. Wait for the download.",
		"3. Open it.",
		"",
		"Here is a Go example:",
		"",
		fence + "go",
		"package main",
		"",
		`import "fmt"`,
		"",
		"// exportAll writes every format, and this comment is deliberately long enough " +
			"that it has to wrap inside the code box to prove that wrapping works.",
		"func main() {",
		`    fmt.Println("exported!")`,
		"}",
		fence,
		"",
		"---",
		"",
		"> Tip: pick JSON if you want to re-import the transcript elsewhere.",
		">",
		"> A quote can hold more than one paragraph, and each keeps its own rule.",
		"",
		"![Export pipeline diagram](diagram.png)",
		"",
	}, nl)
}

// writeInspectAsset drops a small PNG into the session's assets directory so the
// inspection artifacts exercise the image-embedding path.
func writeInspectAsset(t *testing.T, st *session.State, name string) {
	t.Helper()
	dir := strings.TrimSpace(st.GetPersistedSessionDir())
	if dir == "" {
		t.Fatal("session has no persisted directory")
	}
	assets := session.AssetsPath(dir)
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 320, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y * 2 % 256), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, name), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
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
