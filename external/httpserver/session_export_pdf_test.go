//go:build http

package httpserver

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// wrapRuns is the piece that lets a table cell hold a formatted sentence: fpdf's
// own SplitText measures one string against one font, so it cannot wrap a phrase
// whose middle is bold or monospaced.
func TestWrapRunsFitsTheColumnAndKeepsFormatting(t *testing.T) {
	pdf := newExportPDF("probe")
	pdf.AddPage()

	runs := []exportRun{
		{text: "handing the transcript to "},
		{text: "someone", bold: true},
		{text: " who will edit it"},
	}
	const width = 30.0

	lines := wrapRuns(pdf, runs, width, "", exportTableSize)
	if len(lines) < 2 {
		t.Fatalf("expected the phrase to wrap inside %.0fmm, got %d line(s)", width, len(lines))
	}
	for i, line := range lines {
		if w := runsWidth(pdf, line, "", exportTableSize); w > width+0.01 {
			t.Errorf("line %d is %.2fmm wide, over the %.0fmm column", i, w, width)
		}
	}
	// Nothing may be dropped, and the emphasis has to survive the split.
	var rebuilt strings.Builder
	bold := false
	for _, line := range lines {
		for _, r := range line {
			rebuilt.WriteString(r.text)
			if r.bold && strings.Contains(r.text, "someone") {
				bold = true
			}
		}
	}
	if got := strings.Join(strings.Fields(rebuilt.String()), " "); got != "handing the transcript to someone who will edit it" {
		t.Errorf("wrapping lost text: %q", got)
	}
	if !bold {
		t.Error("the bold run lost its weight while wrapping")
	}
}

// A single token wider than the column (a URL, a long identifier) has to be
// broken mid-word or it would overflow the cell.
func TestWrapRunsBreaksAnOverlongWord(t *testing.T) {
	pdf := newExportPDF("probe")
	pdf.AddPage()

	const width = 20.0
	word := strings.Repeat("supercalifragilistic", 3)
	lines := wrapRuns(pdf, []exportRun{{text: word}}, width, "", exportTableSize)

	if len(lines) < 2 {
		t.Fatalf("an over-long word must be split, got %d line(s)", len(lines))
	}
	var rebuilt strings.Builder
	for _, line := range lines {
		if w := runsWidth(pdf, line, "", exportTableSize); w > width+0.01 {
			t.Errorf("split line is %.2fmm wide, over the %.0fmm column", w, width)
		}
		rebuilt.WriteString(runsText(line))
	}
	if rebuilt.String() != word {
		t.Errorf("splitting lost characters: %q", rebuilt.String())
	}
}

func TestTableColumnWidthsFillTheContentBox(t *testing.T) {
	pdf := newExportPDF("probe")
	pdf.AddPage()
	avail := pdfContentWidth(pdf)

	tbl := &exportTable{
		header: []exportTableCell{{runs: []exportRun{{text: "id"}}}, {runs: []exportRun{{text: "description"}}}},
		rows: [][]exportTableCell{{
			{runs: []exportRun{{text: "1"}}},
			{runs: []exportRun{{text: strings.Repeat("a very long cell of prose ", 12)}}},
		}},
	}
	widths := tableColumnWidths(pdf, tbl, 2, avail)

	total := 0.0
	for i, w := range widths {
		if w < tableMinColWidth-0.01 {
			t.Errorf("column %d is %.2fmm, under the %.0fmm floor", i, w, tableMinColWidth)
		}
		total += w
	}
	if total < avail-0.5 || total > avail+0.5 {
		t.Errorf("columns total %.2fmm, want the %.2fmm content box", total, avail)
	}
	if widths[1] <= widths[0] {
		t.Errorf("the prose column (%.2f) should be wider than the id column (%.2f)", widths[1], widths[0])
	}
}

// The regression this whole change exists for: a markdown table must reach the
// page as a drawn cell grid, not as a paragraph of pipe characters.
func TestRenderPDFDrawsTablesAsCells(t *testing.T) {
	md := strings.Join([]string{
		"| Format | Notes |",
		"|--------|-------|",
		"| pdf    | fixed |",
		"| docx   | edit  |",
	}, "\n")

	rects := pdfRects(t, md)
	// Three rows of two columns, each cell a rectangle of its own.
	if len(rects) != 6 {
		t.Fatalf("expected 6 cell rectangles, got %d: %+v", len(rects), rects)
	}
	// Cells in a row share a top edge; columns share a left edge.
	if rects[0].y != rects[1].y {
		t.Errorf("the two header cells are not on one row: %.2f vs %.2f", rects[0].y, rects[1].y)
	}
	if rects[0].x != rects[2].x || rects[0].w != rects[2].w {
		t.Errorf("column 0 is not aligned between rows: %+v vs %+v", rects[0], rects[2])
	}
	if rects[2].y == rects[4].y {
		t.Error("two body rows were drawn at the same height")
	}
}

// A fenced block gets a shaded box spanning the text column, so it reads as code
// rather than as indented prose.
func TestRenderPDFBoxesCodeBlocks(t *testing.T) {
	rects := pdfRects(t, "```go\npackage main\n\nfunc main() {}\n```")
	if len(rects) != 1 {
		t.Fatalf("expected exactly one code box, got %d: %+v", len(rects), rects)
	}
	pdf := newExportPDF("probe")
	pdf.AddPage()
	if want := pdfContentWidth(pdf) * 72 / 25.4; rects[0].w < want-1 || rects[0].w > want+1 {
		t.Errorf("code box is %.2fpt wide, want the %.2fpt content column", rects[0].w, want)
	}
}

