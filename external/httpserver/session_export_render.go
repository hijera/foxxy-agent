//go:build http

package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// This file renders a session transcript into four downloadable formats:
// JSON, HTML, PDF, and DOCX. The dialogue surface is the user/assistant
// turns plus any assistant reasoning blocks; tool/system rows are skipped so
// the exported document reads as a conversation. Markdown in assistant (and
// user) content is parsed once via goldmark into a small block/inline model,
// then each document renderer (PDF/DOCX) walks that model so formatting
// (headings, lists, code, emphasis) is preserved across every format.

// exportFormat is one of the supported render targets.
type exportFormat string

const (
	exportJSON exportFormat = "json"
	exportHTML exportFormat = "html"
	exportPDF  exportFormat = "pdf"
	exportDOCX exportFormat = "docx"
)

// exportRun is an inline text span with optional formatting.
type exportRun struct {
	text   string
	bold   bool
	italic bool
	code   bool
}

// exportBlock is a single rendered markdown block (paragraph, heading, list
// item, fenced code, or quote). Headings carry their level; code blocks carry
// raw text; everything else is a sequence of inline runs.
type exportBlock struct {
	kind    string // "heading" | "paragraph" | "list_item" | "code_block" | "quote"
	level   int    // heading level 1..6
	runs    []exportRun
	text    string // raw text for code blocks
	ordered bool   // list item ordered vs bullet
	number  int    // 1-based ordinal of an ordered list item
}

// exportMessage is one turn in the exported dialogue.
type exportMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// exportDocument is the structured payload rendered into every format.
type exportDocument struct {
	SessionID  string          `json:"session_id"`
	Title      string          `json:"title"`
	ExportedAt string          `json:"exported_at"`
	Messages   []exportMessage `json:"messages"`
}

// buildExportDocument filters the persisted transcript down to the dialogue
// surface and tags it with metadata. Only user and assistant roles are kept;
// for assistant messages a non-empty reasoning block is exported as its own
// labeled turn so the reader can follow the model's thinking.
func buildExportDocument(sessionID, title string, msgs []llm.Message) exportDocument {
	out := exportDocument{
		SessionID:  sessionID,
		Title:      title,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Messages:   make([]exportMessage, 0, len(msgs)),
	}
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			out.Messages = append(out.Messages, exportMessage{
				Role:      "user",
				Content:   m.Content,
				CreatedAt: m.CreatedAt,
			})
		case llm.RoleAssistant:
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.Reasoning) == "" {
				continue
			}
			out.Messages = append(out.Messages, exportMessage{
				Role:      "assistant",
				Content:   m.Content,
				Reasoning: strings.TrimSpace(m.Reasoning),
				CreatedAt: m.CreatedAt,
			})
		default:
			// system / tool rows are not part of the exported conversation.
		}
	}
	return out
}

// markdownToBlocks parses a markdown string into the block/inline model shared
// by the PDF and DOCX renderers. HTML uses goldmark's own renderer; this model
// exists for the document formats that cannot consume HTML directly.
func markdownToBlocks(md string) []exportBlock {
	if strings.TrimSpace(md) == "" {
		return nil
	}
	source := []byte(md)
	reader := text.NewReader(source)
	doc := goldmark.New().Parser().Parse(reader)
	var blocks []exportBlock
	if err := gast.Walk(doc, func(n gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *gast.Heading:
			blocks = append(blocks, exportBlock{
				kind:  "heading",
				level: v.Level,
				runs:  collectInline(v, source),
			})
			return gast.WalkSkipChildren, nil
		case *gast.Paragraph:
			blocks = append(blocks, exportBlock{
				kind: "paragraph",
				runs: collectInline(v, source),
			})
			return gast.WalkSkipChildren, nil
		case *gast.ListItem:
			// The parent *List holds the ordered/bullet flag and, for ordered
			// lists, the first ordinal; the item's position supplies the rest.
			ordered, number := false, 0
			if lst, ok := v.Parent().(*gast.List); ok && lst.IsOrdered() {
				ordered = true
				number = lst.Start
				if number == 0 {
					number = 1
				}
				for prev := v.PreviousSibling(); prev != nil; prev = prev.PreviousSibling() {
					number++
				}
			}
			blocks = append(blocks, exportBlock{
				kind:    "list_item",
				ordered: ordered,
				number:  number,
				runs:    collectInline(v, source),
			})
			return gast.WalkSkipChildren, nil
		case *gast.FencedCodeBlock:
			blocks = append(blocks, exportBlock{
				kind: "code_block",
				text: codeBlockText(v, source),
			})
			return gast.WalkSkipChildren, nil
		case *gast.CodeBlock:
			blocks = append(blocks, exportBlock{
				kind: "code_block",
				text: codeBlockText(v, source),
			})
			return gast.WalkSkipChildren, nil
		case *gast.Blockquote:
			blocks = append(blocks, exportBlock{
				kind: "quote",
				runs: collectInline(v, source),
			})
			return gast.WalkSkipChildren, nil
		case *gast.TextBlock:
			// Loose paragraph text (e.g. inside list items handled above). Treat
			// as a plain paragraph so the content still renders.
			blocks = append(blocks, exportBlock{
				kind: "paragraph",
				runs: collectInline(v, source),
			})
			return gast.WalkSkipChildren, nil
		}
		return gast.WalkContinue, nil
	}); err != nil {
		// goldmark's Walk only forwards walker errors; our walker never returns
		// one, so this is defensive only.
		return blocks
	}
	return blocks
}

