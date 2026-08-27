//go:build http

package httpserver

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// The DOCX export writes a minimal Office Open XML package by hand: a content
// type map, the package relationships, the document relationships, the document
// body, a style sheet and a numbering definition, plus one media part per
// embedded picture. DOCX is a zip of well-known XML parts, so no library is
// needed beyond archive/zip.
//
// Paragraph content comes from the same block/inline model the PDF walks, so a
// construct is either supported in both or in neither.

// docxWriter wraps a zip.Writer that streams an OOXML package into an io.Writer.
type docxWriter struct {
	w *zip.Writer
}

func newDocxWriter(w io.Writer) *docxWriter {
	return &docxWriter{w: zip.NewWriter(w)}
}

func (d *docxWriter) write(name string, content []byte) error {
	fw, err := d.w.Create(name)
	if err != nil {
		return err
	}
	_, err = fw.Write(content)
	return err
}

func (d *docxWriter) writeString(name, content string) error {
	return d.write(name, []byte(content))
}

func (d *docxWriter) close() error {
	return d.w.Close()
}

// Page geometry, in twentieths of a point (twips). A4 minus one-inch margins is
// the width every table and picture is fitted into.
const (
	docxPageWidth    = 11906
	docxPageMargin   = 1440
	docxContentWidth = docxPageWidth - 2*docxPageMargin

	// docxEMUPerPixel converts a 96 dpi pixel (the web default, and what a
	// screenshot reports) into English Metric Units, the only length a drawing
	// understands.
	docxEMUPerPixel = 9525

	// docxContentWidthEMU is docxContentWidth expressed in EMU.
	docxContentWidthEMU = docxContentWidth * 635 // 1 twip = 635 EMU

	// docxMaxImageHeightEMU keeps a tall screenshot from swallowing a whole page.
	docxMaxImageHeightEMU = 6 * 914400 // 6 inches

	// docxTableCellMargin is the left/right padding declared by the ExportTable
	// style; column measurement has to leave room for it.
	docxTableCellMargin = 108

	// docxMinColumnWidth keeps even an empty column wide enough to click into.
	docxMinColumnWidth = 700
)

// docxRel is one entry of word/_rels/document.xml.rels beyond the fixed pair.
type docxRel struct {
	id         string
	relType    string
	target     string
	targetMode string
}

// docxMedia is one picture stored under word/media/.
type docxMedia struct {
	name string
	data []byte
}

// docxDoc accumulates a document body together with the relationships and media
// parts it references. Hyperlinks and pictures both need a relationship id, so
// the body cannot be a pure string transformation the way it used to be.
type docxDoc struct {
	rels     []docxRel
	media    []docxMedia
	exts     map[string]bool // image extensions needing a content-type default
	nextRel  int
	nextDraw int
}

func newDocxDoc() *docxDoc {
	// rId1 and rId2 are taken by the style sheet and the numbering definition.
	return &docxDoc{exts: map[string]bool{}, nextRel: 3, nextDraw: 1}
}

// addRel registers a relationship and returns its id.
func (d *docxDoc) addRel(relType, target, targetMode string) string {
	id := fmt.Sprintf("rId%d", d.nextRel)
	d.nextRel++
	d.rels = append(d.rels, docxRel{id: id, relType: relType, target: target, targetMode: targetMode})
	return id
}

// addImage stores an image part and returns the relationship id pointing at it.
func (d *docxDoc) addImage(img *exportImage) string {
	ext := imageExt(img.mime)
	name := fmt.Sprintf("image%d.%s", len(d.media)+1, ext)
	d.media = append(d.media, docxMedia{name: "word/media/" + name, data: img.data})
	d.exts[ext] = true
	return d.addRel("http://schemas.openxmlformats.org/officeDocument/2006/relationships/image", "media/"+name, "")
}

