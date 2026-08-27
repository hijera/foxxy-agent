//go:build http

package httpserver

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The regression this change exists for: a markdown table must reach Word as a
// real table with selectable cells, not as a paragraph of pipe characters.
func TestDocxRendersTablesAsWordTables(t *testing.T) {
	md := strings.Join([]string{
		"| Format | Notes | Size |",
		"|--------|:-----:|-----:|",
		"| pdf    | fixed |  1 MB|",
		"| docx   | edit  | 40 kB|",
	}, "\n")
	body := newDocxDoc().blockXML(markdownToBlocks(md, nil)[0], false)

	if !strings.Contains(body, "<w:tbl>") {
		t.Fatalf("no table element in the output: %s", body)
	}
	if got := strings.Count(body, "<w:tr>"); got != 3 {
		t.Errorf("table has %d rows, want 3 (header plus two body rows)", got)
	}
	if got := strings.Count(body, "<w:tc>"); got != 9 {
		t.Errorf("table has %d cells, want 9", got)
	}
	if got := strings.Count(body, "<w:gridCol "); got != 3 {
		t.Errorf("table grid declares %d columns, want 3", got)
	}
	// The header repeats when the table spills onto the next page.
	if !strings.Contains(body, "<w:tblHeader/>") {
		t.Error("the header row is not marked as a repeating header")
	}
	if !strings.Contains(body, `<w:shd w:val="clear" w:color="auto" w:fill="F6F8FA"/>`) {
		t.Error("the header row carries no shading")
	}
	// Per-column alignment from the delimiter row.
	for _, jc := range []string{`<w:jc w:val="center"/>`, `<w:jc w:val="right"/>`} {
		if !strings.Contains(body, jc) {
			t.Errorf("missing column alignment %s", jc)
		}
	}
	if strings.Contains(body, "|") {
		t.Error("pipe characters leaked into the table markup")
	}
	// Word and LibreOffice both need a paragraph after a table.
	if !strings.HasSuffix(body, "<w:p/>") {
		t.Error("no paragraph follows the table")
	}
}

func TestDocxColumnWidthsSpanTheTextColumn(t *testing.T) {
	tbl := &exportTable{
		header: []exportTableCell{{runs: []exportRun{{text: "id"}}}, {runs: []exportRun{{text: "description"}}}},
		rows: [][]exportTableCell{{
			{runs: []exportRun{{text: "1"}}},
			{runs: []exportRun{{text: strings.Repeat("long prose ", 10)}}},
		}},
	}
	widths := docxColumnWidths(tbl, 2)

	total := 0
	for _, w := range widths {
		total += w
	}
	if total != docxContentWidth {
		t.Errorf("columns total %d twips, want %d", total, docxContentWidth)
	}
	if widths[1] <= widths[0] {
		t.Errorf("the prose column (%d) should be wider than the id column (%d)", widths[1], widths[0])
	}
}

func TestDocxNestsListLevels(t *testing.T) {
	md := "- top\n  - middle\n    - deep"
	d := newDocxDoc()
	var body strings.Builder
	for _, b := range markdownToBlocks(md, nil) {
		body.WriteString(d.blockXML(b, false))
	}
	for level := 0; level <= 2; level++ {
		if !strings.Contains(body.String(), `<w:ilvl w:val="`+itoa(level)+`"/>`) {
			t.Errorf("no list item at level %d", level)
		}
	}
	// Every level the body names has to exist in the numbering definition.
	for level := 0; level <= docxMaxListLevel; level++ {
		if !strings.Contains(numberingXML, `<w:lvl w:ilvl="`+itoa(level)+`">`) {
			t.Errorf("numbering.xml defines no level %d", level)
		}
	}
}

// A task item has no native Word form, so it carries its own box glyph and must
// not also pick up a numbering bullet.
func TestDocxRendersTaskListsAsCheckboxes(t *testing.T) {
	d := newDocxDoc()
	blocks := markdownToBlocks("- [x] done\n- [ ] pending", nil)
	done := d.blockXML(blocks[0], false)
	pending := d.blockXML(blocks[1], false)

	if !strings.Contains(done, "☑") {
		t.Errorf("a completed task has no checked box: %s", done)
	}
	if !strings.Contains(pending, "☐") {
		t.Errorf("an open task has no empty box: %s", pending)
	}
	if strings.Contains(done, "<w:numId") {
		t.Error("a task item must not also draw a numbering bullet")
	}
}

