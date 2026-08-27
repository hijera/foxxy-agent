//go:build http

package httpserver

import (
	"strings"
	"testing"
)

// htmlFor renders one assistant turn and returns the page, so the assertions
// below read against the same output a user downloads.
func htmlFor(t *testing.T, content, assetsDir string) string {
	t.Helper()
	body, err := renderHTMLExport(exportDocument{
		Title:      "Chat",
		ExportedAt: "2026-01-01T00:00:00Z",
		Messages:   []exportMessage{{Role: "assistant", Content: content}},
		assetsDir:  assetsDir,
	})
	if err != nil {
		t.Fatalf("renderHTMLExport: %v", err)
	}
	return string(body)
}

// GFM was never wired into the HTML renderer either, so a pipe table used to
// arrive as a paragraph of pipes even though the stylesheet already styled
// table/th/td.
func TestRenderHTMLExportRendersGFM(t *testing.T) {
	md := strings.Join([]string{
		"| Format | Notes |",
		"|--------|:-----:|",
		"| pdf    | fixed |",
		"",
		"- [x] done",
		"- [ ] pending",
		"",
		"~~gone~~ and <https://auto.example> and a rule:",
		"",
		"---",
	}, "\n")
	page := htmlFor(t, md, "")

	for _, want := range []string{
		"<table>", "<th", "<td", // a real table
		`<input checked="" disabled="" type="checkbox"`, // task list
		"<del>gone</del>",                // strikethrough
		`<a href="https://auto.example"`, // autolink
		"<hr>",                           // thematic break
		`style="text-align:center"`,      // per-column alignment
	} {
		if !strings.Contains(page, want) {
			t.Errorf("HTML export is missing %q", want)
		}
	}
	if strings.Contains(page, "| pdf    | fixed |") {
		t.Error("the table was rendered as literal pipe text")
	}
}

func TestRenderHTMLExportHighlightsCode(t *testing.T) {
	page := htmlFor(t, "```go\npackage main\n\nfunc main() {}\n```", "")

	if !strings.Contains(page, `<pre class="chroma"><code>`) {
		t.Fatal("no highlighted code block in the output")
	}
	// Token classes plus a generated stylesheet, not inline styles: the page has
	// to be able to swap the whole palette for a dark reader.
	// Go's "package" is a KeywordNamespace token, class "kn".
	if !strings.Contains(page, `<span class="kn">`) {
		t.Error("the code block carries no token classes")
	}
	if !strings.Contains(page, ".chroma .k {") {
		t.Error("the generated stylesheet defines no keyword colour")
	}
	// A language chroma does not know still renders, just without colour.
	plain := htmlFor(t, "```nosuchlang\nhello\n```", "")
	if !strings.Contains(plain, "<pre><code>hello") {
		t.Error("an unknown language lost its code block")
	}
}

// The token palette is baked into the file, so a dark reader must get a dark one
// — the light "github" blue on a dark code box sits near 1.6:1 contrast.
func TestRenderHTMLExportShipsBothCodePalettes(t *testing.T) {
	page := htmlFor(t, "```go\npackage main\n```", "")

	// The page's own theme block also opens a dark media query and comes first;
	// the chroma palette is the last one in the sheet.
	darkStart := strings.LastIndex(page, "@media (prefers-color-scheme: dark) {\n")
	if darkStart < 0 {
		t.Fatal("no dark token palette in the stylesheet")
	}
	light, dark := page[:darkStart], page[darkStart:]
	lightKeyword := strings.Contains(light, ".chroma .k {")
	darkKeyword := strings.Contains(dark, ".chroma .k {")
	if !lightKeyword || !darkKeyword {
		t.Errorf("keyword colour defined light=%v dark=%v, want both", lightKeyword, darkKeyword)
	}
	// The code box keeps the page's own surface colour rather than chroma's.
	if !strings.Contains(page, "pre, pre.chroma {") {
		t.Error("the code box does not override chroma's background")
	}
}

// The exported page is a single file a user mails or archives, so a picture it
// can resolve locally has to travel inside it.
func TestRenderHTMLExportInlinesLocalImages(t *testing.T) {
	dir := t.TempDir()
	writePNGFixture(t, dir, "shot.png", 64, 32)

	page := htmlFor(t, "![a screenshot](shot.png)", dir)
	if !strings.Contains(page, `src="data:image/png;base64,`) {
		t.Error("a local asset was not inlined as a data URI")
	}

	// A remote URL is left alone: fetching it would make a download request
	// issue arbitrary outbound traffic.
	remote := htmlFor(t, "![a chart](https://example.com/chart.png)", dir)
	if strings.Contains(remote, "data:image") {
		t.Error("a remote image must not be fetched at export time")
	}
	if !strings.Contains(remote, `src="https://example.com/chart.png"`) {
		t.Error("the remote image lost its source")
	}
}

// Uploads reach the transcript as a <foxxycode_session_assets> wrapper; printing
// that raw XML in the page is exactly the kind of noise the readable formats
// exist to avoid.
func TestRenderHTMLExportShowsAttachmentsAsFiles(t *testing.T) {
	dir := t.TempDir()
	path := writePNGFixture(t, dir, "shot.png", 40, 20)

	doc := readableExportDocument(exportDocument{
		Title:      "Uploads",
		ExportedAt: "2026-01-01T00:00:00Z",
		Messages: []exportMessage{{
			Role: "user",
			Content: "Look at this\n\n<foxxycode_session_assets>Uploaded files:\n- " +
				path + " (shot.png)\n</foxxycode_session_assets>",
		}},
		assetsDir: dir,
	})
	body, err := renderHTMLExport(doc)
	if err != nil {
		t.Fatalf("renderHTMLExport: %v", err)
	}
	page := string(body)

	if strings.Contains(page, "foxxycode_session_assets") {
		t.Error("the raw uploads wrapper reached the page")
	}
	if !strings.Contains(page, "Attachments:") {
		t.Error("the upload is not shown as an attachment")
	}
	if !strings.Contains(page, "data:image/png;base64,") {
		t.Error("the uploaded picture was not inlined")
	}
}

// The stylesheet has to survive printing and a dark-themed reader, since the
// point of the HTML export is that it is readable anywhere.
func TestRenderHTMLExportStyleSheetCoversPrintAndDark(t *testing.T) {
	page := htmlFor(t, "hello", "")
	for _, want := range []string{
		"@media print",
		"prefers-color-scheme: dark",
		"break-inside: avoid",
		"white-space: pre-wrap", // long code lines must not be clipped on paper
		"border-collapse: collapse",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
}