func renderDOCXExport(doc exportDocument) ([]byte, error) {
	d := newDocxDoc()
	media := doc.media()

	var body strings.Builder
	body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr>`)
	body.WriteString(docxRun(doc.Title, exportRun{bold: true}))
	body.WriteString(`</w:p>`)
	body.WriteString(`<w:p><w:r><w:rPr><w:color w:val="787878"/><w:sz w:val="16"/></w:rPr><w:t xml:space="preserve">Exported `)
	body.WriteString(docxEscape(doc.ExportedAt))
	body.WriteString(`</w:t></w:r></w:p>`)

	for _, m := range doc.Messages {
		if m.Reasoning != "" {
			body.WriteString(docxParagraph(docxRun("Reasoning", exportRun{bold: true, italic: true}), "Heading4"))
			for _, b := range markdownToBlocks(m.Reasoning, media) {
				body.WriteString(d.blockXML(b, true))
			}
		}
		body.WriteString(docxParagraph(docxRun(exportRoleLabel(m.Role), exportRun{bold: true}), docxRoleStyle(m.Role)))
		for _, b := range markdownToBlocks(m.Content, media) {
			body.WriteString(d.blockXML(b, false))
		}
		body.WriteString(d.attachmentsXML(m.Attachments, media))
	}

	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document ` + docxNamespaces + `>
<w:body>` + body.String() +
		`<w:sectPr><w:pgSz w:w="` + itoa(docxPageWidth) + `" w:h="16838"/>` +
		`<w:pgMar w:top="` + itoa(docxPageMargin) + `" w:right="` + itoa(docxPageMargin) +
		`" w:bottom="` + itoa(docxPageMargin) + `" w:left="` + itoa(docxPageMargin) + `"/></w:sectPr></w:body>
</w:document>`

	var buf bytes.Buffer
	zw := newDocxWriter(&buf)
	for _, part := range []struct{ name, content string }{
		{"[Content_Types].xml", d.contentTypesXML()},
		{"_rels/.rels", rootRelsXML},
		{"word/_rels/document.xml.rels", d.docRelsXML()},
		{"word/document.xml", document},
		{"word/styles.xml", stylesXML},
		{"word/numbering.xml", numberingXML},
	} {
		if err := zw.writeString(part.name, part.content); err != nil {
			return nil, err
		}
	}
	for _, m := range d.media {
		if err := zw.write(m.name, m.data); err != nil {
			return nil, err
		}
	}
	if err := zw.close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// docxRoleStyle names the heading style for a turn, so the reader can tell the
// two speakers apart at a glance the way they can in the chat window and in the
// other two formats.
func docxRoleStyle(role string) string {
	if role == "user" {
		return "RoleUser"
	}
	return "RoleAssistant"
}

// blockXML renders one block. reasoning italicises everything, which is how the
// reasoning turn is set apart from the answer.
func (d *docxDoc) blockXML(b exportBlock, reasoning bool) string {
	switch b.kind {
	case blockHeading:
		return d.paragraph(b, d.runsXML(b.runs, reasoning), "Heading"+itoa(headingLevel(b.level)))
	case blockCode:
		return d.codeXML(b)
	case blockTable:
		return d.tableXML(b)
	case blockRule:
		return `<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="D0D7DE"/></w:pBdr></w:pPr></w:p>`
	case blockImage:
		return d.imageXML(b.image)
	case blockListItem:
		return d.listXML(b, reasoning)
	default:
		style := ""
		if b.quote > 0 {
			style = "Quote"
		}
		return d.paragraph(b, d.runsXML(b.runs, reasoning || b.quote > 0), style)
	}
}

// paragraph wraps runs in a <w:p>, carrying the block's nesting indent and, for
// a quoted block, the Quote style's left rule.
func (d *docxDoc) paragraph(b exportBlock, runsXML, style string) string {
	if b.quote > 0 && style == "" {
		style = "Quote"
	}
	return docxParagraphIndented(runsXML, style, docxIndent(b))
}

// docxIndent is the left indent a block inherits from its list and quote depth,
// in twips. Word's own list indent covers the first list level, so only the
// levels beyond it are added here.
func docxIndent(b exportBlock) int {
	return (b.indent + b.quote) * 360
}

// listXML renders a list item. Bullets and numbers come from numbering.xml, so
// the run text must not repeat them — except for a task item, which Word has no
// native form for and which therefore carries its own box glyph instead of a
// numbering reference.
func (d *docxDoc) listXML(b exportBlock, reasoning bool) string {
	runs := d.runsXML(b.runs, reasoning)
	if b.checked != nil {
		glyph := "☐  " // empty ballot box
		if *b.checked {
			glyph = "☑  " // ballot box with check
		}
		// Hanging indent so the box lands where a bullet would, instead of one
		// step further in than the list it sits beside.
		return docxHangingParagraph(docxRun(glyph, exportRun{})+runs, "ListParagraph", 360+b.indent*360)
	}
	numID := docxNumIDBullet
	if b.ordered {
		numID = docxNumIDOrdered
	}
	level := b.indent
	if level > docxMaxListLevel {
		level = docxMaxListLevel
	}
	return fmt.Sprintf(
		`<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr></w:pPr>%s</w:p>`,
		level, numID, runs)
}

// codeXML renders a fenced block as one shaded, left-ruled paragraph per line.
// A single-cell table would draw a tidier box, but a table cannot sit inside a
// list item, and code inside a list is common in a transcript.
func (d *docxDoc) codeXML(b exportBlock) string {
	var sb strings.Builder
	indent := docxIndent(b)
	for _, line := range codeLinesOf(b) {
		var runs strings.Builder
		for _, r := range line {
			r.code = true
			runs.WriteString(docxRun(r.text, r))
		}
		if runs.Len() == 0 {
			// A blank line still needs a paragraph or the box would close early.
			runs.WriteString(docxRun(" ", exportRun{code: true}))
		}
		sb.WriteString(docxParagraphIndented(runs.String(), "Code", indent))
	}
	return sb.String()
}

// tableXML renders a GFM table as a real Word table: fixed layout over measured
// column widths, shaded header repeated across page breaks, per-column
// alignment.
func (d *docxDoc) tableXML(b exportBlock) string {
	tbl := b.table
	if tbl == nil {
		return ""
	}
	cols := tableColumnCount(tbl)
	if cols == 0 {
		return ""
	}
	widths := docxColumnWidths(tbl, cols)

	var sb strings.Builder
	sb.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="ExportTable"/><w:tblW w:w="` + itoa(docxContentWidth) + `" w:type="dxa"/>`)
	sb.WriteString(`<w:tblBorders>`)
	for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		sb.WriteString(`<w:` + edge + ` w:val="single" w:sz="4" w:space="0" w:color="D0D7DE"/>`)
	}
	sb.WriteString(`</w:tblBorders><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	for _, w := range widths {
		sb.WriteString(`<w:gridCol w:w="` + itoa(w) + `"/>`)
	}
	sb.WriteString(`</w:tblGrid>`)

	if len(tbl.header) > 0 {
		sb.WriteString(d.tableRowXML(tbl, tbl.header, widths, cols, true))
	}
	for _, row := range tbl.rows {
		sb.WriteString(d.tableRowXML(tbl, row, widths, cols, false))
	}
	sb.WriteString(`</w:tbl>`)
	// Word and LibreOffice both want a paragraph after a table; without it two
	// adjacent tables merge and a trailing table can leave the file unopenable.
	sb.WriteString(`<w:p/>`)
	return sb.String()
}

func (d *docxDoc) tableRowXML(tbl *exportTable, row []exportTableCell, widths []int, cols int, header bool) string {
	var sb strings.Builder
	sb.WriteString(`<w:tr>`)
	if header {
		// Repeat the header when the table spills onto the next page.
		// Schema order inside w:trPr is cantSplit before tblHeader; Word repairs
		// (and silently drops) a table whose properties arrive out of order.
		sb.WriteString(`<w:trPr><w:cantSplit/><w:tblHeader/></w:trPr>`)
	}
	for i := 0; i < cols; i++ {
		sb.WriteString(`<w:tc><w:tcPr><w:tcW w:w="` + itoa(widths[i]) + `" w:type="dxa"/>`)
		if header {
			sb.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="F6F8FA"/>`)
		}
		sb.WriteString(`</w:tcPr>`)

		var runs string
		if i < len(row) {
			// The bold goes on the runs themselves: a paragraph mark's rPr styles
			// only the mark.
			runs = d.runsForced(row[i].runs, header, false)
		}
		// w:pPr child order: pStyle, then jc, then rPr last.
		pPr := `<w:pPr><w:pStyle w:val="TableCell"/>` + docxJustify(tableAlign(tbl, i))
		if header {
			pPr += `<w:rPr><w:b/></w:rPr>`
		}
		pPr += `</w:pPr>`
		sb.WriteString(`<w:p>` + pPr + runs + `</w:p></w:tc>`)
	}
	sb.WriteString(`</w:tr>`)
	return sb.String()
}

// docxJustify maps a column alignment onto a paragraph justification. Left is
// Word's default and needs no element.
func docxJustify(align string) string {
	switch align {
	case "C":
		return `<w:jc w:val="center"/>`
	case "R":
		return `<w:jc w:val="right"/>`
	}
	return ""
}

// docxCharWidth is a rough twips-per-character estimate for the table body
// font. Word does the real measuring; this only has to be close enough to stop
// a column being sized so small that a word breaks across lines.
const docxCharWidth = 120

// docxColumnWidths splits the text column between the table's columns in
// proportion to the longest cell each one holds, with a per-column floor wide
// enough for that column's longest single word. Without the word floor a header
// like "Format" sitting over short values got squeezed until it wrapped to
// "Form / at".
func docxColumnWidths(tbl *exportTable, cols int) []int {
	weights := make([]int, cols)
	floors := make([]int, cols)
	fairShare := docxContentWidth / cols

	note := func(cells []exportTableCell) {
		for i, c := range cells {
			if i >= cols {
				break
			}
			text := runsText(c.runs)
			if n := utf8.RuneCountInString(text); n > weights[i] {
				weights[i] = n
			}
			for _, word := range strings.Fields(text) {
				want := utf8.RuneCountInString(word)*docxCharWidth + 2*docxTableCellMargin
				if want > floors[i] {
					floors[i] = want
				}
			}
		}
	}
	note(tbl.header)
	for _, row := range tbl.rows {
		note(row)
	}

	total := 0
	for i := range weights {
		if weights[i] < 1 {
			weights[i] = 1
		}
		total += weights[i]
		// A single very long word must not claim the whole row on its own.
		if floors[i] > fairShare {
			floors[i] = fairShare
		}
		if floors[i] < docxMinColumnWidth {
			floors[i] = docxMinColumnWidth
		}
	}

	widths := make([]int, cols)
	assigned := 0
	for i := range widths {
		widths[i] = docxContentWidth * weights[i] / total
		if widths[i] < floors[i] {
			widths[i] = floors[i]
		}
		assigned += widths[i]
	}
	// Rounding and the floors both drift; settle the difference on the column
	// that can best absorb it so the row still spans exactly the text width.
	if assigned != docxContentWidth {
		widest := 0
		for i := range widths {
			if widths[i]-floors[i] > widths[widest]-floors[widest] {
				widest = i
			}
		}
		widths[widest] += docxContentWidth - assigned
		if widths[widest] < floors[widest] {
			widths[widest] = floors[widest]
		}
	}
	return widths
}

// imageXML places a picture inline, scaled to the text column. An image that
// could not be resolved locally degrades to its caption plus a link.
func (d *docxDoc) imageXML(img *exportImage) string {
	if img == nil {
		return ""
	}
	if !img.embeddable() {
		return docxParagraph(d.runsXML([]exportRun{{text: img.alt, italic: true, link: img.src}}, false), "")
	}
	relID := d.addImage(img)
	id := d.nextDraw
	d.nextDraw++

	cx := img.widthPx * docxEMUPerPixel
	cy := img.heightPx * docxEMUPerPixel
	if cx > docxContentWidthEMU {
		cy = cy * docxContentWidthEMU / cx
		cx = docxContentWidthEMU
	}
	if cy > docxMaxImageHeightEMU {
		cx = cx * docxMaxImageHeightEMU / cy
		cy = docxMaxImageHeightEMU
	}

	drawing := fmt.Sprintf(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:drawing>`+
		`<wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/>`+
		`<wp:docPr id="%d" name="Picture %d" descr="%s"/>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic><pic:nvPicPr><pic:cNvPr id="%d" name="Picture %d"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`,
		cx, cy, id, id, docxEscape(img.alt), id, id, relID, cx, cy)

	if img.alt != "" && img.alt != img.src {
		drawing += docxParagraph(docxRun(img.alt, exportRun{italic: true}), "Caption")
	}
	return drawing
}

// attachmentsXML renders the files uploaded on a turn.
func (d *docxDoc) attachmentsXML(atts []exportAttachment, media *exportMediaResolver) string {
	if len(atts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(docxParagraph(docxRun("Attachments", exportRun{bold: true}), "Heading5"))
	for _, a := range atts {
		img := &exportImage{alt: a.Name, src: a.Path}
		if media != nil {
			media.fill(img)
		}
		if img.embeddable() {
			sb.WriteString(d.imageXML(img))
			continue
		}
		sb.WriteString(d.listXML(exportBlock{kind: blockListItem, runs: []exportRun{{text: a.Name}}}, false))
	}
	return sb.String()
}

// --- runs -----------------------------------------------------------------

// runsXML renders inline runs, grouping consecutive runs that share a link
// destination into one <w:hyperlink> so a multi-word link stays one click
// target.
func (d *docxDoc) runsXML(runs []exportRun, italic bool) string {
	return d.runsForced(runs, false, italic)
}

// runsForced renders inline runs with bold and/or italic forced on, which is how
// a table header cell is set without rewriting the markup afterwards.
func (d *docxDoc) runsForced(runs []exportRun, bold, italic bool) string {
	var sb strings.Builder
	for i := 0; i < len(runs); {
		link := runs[i].link
		if link == "" {
			r := runs[i]
			r.bold = r.bold || bold
			r.italic = r.italic || italic
			sb.WriteString(docxRun(r.text, r))
			i++
			continue
		}
		j := i
		var group strings.Builder
		for ; j < len(runs) && runs[j].link == link; j++ {
			r := runs[j]
			r.bold = r.bold || bold
			r.italic = r.italic || italic
			// r.link stays set: the enclosing w:hyperlink carries the destination,
			// while the run keeps it only to pick up the Hyperlink character style.
			// Clearing it here left link text rendered as ordinary black prose.
			group.WriteString(docxRun(r.text, r))
		}
		relID := d.addRel(
			"http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink",
			link, "External")
		fmt.Fprintf(&sb, `<w:hyperlink r:id="%s">%s</w:hyperlink>`, relID, group.String())
		i = j
	}
	return sb.String()
}

// docxRun renders one formatted span. A run carrying a link but rendered outside
// a <w:hyperlink> still gets the Hyperlink character style, so a link inside a
// table cell or a caption looks like one.
func docxRun(text string, r exportRun) string {
	// w:rPr is a sequence, not a bag: its children must appear in schema order
	// (rStyle, rFonts, b, i, strike, color, sz). Word repairs a document that
	// gets this wrong, and repairing drops whole constructs.
	var rpr strings.Builder
	rpr.WriteString("<w:rPr>")
	if r.link != "" {
		rpr.WriteString(`<w:rStyle w:val="Hyperlink"/>`)
	}
	if r.code {
		rpr.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/>`)
	}
	if r.bold {
		rpr.WriteString("<w:b/>")
	}
	if r.italic {
		rpr.WriteString("<w:i/>")
	}
	if r.strike {
		rpr.WriteString("<w:strike/>")
	}
	if r.color != "" {
		rpr.WriteString(`<w:color w:val="` + docxEscape(r.color) + `"/>`)
	}
	if r.code {
		rpr.WriteString(`<w:sz w:val="18"/>`)
	}
	rpr.WriteString("</w:rPr>")
	props := rpr.String()
	if props == "<w:rPr></w:rPr>" {
		props = "" // an unformatted run needs no properties element
	}
	return fmt.Sprintf(`<w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>`, props, docxEscape(text))
}

func docxParagraph(runsXML, style string) string {
	return docxParagraphIndented(runsXML, style, 0)
}

// docxParagraphIndented emits a paragraph with an optional named style and an
// extra left indent in twips.
func docxParagraphIndented(runsXML, style string, indent int) string {
	return docxStyledParagraph(runsXML, style, indent, 0)
}

// docxHangingParagraph is docxParagraphIndented with the first line pulled back
// out to the margin, which is how a list marker sits beside its text.
func docxHangingParagraph(runsXML, style string, indent int) string {
	return docxStyledParagraph(runsXML, style, indent, 360)
}

func docxStyledParagraph(runsXML, style string, indent, hanging int) string {
	var pPr strings.Builder
	if style != "" {
		fmt.Fprintf(&pPr, `<w:pStyle w:val="%s"/>`, style)
	}
	if indent > 0 && hanging > 0 {
		fmt.Fprintf(&pPr, `<w:ind w:left="%d" w:hanging="%d"/>`, indent, hanging)
	} else if indent > 0 {
		fmt.Fprintf(&pPr, `<w:ind w:left="%d"/>`, indent)
	}
	if pPr.Len() == 0 {
		return "<w:p>" + runsXML + "</w:p>"
	}
	return "<w:p><w:pPr>" + pPr.String() + "</w:pPr>" + runsXML + "</w:p>"
}

// docxNumIDBullet and docxNumIDOrdered select the list definition from
// numbering.xml: an unordered bullet or a decimal sequence.
const (
	docxNumIDBullet  = 1
	docxNumIDOrdered = 2

	// docxMaxListLevel is the deepest level numbering.xml defines.
	docxMaxListLevel = 4
)

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

// --- package parts --------------------------------------------------------

// docxNamespaces are the namespace declarations word/document.xml needs. r is
// required by hyperlinks and image blips; wp / a / pic by the drawing markup.
const docxNamespaces = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
	`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

// contentTypesXML declares a default per file extension plus an override per
// XML part. Every embedded picture's extension has to appear here or Word
// rejects the package.
func (d *docxDoc) contentTypesXML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
`)
	exts := make([]string, 0, len(d.exts))
	for ext := range d.exts {
		exts = append(exts, ext)
	}
	sort.Strings(exts) // deterministic package bytes
	for _, ext := range exts {
		fmt.Fprintf(&sb, "<Default Extension=%q ContentType=\"image/%s\"/>\n", ext, ext)
	}
	sb.WriteString(`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>
</Types>`)
	return sb.String()
}

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

// docRelsXML links the document to its supporting parts. styles and numbering
// are referenced by the document body, so the relationships must exist; the
// hyperlink and image entries are collected while rendering.
func (d *docxDoc) docRelsXML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>
`)
	for _, rel := range d.rels {
		fmt.Fprintf(&sb, `<Relationship Id=%q Type=%q Target=%q`, rel.id, rel.relType, docxEscape(rel.target))
		if rel.targetMode != "" {
			fmt.Fprintf(&sb, ` TargetMode=%q`, rel.targetMode)
		}
		sb.WriteString("/>\n")
	}
	sb.WriteString(`</Relationships>`)
	return sb.String()
}

// stylesXML defines the named styles referenced from the document body. Markdown
// reaches down to six heading levels, so all six are defined here; a body that
// names a style the sheet omits silently falls back to Normal in Word. Each
// carries concrete formatting so Word/LibreOffice render the export close to the
// HTML version without relying on built-in defaults that differ per application.
const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:sz w:val="22"/></w:rPr></w:rPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:pPr><w:spacing w:after="160" w:line="276" w:lineRule="auto"/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="160"/></w:pPr><w:rPr><w:b/><w:sz w:val="44"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="240" w:after="80"/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="200" w:after="60"/></w:pPr><w:rPr><w:b/><w:sz w:val="28"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="160" w:after="40"/></w:pPr><w:rPr><w:b/><w:color w:val="1A7F37"/><w:sz w:val="25"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="RoleUser"><w:name w:val="Role User"/><w:basedOn w:val="Heading3"/><w:rPr><w:b/><w:color w:val="0969DA"/><w:sz w:val="25"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="RoleAssistant"><w:name w:val="Role Assistant"/><w:basedOn w:val="Heading3"/><w:rPr><w:b/><w:color w:val="1A7F37"/><w:sz w:val="25"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading4"><w:name w:val="heading 4"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="120" w:after="40"/></w:pPr><w:rPr><w:i/><w:color w:val="6E7781"/><w:sz w:val="23"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading5"><w:name w:val="heading 5"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="120" w:after="40"/></w:pPr><w:rPr><w:b/><w:sz w:val="22"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading6"><w:name w:val="heading 6"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="120" w:after="40"/></w:pPr><w:rPr><w:b/><w:i/><w:sz w:val="22"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:basedOn w:val="Normal"/><w:pPr><w:pBdr><w:left w:val="single" w:sz="18" w:space="4" w:color="D0D7DE"/></w:pBdr><w:shd w:val="clear" w:color="auto" w:fill="F6F8FA"/><w:spacing w:after="160" w:line="240" w:lineRule="auto"/><w:ind w:left="120"/><w:contextualSpacing/></w:pPr><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/><w:sz w:val="18"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:pBdr><w:left w:val="single" w:sz="12" w:space="8" w:color="D0D7DE"/></w:pBdr><w:spacing w:before="80" w:after="80"/><w:ind w:left="360"/></w:pPr><w:rPr><w:i/><w:color w:val="6E7781"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="160"/><w:ind w:left="360"/><w:contextualSpacing/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="TableCell"><w:name w:val="Table Cell"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:before="40" w:after="40" w:line="240" w:lineRule="auto"/></w:pPr><w:rPr><w:sz w:val="20"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="caption"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="160"/><w:jc w:val="center"/></w:pPr><w:rPr><w:i/><w:color w:val="6E7781"/><w:sz w:val="18"/></w:rPr></w:style>
<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="0969DA"/><w:u w:val="single"/></w:rPr></w:style>
<w:style w:type="table" w:styleId="ExportTable"><w:name w:val="Export Table"/><w:tblPr><w:tblCellMar><w:top w:w="60" w:type="dxa"/><w:left w:w="108" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>
</w:styles>`

// numberingXML defines the two list definitions ListParagraph items point at:
// numId=1 draws a bullet, numId=2 an incrementing decimal. Word renders the
// marker itself from here, so the document body must not repeat it in the run
// text — see listXML. Five levels are defined because markdown nests lists as
// deep as the writer cares to indent them.
var numberingXML = buildNumberingXML()

// buildNumberingXML generates the level definitions for both list flavours.
// Bullets cycle through three glyphs and decimals through the usual
// decimal / lower-letter / lower-roman ladder, matching what Word does for a
// list a user creates by hand.
func buildNumberingXML() string {
	bullets := []string{"•", "◦", "▪"}
	numFmts := []string{"decimal", "lowerLetter", "lowerRoman"}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:abstractNum w:abstractNumId="0">
`)
	for lvl := 0; lvl <= docxMaxListLevel; lvl++ {
		fmt.Fprintf(&sb,
			`<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="%s"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`+"\n",
			lvl, bullets[lvl%len(bullets)], 360*(lvl+1))
	}
	sb.WriteString(`</w:abstractNum>
<w:abstractNum w:abstractNumId="1">
`)
	for lvl := 0; lvl <= docxMaxListLevel; lvl++ {
		fmt.Fprintf(&sb,
			`<w:lvl w:ilvl="%d"><w:start w:val="1"/><w:numFmt w:val="%s"/><w:lvlText w:val="%%%d."/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`+"\n",
			lvl, numFmts[lvl%len(numFmts)], lvl+1, 360*(lvl+1))
	}
	sb.WriteString(`</w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
<w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>
</w:numbering>`)
	return sb.String()
}