// Links have to stay clickable: dropping the URL was the old behaviour and it
// makes a referenced page unrecoverable from the document.
func TestRenderPDFKeepsLinkTargets(t *testing.T) {
	body, err := renderPDFExport(exportDocument{
		Title:      "Links",
		ExportedAt: "2026-01-01T00:00:00Z",
		Messages: []exportMessage{{
			Role:    "assistant",
			Content: "See [the docs](https://example.com/docs) for more.",
		}},
	})
	if err != nil {
		t.Fatalf("renderPDFExport: %v", err)
	}
	if !bytes.Contains(body, []byte("https://example.com/docs")) {
		t.Error("the link target did not reach the PDF")
	}
	if !bytes.Contains(body, []byte("/URI")) {
		t.Error("the PDF carries no link annotation")
	}
}

// A transcript table can easily outrun one page. The header has to be redrawn at
// the top of each continuation, or the reader loses the column meanings halfway
// through.
func TestRenderPDFRepeatsTableHeaderAcrossPages(t *testing.T) {
	rows := []string{"| Step | What it does |", "|------|--------------|"}
	for i := 0; i < 60; i++ {
		rows = append(rows, fmt.Sprintf("| %d | row number %d of a table that outgrows the page |", i, i))
	}

	pdf := newExportPDF("probe")
	pdf.AddPage()
	writeBlocksPDF(pdf, markdownToBlocks(strings.Join(rows, "\n"), nil))
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output: %v", err)
	}
	if pdf.PageNo() < 2 {
		t.Fatalf("the table fit on %d page(s); the fixture needs to be longer", pdf.PageNo())
	}

	// Header cells are the only ones filled as well as stroked, so fpdf writes
	// them with the "B" operator and body cells with "S".
	filled := 0
	for _, stream := range pdfContentStreams(t, buf.Bytes()) {
		filled += len(regexp.MustCompile(`re B\b`).FindAllString(stream, -1))
	}
	if want := 2 * pdf.PageNo(); filled != want {
		t.Errorf("drew %d header cells over %d pages, want %d (two columns per page)",
			filled, pdf.PageNo(), want)
	}
}

// A long snippet has to keep its shaded box on every page it reaches, rather
// than painting one rectangle across the page break.
func TestRenderPDFBoxesCodeOnEveryPage(t *testing.T) {
	lines := []string{"```go"}
	for i := 0; i < 120; i++ {
		lines = append(lines, fmt.Sprintf("\tfmt.Println(%d) // a line of a snippet that outgrows one page", i))
	}
	lines = append(lines, "```")

	pdf := newExportPDF("probe")
	pdf.AddPage()
	writeBlocksPDF(pdf, markdownToBlocks(strings.Join(lines, "\n"), nil))
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output: %v", err)
	}
	if pdf.PageNo() < 2 {
		t.Fatalf("the snippet fit on %d page(s); the fixture needs to be longer", pdf.PageNo())
	}

	boxes := 0
	bottom := pdfPageBottom(pdf)
	for _, stream := range pdfContentStreams(t, buf.Bytes()) {
		for _, m := range regexp.MustCompile(`([\d.]+) ([\d.]+) ([\d.]+) (-?[\d.]+) re B`).FindAllStringSubmatch(stream, -1) {
			boxes++
			// PDF y grows upward from the page foot; a box must not reach past the
			// bottom margin, which is what a single rectangle spanning the break
			// would do.
			top := mustFloat(t, m[2])
			height := -mustFloat(t, m[4])
			foot := (top - height) / 72 * 25.4
			if _, pageH := pdf.GetPageSize(); pageH-foot > bottom+1 {
				t.Errorf("a code box runs %.1fmm past the bottom margin", pageH-foot-bottom)
			}
		}
	}
	if boxes < pdf.PageNo() {
		t.Errorf("drew %d code boxes over %d pages, want one per page", boxes, pdf.PageNo())
	}
}

// pdfRect is one rectangle drawn in a page content stream, in PDF points.
type pdfRect struct{ x, y, w, h float64 }

// pdfRects renders markdown through the real PDF pipeline and returns every
// rectangle drawn for it. Rectangles are how tables and code boxes are built,
// so this is the cheapest honest assertion about their geometry.
func pdfRects(t *testing.T, md string) []pdfRect {
	t.Helper()
	pdf := newExportPDF("probe")
	pdf.AddPage()
	writeBlocksPDF(pdf, markdownToBlocks(md, nil))
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output: %v", err)
	}

	var out []pdfRect
	re := regexp.MustCompile(`([\d.]+) ([\d.]+) ([\d.]+) (-?[\d.]+) re`)
	for _, stream := range pdfContentStreams(t, buf.Bytes()) {
		for _, m := range re.FindAllStringSubmatch(stream, -1) {
			out = append(out, pdfRect{
				x: mustFloat(t, m[1]), y: mustFloat(t, m[2]),
				w: mustFloat(t, m[3]), h: mustFloat(t, m[4]),
			})
		}
	}
	return out
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

// pdfContentStreams inflates every deflated stream of a PDF that looks like a
// page content stream, so a test can read the drawing operators back.
func pdfContentStreams(t *testing.T, pdfBytes []byte) []string {
	t.Helper()
	var out []string
	for _, m := range pdfStreamRe.FindAllSubmatch(pdfBytes, -1) {
		zr, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		content, err := io.ReadAll(zr)
		_ = zr.Close()
		if err != nil {
			continue
		}
		out = append(out, string(content))
	}
	return out
}
