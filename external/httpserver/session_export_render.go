//go:build http

package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

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
			// The parent *List holds the ordered/bullet flag.
			ordered := false
			if lst, ok := v.Parent().(*gast.List); ok && lst.IsOrdered() {
				ordered = true
			}
			blocks = append(blocks, exportBlock{
				kind:    "list_item",
				ordered: ordered,
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

func renderPDFExport(doc exportDocument) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(doc.Title, true)
	pdf.SetAuthor("FoxxyCode", true)
	pdf.SetAutoPageBreak(true, 18)
	pdf.SetMargins(18, 18, 18)
	// Embed the Unicode-capable DejaVu Sans cuts so Cyrillic (and any other
	// non-Latin-1 code point) renders instead of panicking in fpdf's width
	// table. Bold maps to the bold cut; italic styles fall back to the regular
	// cut because we deliberately ship only regular + bold to bound size.
	pdf.AddUTF8FontFromBytes("DejaVu", "", dejavuSansRegular)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", dejavuSansBold)
	pdf.AddUTF8FontFromBytes("DejaVu", "I", dejavuSansRegular)
	pdf.AddUTF8FontFromBytes("DejaVu", "BI", dejavuSansBold)
	pdf.AddPage()

	// Document title.
	pdf.SetFont("DejaVu", "B", 18)
	pdf.MultiCell(174, 9, doc.Title, "", "L", false)
	pdf.Ln(2)
	pdf.SetFont("DejaVu", "", 8)
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
	pdf.SetFont("DejaVu", "B", 12)
	pdf.SetTextColor(r, g, b)
	display := label
	if label != "" {
		display = strings.ToUpper(label[:1]) + label[1:]
	}
	pdf.MultiCell(174, 6, display, "", "L", false)
	pdf.SetTextColor(0, 0, 0)
}

// writeReasoningPDF renders reasoning in a tinted, italic block.
func writeReasoningPDF(pdf *fpdf.Fpdf, md string) {
	pdf.SetFont("DejaVu", "I", 10)
	pdf.SetTextColor(90, 90, 90)
	for _, line := range strings.Split(md, "\n") {
		if strings.TrimSpace(line) == "" {
			pdf.Ln(2)
			continue
		}
		writeWrapped(pdf, "DejaVu", "I", 10, line)
	}
	pdf.SetTextColor(0, 0, 0)
}

func writeBlocksPDF(pdf *fpdf.Fpdf, blocks []exportBlock) {
	for _, b := range blocks {
		switch b.kind {
		case "heading":
			size := 16 - float64(b.level)*1.5
			if size < 10 {
				size = 10
			}
			writeRunsPDF(pdf, b.runs, "DejaVu", "B", size)
			pdf.Ln(1)
		case "code_block":
			// DejaVu Sans is proportional, not monospace, but it carries the
			// full code-point range so code containing non-Latin comments or
			// strings still exports instead of crashing.
			pdf.SetFont("DejaVu", "", 9)
			pdf.SetFillColor(246, 248, 250)
			for _, line := range strings.Split(b.text, "\n") {
				writeWrappedFill(pdf, "DejaVu", "", 9, line)
			}
			pdf.Ln(1)
		case "list_item":
			marker := "•  "
			if b.ordered {
				marker = "–  "
			}
			writeRunsPDF(pdf, prependRun(b.runs, exportRun{text: marker}), "DejaVu", "", 11)
		case "quote":
			pdf.SetTextColor(110, 119, 129)
			writeRunsPDF(pdf, b.runs, "DejaVu", "I", 11)
			pdf.SetTextColor(0, 0, 0)
		default: // paragraph / text
			writeRunsPDF(pdf, b.runs, "DejaVu", "", 11)
		}
	}
}

func prependRun(runs []exportRun, r exportRun) []exportRun {
	out := make([]exportRun, 0, len(runs)+1)
	out = append(out, r)
	return append(out, runs...)
}

// writeRunsPDF emits a single paragraph made of formatted runs, wrapping at
// the right margin. fpdf has no rich-text MultiCell, so we segment by run and
// rely on word wrapping for the common case.
func writeRunsPDF(pdf *fpdf.Fpdf, runs []exportRun, family, style string, size float64) {
	x0 := pdf.GetX()
	for _, r := range runs {
		if r.text == "" {
			continue
		}
		f := family
		s := style
		fs := size
		if r.bold {
			s = "B"
		} else if r.italic {
			s = "I"
		}
		if r.code {
			// Inline code stays on the proportional DejaVu family (see the note
			// in writeBlocksPDF on why we do not switch to a core mono font).
			fs = size - 1
		}
		writeWrapped(pdf, f, s, fs, r.text)
	}
	// New line after the paragraph unless MultiCell already advanced.
	if pdf.GetX() > x0 {
		pdf.Ln(size * 0.5)
	}
}

func writeWrapped(pdf *fpdf.Fpdf, family, style string, size float64, text string) {
	pdf.SetFont(family, style, size)
	width, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	avail := width - left - right
	for _, line := range pdf.SplitText(text, avail) {
		pdf.MultiCell(avail, size*0.42, line, "", "L", false)
	}
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
		return docxParagraph(docxRuns(b.runs, reasoning), "Heading"+itoa(b.level))
	case "code_block":
		var sb strings.Builder
		for _, line := range strings.Split(b.text, "\n") {
			sb.WriteString(docxParagraph(docxRun(line, false, false, true), "Code"))
		}
		return sb.String()
	case "list_item":
		runs := append([]exportRun{{text: "•  "}}, b.runs...)
		return docxListParagraph(docxRuns(runs, reasoning))
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

func docxListParagraph(runsXML string) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>%s</w:p>`, runsXML)
}

func docxEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
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
// body (Title, Heading1-4, Code, Quote, ListParagraph). Each carries concrete
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
<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:basedOn w:val="Normal"/><w:pPr><w:shd w:val="clear" w:color="auto" w:fill="F6F8FA"/><w:spacing w:after="0"/></w:pPr><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="19"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="80" w:after="80"/><w:ind w:left="360"/></w:pPr><w:rPr><w:i/><w:color w:val="6E7781"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="360"/></w:pPr></w:style>
</w:styles>`

// numberingXML defines the bullet list (numId=1) used by ListParagraph items.
// A single abstractNum with a bullet mapping covers all list items the export
// emits; ordered lists currently reuse the same bullet glyph.
const numberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:abstractNum w:abstractNumId="0">
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="360" w:hanging="360"/></w:pPr></w:lvl>
</w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
</w:numbering>`