func codeBlockText(n gast.Node, source []byte) string {
	var sb strings.Builder
	// Fenced and indented code blocks store their content in Lines() (a slice of
	// source segments), not in child *gast.Text nodes — the inline walker path
	// is for paragraphs, not code.
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i) // value receiver; Value needs an addressable segment
		sb.Write(seg.Value(source))
	}
	// Some code blocks (e.g. html-as-code) carry trailing inline children too;
	// fold them in for completeness.
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*gast.Text); ok {
			sb.Write(t.Segment.Value(source))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// collectInline walks the inline children of a block into formatted runs.
func collectInline(n gast.Node, source []byte) []exportRun {
	var runs []exportRun
	var walk func(n gast.Node, bold, italic, code bool)
	walk = func(node gast.Node, bold, italic, code bool) {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *gast.Text:
				runs = append(runs, exportRun{
					text:   string(v.Segment.Value(source)),
					bold:   bold,
					italic: italic,
					code:   code,
				})
			case *gast.CodeSpan:
				walk(v, bold, italic, true)
			case *gast.Emphasis:
				if v.Level == 2 {
					walk(v, true, italic, code)
				} else {
					walk(v, bold, true, code)
				}
			case *gast.Link:
				// Render link text; the URL is dropped in document formats to
				// keep inline runs readable.
				walk(v, bold, italic, code)
			case *gast.Image:
				walk(v, bold, italic, code)
			default:
				walk(c, bold, italic, code)
			}
		}
	}
	walk(n, false, false, false)
	// Coalesce adjacent runs with identical formatting so the renderers emit
	// fewer spans without changing output.
	out := runs[:0]
	for _, r := range runs {
		if len(out) > 0 {
			last := &out[len(out)-1]
			if last.bold == r.bold && last.italic == r.italic && last.code == r.code {
				last.text += r.text
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// --- JSON -----------------------------------------------------------------

func renderJSONExport(doc exportDocument) ([]byte, error) {
	return json.MarshalIndent(doc, "", "  ")
}

// --- HTML -----------------------------------------------------------------

const htmlExportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
  body { font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; max-width: 820px; margin: 32px auto; padding: 0 16px; color: #1f2328; }
  h1 { font-size: 1.7em; border-bottom: 1px solid #d0d7de; padding-bottom: .3em; }
  .turn { margin: 1.5em 0; }
  .turn-role { font-weight: 600; text-transform: capitalize; margin: 0 0 .25em; }
  .turn-role.user { color: #0969da; }
  .turn-role.assistant { color: #1a7f37; }
  .turn-role.reasoning { color: #6e7781; font-style: italic; }
  .turn-body { border-left: 3px solid #d0d7de; padding-left: 12px; }
  .turn-body.reasoning { border-left-color: #d0bc00; background: #fffdf0; padding: 8px 12px; border-radius: 4px; }
  pre, code { font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace; }
  pre { background: #f6f8fa; padding: 12px; border-radius: 6px; overflow-x: auto; }
  code { background: #f6f8fa; padding: .15em .35em; border-radius: 4px; }
  pre code { background: none; padding: 0; }
  table { border-collapse: collapse; }
  th, td { border: 1px solid #d0d7de; padding: 6px 12px; }
  blockquote { color: #6e7781; border-left: 3px solid #d0d7de; margin: 0; padding-left: 12px; }
  .meta { color: #6e7781; font-size: .85em; margin-top: .2em; }
</style>
</head>
<body>
<h1>{{.Title}}</h1>
{{.Rows}}
</body>
</html>`

func renderHTMLExport(doc exportDocument) ([]byte, error) {
	// goldmark.New() already wires the default HTML renderer, which is what we
	// need to turn markdown fragments into HTML strings.
	md := goldmark.New()
	var rows strings.Builder
	for _, m := range doc.Messages {
		if m.Reasoning != "" {
			fmt.Fprintf(&rows, `<div class="turn"><p class="turn-role reasoning">Reasoning</p><div class="turn-body reasoning">%s</div></div>`+"\n", markdownToHTML(md, m.Reasoning))
		}
		extra := ""
		if m.CreatedAt != "" {
			extra = fmt.Sprintf(` <span class="meta">%s</span>`, template.HTMLEscapeString(m.CreatedAt))
		}
		fmt.Fprintf(&rows, `<div class="turn"><p class="turn-role %s">%s%s</p><div class="turn-body">%s</div></div>`+"\n",
			template.HTMLEscapeString(m.Role), template.HTMLEscapeString(m.Role), extra, markdownToHTML(md, m.Content))
	}
	var buf bytes.Buffer
	tmpl := template.Must(template.New("export").Parse(htmlExportTemplate))
	if err := tmpl.Execute(&buf, map[string]interface{}{
		"Title": doc.Title,
		// Rows is the goldmark-rendered HTML for the transcript; mark it as
		// trusted so html/template does not escape the tags goldmark produced.
		"Rows": template.HTML(rows.String()),
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// markdownToHTML renders a markdown fragment to an HTML string via goldmark.
func markdownToHTML(md goldmark.Markdown, source string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(source), &buf); err != nil {
		return template.HTMLEscapeString(source)
	}
	return buf.String()
}

// --- PDF ------------------------------------------------------------------

// exportPDFFont is the single family the PDF export draws with; all four style
// slots are registered so fpdf never falls back to a core Latin-1 font.
const exportPDFFont = "DejaVu"

// newExportPDF builds the page geometry and registers the embedded font cuts.
// Extracted so layout tests can exercise the block writers on the exact same
// document setup the real export uses.
func newExportPDF(title string) *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(title, true)
	pdf.SetAuthor("FoxxyCode", true)
	pdf.SetAutoPageBreak(true, 18)
	pdf.SetMargins(18, 18, 18)
	// Embed the Unicode-capable DejaVu Sans cuts so Cyrillic (and any other
	// non-Latin-1 code point) renders instead of panicking in fpdf's width
	// table. Bold maps to the bold cut; italic styles fall back to the regular
	// cut because we deliberately ship only regular + bold to bound size.
	pdf.AddUTF8FontFromBytes(exportPDFFont, "", dejavuSansRegular)
	pdf.AddUTF8FontFromBytes(exportPDFFont, "B", dejavuSansBold)
	pdf.AddUTF8FontFromBytes(exportPDFFont, "I", dejavuSansRegular)
	pdf.AddUTF8FontFromBytes(exportPDFFont, "BI", dejavuSansBold)
	return pdf
}

func renderPDFExport(doc exportDocument) ([]byte, error) {
	pdf := newExportPDF(doc.Title)
	pdf.AddPage()

	// Document title.
	pdf.SetFont(exportPDFFont, "B", 18)
	pdf.MultiCell(174, 9, doc.Title, "", "L", false)
	pdf.Ln(2)
	pdf.SetFont(exportPDFFont, "", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.MultiCell(174, 4, "Exported "+doc.ExportedAt, "", "L", false)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(3)

	for _, m := range doc.Messages {
		if m.Reasoning != "" {
			writeRoleLabel(pdf, "Reasoning", 110, 110, 110)
			writeReasoningPDF(pdf, m.Reasoning)
		}
		var color [3]int
		switch m.Role {
		case "user":
			color = [3]int{9, 105, 218}
		default:
			color = [3]int{26, 127, 55}
		}
		writeRoleLabel(pdf, m.Role, color[0], color[1], color[2])
		writeBlocksPDF(pdf, markdownToBlocks(m.Content))
		pdf.Ln(3)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeRoleLabel(pdf *fpdf.Fpdf, label string, r, g, b int) {
	pdf.Ln(1)
	pdf.SetFont(exportPDFFont, "B", 12)
	pdf.SetTextColor(r, g, b)
	display := label
	if label != "" {
		display = strings.ToUpper(label[:1]) + label[1:]
	}
	pdf.MultiCell(174, 6, display, "", "L", false)
	pdf.SetTextColor(0, 0, 0)
}

// writeReasoningPDF renders reasoning in a tinted, italic block. It goes
// through the same markdown pipeline as the answer so a multi-paragraph thought
// keeps its structure instead of being poured out as raw lines.
func writeReasoningPDF(pdf *fpdf.Fpdf, md string) {
	const size = 10.0
	pdf.SetTextColor(90, 90, 90)
	for _, b := range markdownToBlocks(md) {
		runs := b.runs
		switch b.kind {
		case "code_block":
			runs = []exportRun{{text: b.text, code: true}}
		case "list_item":
			runs = prependRun(runs, exportRun{text: listMarker(b)})
		}
		writeRunsPDF(pdf, runs, "I", size)
		pdf.Ln(size * paragraphGap)
	}
	pdf.SetTextColor(0, 0, 0)
}

// exportBodySize is the base point size for paragraph text; headings and code
// scale relative to it.
const exportBodySize = 11.0

// paragraphGap is the vertical breathing room left after a block, expressed as
// a fraction of the block's font size. It has to stay clearly below a full line
// advance so a gap never reads as an empty line.
const paragraphGap = 0.45

func writeBlocksPDF(pdf *fpdf.Fpdf, blocks []exportBlock) {
	for i, b := range blocks {
		switch b.kind {
		case "heading":
			size := 16 - float64(headingLevel(b.level))*1.5
			if size < 10 {
				size = 10
			}
			writeRunsPDF(pdf, b.runs, "B", size)
			pdf.Ln(size * paragraphGap)
		case "code_block":
			// DejaVu Sans is proportional, not monospace, but it carries the
			// full code-point range so code containing non-Latin comments or
			// strings still exports instead of crashing.
			pdf.SetFillColor(246, 248, 250)
			for _, line := range strings.Split(b.text, "\n") {
				writeWrappedFill(pdf, exportPDFFont, "", 9, line)
			}
			pdf.Ln(9 * paragraphGap)
		case "list_item":
			// List items stay tight against each other; the marker is part of
			// the flowed text because PDF has no list construct of its own. Only
			// the item that closes the list gets the block gap, so the list does
			// not run into the paragraph below it.
			writeRunsPDF(pdf, prependRun(b.runs, exportRun{text: listMarker(b)}), "", exportBodySize)
			if i+1 >= len(blocks) || blocks[i+1].kind != "list_item" {
				pdf.Ln(exportBodySize * paragraphGap)
			}
		case "quote":
			pdf.SetTextColor(110, 119, 129)
			writeRunsPDF(pdf, b.runs, "I", exportBodySize)
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(exportBodySize * paragraphGap)
		default: // paragraph / text
			writeRunsPDF(pdf, b.runs, "", exportBodySize)
			pdf.Ln(exportBodySize * paragraphGap)
		}
	}
}

// headingLevel clamps a markdown heading level into the 1..6 range the
// renderers have styles for.
func headingLevel(level int) int {
	if level < 1 {
		return 1
	}
	if level > 6 {
		return 6
	}
	return level
}

// listMarker is the glyph the PDF prints in front of a list item. Ordered items
// print their real ordinal; DOCX leaves this to the document numbering instead.
func listMarker(b exportBlock) string {
	if b.ordered && b.number > 0 {
		return fmt.Sprintf("%d.  ", b.number)
	}
	if b.ordered {
		return "1.  "
	}
	return "•  "
}

func prependRun(runs []exportRun, r exportRun) []exportRun {
	out := make([]exportRun, 0, len(runs)+1)
	out = append(out, r)
	return append(out, runs...)
}

// writeRunsPDF emits one paragraph built from formatted runs. fpdf has no
// rich-text cell, but Write flows text from the current position and wraps at
// the right margin, so switching the font between Write calls keeps every run
// on the same line instead of starting a new one per run (which MultiCell would
// do, shattering a formatted sentence across several lines).
func writeRunsPDF(pdf *fpdf.Fpdf, runs []exportRun, style string, size float64) {
	lineHeight := size * lineHeightRatio
	wrote := false
	for _, r := range runs {
		if r.text == "" {
			continue
		}
		fs := size
		if r.code {
			// Inline code stays on the proportional DejaVu family (see the note
			// in writeBlocksPDF on why we do not switch to a core mono font).
			fs = size - 1
		}
		pdf.SetFont(exportPDFFont, runStyle(style, r), fs)
		pdf.Write(lineHeight, r.text)
		wrote = true
	}
	if wrote {
		// Close the flowed line; the caller adds any inter-block spacing.
		pdf.Ln(lineHeight)
	}
}

// lineHeightRatio converts a point size into the millimetre line advance used
// throughout the PDF body.
const lineHeightRatio = 0.42

// runStyle merges the block's base style with the run's own emphasis so a bold
// heading keeps its weight and an italic quote keeps its slant.
func runStyle(base string, r exportRun) string {
	bold := strings.Contains(base, "B") || r.bold
	italic := strings.Contains(base, "I") || r.italic
	style := ""
	if bold {
		style += "B"
	}
	if italic {
		style += "I"
	}
	return style
}

func writeWrappedFill(pdf *fpdf.Fpdf, family, style string, size float64, text string) {
	pdf.SetFont(family, style, size)
	width, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	avail := width - left - right
	lines := pdf.SplitText(text, avail)
	if len(lines) == 0 {
		lines = []string{""}
	}
	for _, line := range lines {
		pdf.MultiCell(avail, size*0.48, line, "", "L", true)
	}
}

// --- DOCX -----------------------------------------------------------------

// renderDOCXExport builds a minimal Office Open XML (.docx) package: a content
// type map, the package relationships, the document relationship, and the
// document body. Paragraphs use the same block/inline model as the PDF path.
func renderDOCXExport(doc exportDocument) ([]byte, error) {
	var body strings.Builder
	body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr>`)
	body.WriteString(docxRun(doc.Title, true, false, false))
	body.WriteString(`</w:p>`)
	body.WriteString(`<w:p><w:r><w:rPr><w:sz w:val="16"/><w:color w:val="787878"/></w:rPr><w:t xml:space="preserve">Exported `)
	body.WriteString(docxEscape(doc.ExportedAt))
	body.WriteString(`</w:t></w:r></w:p>`)

	for _, m := range doc.Messages {
		if m.Reasoning != "" {
			body.WriteString(docxParagraph(docxRun("Reasoning", true, true, false), "Heading4"))
			for _, b := range markdownToBlocks(m.Reasoning) {
				body.WriteString(docxBlockXML(b, true))
			}
		}
		body.WriteString(docxParagraph(docxRun(m.Role, true, false, false), "Heading3"))
		for _, b := range markdownToBlocks(m.Content) {
			body.WriteString(docxBlockXML(b, false))
		}
	}

	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>` + body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body>
</w:document>`

	var buf bytes.Buffer
	zw := newDocxWriter(&buf)
	for _, part := range []struct{ name, content string }{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", rootRelsXML},
		{"word/_rels/document.xml.rels", docRelsXML},
		{"word/document.xml", xml},
		{"word/styles.xml", stylesXML},
		{"word/numbering.xml", numberingXML},
	} {
		if err := zw.write(part.name, part.content); err != nil {
			return nil, err
		}
	}
	if err := zw.close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func docxBlockXML(b exportBlock, reasoning bool) string {
	switch b.kind {
	case "heading":
		return docxParagraph(docxRuns(b.runs, reasoning), "Heading"+itoa(headingLevel(b.level)))
	case "code_block":
		var sb strings.Builder
		for _, line := range strings.Split(b.text, "\n") {
			sb.WriteString(docxParagraph(docxRun(line, false, false, true), "Code"))
		}
		return sb.String()
	case "list_item":
		// Word draws the bullet or the number from numbering.xml; adding a
		// literal marker to the text would print it twice.
		return docxListParagraph(docxRuns(b.runs, reasoning), b.ordered)
	case "quote":
		quoted := append([]exportRun{{text: "“"}}, b.runs...)
		quoted = append(quoted, exportRun{text: "”"})
		return docxParagraph(docxRuns(quoted, true), "Quote")
	default:
		return docxParagraph(docxRuns(b.runs, reasoning), "")
	}
}

func docxRuns(runs []exportRun, italic bool) string {
	var sb strings.Builder
	for _, r := range runs {
		sb.WriteString(docxRun(r.text, r.bold, r.italic || italic, r.code))
	}
	return sb.String()
}

func docxRun(text string, bold, italic, code bool) string {
	var rpr strings.Builder
	rpr.WriteString("<w:rPr>")
	if bold {
		rpr.WriteString("<w:b/>")
	}
	if italic {
		rpr.WriteString("<w:i/>")
	}
	if code {
		rpr.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/>`)
	}
	rpr.WriteString("</w:rPr>")
	return fmt.Sprintf(`<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, rpr.String(), docxEscape(text))
}

func docxParagraph(runsXML, style string) string {
	pPr := ""
	if style != "" {
		pPr = fmt.Sprintf(`<w:pPr><w:pStyle w:val="%s"/></w:pPr>`, style)
	}
	return fmt.Sprintf("<w:p>%s%s</w:p>", pPr, runsXML)
}

// docxNumIDBullet and docxNumIDOrdered select the list definition from
// numbering.xml: an unordered bullet or a decimal sequence.
const (
	docxNumIDBullet  = 1
	docxNumIDOrdered = 2
)

func docxListParagraph(runsXML string, ordered bool) string {
	numID := docxNumIDBullet
	if ordered {
		numID = docxNumIDOrdered
	}
	return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="%d"/></w:numPr></w:pPr>%s</w:p>`, numID, runsXML)
}

var docxEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
)

// docxEscape prepares message text for an XML text node. Every run in the
// package goes through here, so this is also where characters XML 1.0 forbids
// are dropped: a pasted terminal snippet carries ANSI escapes and the odd NUL,
// and leaving them in word/document.xml yields a package Word and LibreOffice
// refuse to open.
func docxEscape(s string) string {
	return docxEscaper.Replace(sanitizeXMLText(s))
}

// sanitizeXMLText removes the code points that are illegal in XML 1.0 character
// data, keeping the three whitespace controls the spec allows.
func sanitizeXMLText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r < 0x20:
			return -1
		case r >= 0xD800 && r <= 0xDFFF, r == 0xFFFE, r == 0xFFFF:
			return -1
		case r == utf8.RuneError:
			// strings.Map reports malformed input as RuneError; dropping it keeps
			// the document well-formed at the cost of a replacement glyph.
			return -1
		}
		return r
	}, s)
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>
</Types>`

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

// docRelsXML links the document to its supporting parts. styles and numbering
// are referenced by the document body, so the relationships must exist.
const docRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>
</Relationships>`

// stylesXML defines the named paragraph styles referenced from the document
// body (Title, Heading1-6, Code, Quote, ListParagraph). Markdown reaches down to
// six heading levels, so all six are defined here; a body that names a style the
// sheet omits silently falls back to Normal in Word.  Each carries concrete
// formatting so Word/LibreOffice render the export close to the HTML version
// without relying on built-in defaults that differ per application.
const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/></w:rPr></w:rPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="160"/></w:pPr><w:rPr><w:b/><w:sz w:val="44"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="80"/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="200" w:after="60"/></w:pPr><w:rPr><w:b/><w:sz w:val="28"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="160" w:after="40"/></w:pPr><w:rPr><w:b/><w:color w:val="1A7F37"/><w:sz w:val="25"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading4"><w:name w:val="heading 4"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="120" w:after="40"/></w:pPr><w:rPr><w:i/><w:color w:val="6E7781"/><w:sz w:val="23"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading5"><w:name w:val="heading 5"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="120" w:after="40"/></w:pPr><w:rPr><w:b/><w:sz w:val="22"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading6"><w:name w:val="heading 6"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="120" w:after="40"/></w:pPr><w:rPr><w:b/><w:i/><w:sz w:val="22"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:basedOn w:val="Normal"/><w:pPr><w:shd w:val="clear" w:color="auto" w:fill="F6F8FA"/><w:spacing w:after="0"/></w:pPr><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="19"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="80" w:after="80"/><w:ind w:left="360"/></w:pPr><w:rPr><w:i/><w:color w:val="6E7781"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="360"/></w:pPr></w:style>
</w:styles>`

// numberingXML defines the two list definitions ListParagraph items point at:
// numId=1 draws a bullet, numId=2 an incrementing decimal. Word renders the
// marker itself from here, so the document body must not repeat it in the run
// text — see docxBlockXML.
const numberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:abstractNum w:abstractNumId="0">
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="360" w:hanging="360"/></w:pPr></w:lvl>
</w:abstractNum>
<w:abstractNum w:abstractNumId="1">
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="360" w:hanging="360"/></w:pPr></w:lvl>
</w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
<w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>
</w:numbering>`