func TestDocxKeepsLinkTargets(t *testing.T) {
	d := newDocxDoc()
	body := d.blockXML(markdownToBlocks("See [the docs](https://example.com/docs) now.", nil)[0], false)

	if !strings.Contains(body, "<w:hyperlink r:id=") {
		t.Fatalf("the link did not become a hyperlink: %s", body)
	}
	rels := d.docRelsXML()
	if !strings.Contains(rels, `Target="https://example.com/docs"`) {
		t.Errorf("the relationship file has no target for the link: %s", rels)
	}
	if !strings.Contains(rels, `TargetMode="External"`) {
		t.Error("an external link needs TargetMode=External")
	}
	if !strings.Contains(stylesXML, `w:styleId="Hyperlink"`) {
		t.Error("the Hyperlink character style is not defined")
	}
}

func TestDocxEmbedsImages(t *testing.T) {
	dir := t.TempDir()
	writePNGFixture(t, dir, "shot.png", 200, 80)

	doc := exportDocument{
		Title:      "With a picture",
		ExportedAt: "2026-01-01T00:00:00Z",
		Messages:   []exportMessage{{Role: "assistant", Content: "![a screenshot](shot.png)"}},
		assetsDir:  dir,
	}
	body, err := renderDOCXExport(doc)
	if err != nil {
		t.Fatalf("renderDOCXExport: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("DOCX is not a valid zip: %v", err)
	}

	var media bool
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "word/media/") {
			media = true
		}
	}
	if !media {
		t.Error("no media part was written for the embedded image")
	}
	var document, types, rels string
	for name, out := range map[string]*string{
		"word/document.xml":            &document,
		"[Content_Types].xml":          &types,
		"word/_rels/document.xml.rels": &rels,
	} {
		if err := readDocxPart(zr, name, out); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
	}
	if !strings.Contains(document, "<w:drawing>") {
		t.Error("the document body draws no picture")
	}
	if !strings.Contains(document, `xmlns:pic=`) {
		t.Error("the drawing namespaces are not declared")
	}
	if !strings.Contains(types, `<Default Extension="png" ContentType="image/png"/>`) {
		t.Errorf("png is not declared as a content type: %s", types)
	}
	if !strings.Contains(rels, "media/image1.png") {
		t.Errorf("the media part has no relationship: %s", rels)
	}
}

// A picture the server cannot read locally must not break the export; it falls
// back to a caption and a link.
func TestDocxDegradesUnresolvableImages(t *testing.T) {
	doc := exportDocument{
		Title:      "Remote picture",
		ExportedAt: "2026-01-01T00:00:00Z",
		Messages:   []exportMessage{{Role: "assistant", Content: "![a chart](https://example.com/chart.png)"}},
		assetsDir:  t.TempDir(),
	}
	body, err := renderDOCXExport(doc)
	if err != nil {
		t.Fatalf("renderDOCXExport: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	var document string
	if err := readDocxPart(zr, "word/document.xml", &document); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(document, "<w:drawing>") {
		t.Error("a remote image must not be fetched and embedded")
	}
	if !strings.Contains(docxPlainText(document), "a chart") {
		t.Error("the caption did not survive as the fallback")
	}
}

// Strikethrough, code colours and thematic breaks all have direct
// WordprocessingML forms; losing them silently is what made the old export read
// as flat prose.
func TestDocxRendersInlineDetails(t *testing.T) {
	d := newDocxDoc()
	var body strings.Builder
	for _, b := range markdownToBlocks("~~gone~~ and text\n\n---\n\n```go\npackage main\n```", nil) {
		body.WriteString(d.blockXML(b, false))
	}
	out := body.String()
	if !strings.Contains(out, "<w:strike/>") {
		t.Error("strikethrough was lost")
	}
	if !strings.Contains(out, "<w:pBdr><w:bottom") {
		t.Error("the thematic break drew no rule")
	}
	if !strings.Contains(out, "<w:color w:val=") {
		t.Error("the code block carries no syntax colours")
	}
}

// writePNGFixture writes a small opaque PNG into dir so image-embedding tests
// have something real to decode.
func writePNGFixture(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
