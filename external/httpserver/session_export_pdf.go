//go:build http

package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// The PDF export draws the shared block model with fpdf. There is no HTML step
// in between and no browser involved, so the binary stays self-contained; the
// cost is that every construct with a shape — tables, code boxes, images,
// indented lists — is laid out here by hand.

// Font families registered on every export document. Sans carries prose, mono
// carries code and table cells that hold code, because column alignment is the
// whole point of a monospaced face.
const (
	exportPDFFont     = "DejaVu"
	exportPDFMonoFont = "DejaVuMono"
)

// Page geometry and type scale.
const (
	exportPDFMargin = 18.0 // mm, all four sides
	exportBodySize  = 11.0 // pt, paragraph text
	exportCodeSize  = 8.5  // pt, code blocks and table cells holding code
	exportTableSize = 9.5  // pt, table body text

	// lineHeightRatio converts a point size into the millimetre line advance
	// used throughout the PDF body.
	lineHeightRatio = 0.42

	// paragraphGap is the vertical breathing room left after a block, expressed
	// as a fraction of the block's font size. It has to stay clearly below a full
	// line advance so a gap never reads as an empty line.
	paragraphGap = 0.45

	// exportIndentStep is how far one list or quote nesting level moves the text.
	exportIndentStep = 6.0 // mm

	// codeBoxPad is the padding inside a code block's shaded box.
	codeBoxPad = 2.0 // mm

	// cellPad is the horizontal padding inside a table cell.
	cellPad = 1.8 // mm

	// tableMinColWidth keeps a squeezed column wide enough to hold a short word.
	tableMinColWidth = 14.0 // mm
)

// Palette, matching the HTML export's light theme.
var (
	pdfColorMuted     = [3]int{110, 119, 129}
	pdfColorRule      = [3]int{208, 215, 222}
	pdfColorSoftFill  = [3]int{246, 248, 250}
	pdfColorLink      = [3]int{9, 105, 218}
	pdfColorUser      = [3]int{9, 105, 218}
	pdfColorAssistant = [3]int{26, 127, 55}
)

// newExportPDF builds the page geometry, registers the embedded font cuts and
// installs the page footer. Extracted so layout tests can exercise the block
// writers on the exact same document setup the real export uses.
func newExportPDF(title string) *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(title, true)
	pdf.SetAuthor("FoxxyCode", true)
	pdf.SetAutoPageBreak(true, exportPDFMargin)
	pdf.SetMargins(exportPDFMargin, exportPDFMargin, exportPDFMargin)
	// Embed the Unicode-capable DejaVu cuts so Cyrillic (and any other non-Latin-1
	// code point) renders instead of panicking in fpdf's width table. Italic maps
	// to the upright cut in both families because we deliberately ship only
	// regular + bold to bound the binary size.
	pdf.AddUTF8FontFromBytes(exportPDFFont, "", dejavuSansRegular)
	pdf.AddUTF8FontFromBytes(exportPDFFont, "B", dejavuSansBold)
	pdf.AddUTF8FontFromBytes(exportPDFFont, "I", dejavuSansRegular)
	pdf.AddUTF8FontFromBytes(exportPDFFont, "BI", dejavuSansBold)
	pdf.AddUTF8FontFromBytes(exportPDFMonoFont, "", dejavuMonoRegular)
	pdf.AddUTF8FontFromBytes(exportPDFMonoFont, "B", dejavuMonoBold)
	pdf.AddUTF8FontFromBytes(exportPDFMonoFont, "I", dejavuMonoRegular)
	pdf.AddUTF8FontFromBytes(exportPDFMonoFont, "BI", dejavuMonoBold)

	// A transcript runs to many pages; "3 / 11" tells the reader where they are
	// and makes a printed copy reorderable.
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont(exportPDFFont, "", 8)
		pdf.SetTextColor(pdfColorMuted[0], pdfColorMuted[1], pdfColorMuted[2])
		pdf.CellFormat(0, 6, fmt.Sprintf("%d / {nb}", pdf.PageNo()), "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})
	return pdf
}

func renderPDFExport(doc exportDocument) ([]byte, error) {
	pdf := newExportPDF(doc.Title)
	media := doc.media()
	pdf.AddPage()

	contentWidth := pdfContentWidth(pdf)

	// Document title.
	pdf.SetFont(exportPDFFont, "B", 18)
	pdf.MultiCell(contentWidth, 9, doc.Title, "", "L", false)
	pdf.Ln(2)
	pdf.SetFont(exportPDFFont, "", 8)
	pdf.SetTextColor(pdfColorMuted[0], pdfColorMuted[1], pdfColorMuted[2])
	pdf.MultiCell(contentWidth, 4, "Exported "+doc.ExportedAt, "", "L", false)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(3)

	for _, m := range doc.Messages {
		if m.Reasoning != "" {
			writeRoleLabel(pdf, "Reasoning", pdfColorMuted)
			writeReasoningPDF(pdf, markdownToBlocks(m.Reasoning, media))
		}
		color := pdfColorAssistant
		if m.Role == "user" {
			color = pdfColorUser
		}
		writeRoleLabel(pdf, m.Role, color)
		writeBlocksPDF(pdf, markdownToBlocks(m.Content, media))
		writeAttachmentsPDF(pdf, m.Attachments, media)
		pdf.Ln(3)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// pdfContentWidth is the drawable width between the page margins.
func pdfContentWidth(pdf *fpdf.Fpdf) float64 {
	w, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	return w - left - right
}

// pdfPageBottom is the y coordinate past which content spills onto a new page.
func pdfPageBottom(pdf *fpdf.Fpdf) float64 {
	_, h := pdf.GetPageSize()
	return h - exportPDFMargin
}

func writeRoleLabel(pdf *fpdf.Fpdf, label string, color [3]int) {
	pdf.Ln(1)
	pdf.SetFont(exportPDFFont, "B", 12)
	pdf.SetTextColor(color[0], color[1], color[2])
	pdf.MultiCell(pdfContentWidth(pdf), 6, exportRoleLabel(label), "", "L", false)
	pdf.SetTextColor(0, 0, 0)
}

// writeReasoningPDF renders reasoning in a muted, italic block. It goes through
// the same block writer as the answer so a multi-paragraph thought keeps its
// structure instead of being poured out as raw lines.
func writeReasoningPDF(pdf *fpdf.Fpdf, blocks []exportBlock) {
	pdf.SetTextColor(90, 90, 90)
	writeBlocksStyled(pdf, blocks, "I", exportBodySize-1)
	pdf.SetTextColor(0, 0, 0)
}

func writeBlocksPDF(pdf *fpdf.Fpdf, blocks []exportBlock) {
	writeBlocksStyled(pdf, blocks, "", exportBodySize)
}

// writeBlocksStyled draws every block, applying a base style and body size the
// reasoning path can override.
func writeBlocksStyled(pdf *fpdf.Fpdf, blocks []exportBlock, baseStyle string, bodySize float64) {
	baseLeft, _, _, _ := pdf.GetMargins()
	defer pdf.SetLeftMargin(baseLeft)

	for i, b := range blocks {
		indent := float64(b.indent+b.quote) * exportIndentStep
		pdf.SetLeftMargin(baseLeft + indent)
		pdf.SetX(baseLeft + indent)

		if b.quote > 0 && b.kind != blockRule {
			// A quote reads as a quote only if the reader can see where it starts
			// and stops, so every block inside one carries the rule beside it.
			writeQuotedBlock(pdf, b, baseLeft+indent, baseStyle, bodySize)
			continue
		}
		writeOneBlockPDF(pdf, b, baseStyle, bodySize, i, blocks)
	}
}

// writeQuotedBlock draws a block inside a blockquote: muted, italic, with a grey
// vertical rule down its left edge.
func writeQuotedBlock(pdf *fpdf.Fpdf, b exportBlock, left float64, baseStyle string, bodySize float64) {
	top := pdf.GetY()
	startPage := pdf.PageNo()
	pdf.SetTextColor(pdfColorMuted[0], pdfColorMuted[1], pdfColorMuted[2])
	writeOneBlockPDF(pdf, b, mergeStyle(baseStyle, "I"), bodySize, 0, nil)
	pdf.SetTextColor(0, 0, 0)

	// Only rule the part that stayed on this page; a block that spilled over got
	// its own top on the next one and ruling across the break would draw over the
	// footer.
	if pdf.PageNo() == startPage {
		pdf.SetFillColor(pdfColorRule[0], pdfColorRule[1], pdfColorRule[2])
		pdf.Rect(left-3, top, 0.8, pdf.GetY()-top, "F")
	}
}

// writeOneBlockPDF draws a single block at the current position. blocks/i are
// used only to decide whether a list item closes its list; pass nil to skip.
func writeOneBlockPDF(pdf *fpdf.Fpdf, b exportBlock, baseStyle string, bodySize float64, i int, blocks []exportBlock) {
	switch b.kind {
	case blockHeading:
		size := 16 - float64(headingLevel(b.level))*1.5
		if size < 10 {
			size = 10
		}
		writeRunsPDF(pdf, b.runs, mergeStyle(baseStyle, "B"), size)
		pdf.Ln(size * paragraphGap)
	case blockCode:
		writeCodeBlockPDF(pdf, b)
	case blockTable:
		writeTablePDF(pdf, b.table)
	case blockRule:
		writeRulePDF(pdf)
	case blockImage:
		writeImagePDF(pdf, b.image)
	case blockListItem:
		// The marker is part of the flowed text because PDF has no list construct
		// of its own. Only the item that closes the list gets the block gap, so
		// the list does not run into the paragraph below it.
		writeRunsPDF(pdf, prependRun(b.runs, exportRun{text: listMarker(b)}), baseStyle, bodySize)
		if blocks == nil || i+1 >= len(blocks) || blocks[i+1].kind != blockListItem {
			pdf.Ln(bodySize * paragraphGap)
		}
	default: // paragraph / text
		writeRunsPDF(pdf, b.runs, baseStyle, bodySize)
		pdf.Ln(bodySize * paragraphGap)
	}
}

// mergeStyle folds an extra fpdf style flag into a base one.
func mergeStyle(base, extra string) string {
	style := base
	for _, f := range extra {
		if !strings.ContainsRune(style, f) {
			style += string(f)
		}
	}
	return style
}

// listMarker is the glyph the PDF prints in front of a list item. Ordered items
// print their real ordinal; bullets cycle through three shapes so a nested list
// is readable at a glance. DOCX leaves this to the document numbering instead.
func listMarker(b exportBlock) string {
	if b.checked != nil {
		if *b.checked {
			return "☑  " // ballot box with check
		}
		return "☐  " // empty ballot box
	}
	if b.ordered && b.number > 0 {
		return fmt.Sprintf("%d.  ", b.number)
	}
	if b.ordered {
		return "1.  "
	}
	switch b.indent % 3 {
	case 1:
		return "◦  " // white bullet
	case 2:
		return "▪  " // small black square
	default:
		return "•  "
	}
}

func prependRun(runs []exportRun, r exportRun) []exportRun {
	out := make([]exportRun, 0, len(runs)+1)
	out = append(out, r)
	return append(out, runs...)
}

// writeRulePDF draws a thematic break.
func writeRulePDF(pdf *fpdf.Fpdf) {
	pdf.Ln(2)
	y := pdf.GetY()
	left, _, _, _ := pdf.GetMargins()
	pdf.SetDrawColor(pdfColorRule[0], pdfColorRule[1], pdfColorRule[2])
	pdf.SetLineWidth(0.3)
	pdf.Line(left, y, left+pdfContentWidth(pdf), y)
	pdf.SetDrawColor(0, 0, 0)
	pdf.Ln(3)
}

// runFont selects the family and style one run draws with.
func runFont(base string, r exportRun) (family, style string) {
	family = exportPDFFont
	if r.code {
		family = exportPDFMonoFont
	}
	bold := strings.Contains(base, "B") || r.bold
	italic := strings.Contains(base, "I") || r.italic
	if bold {
		style += "B"
	}
	if italic {
		style += "I"
	}
	return family, style
}

// runSize is the point size a run draws at. Monospaced glyphs run wider than
// their sans counterparts, so code steps down to keep the colour of the line
// even.
func runSize(base float64, r exportRun) float64 {
	if r.code {
		return base - 1.5
	}
	return base
}

// applyRunStyle sets the font and colour for one run and reports whether the
// text colour was changed (so the caller can restore it).
func applyRunStyle(pdf *fpdf.Fpdf, base string, size float64, r exportRun) bool {
	family, style := runFont(base, r)
	pdf.SetFont(family, style, runSize(size, r))
	if red, green, blue, ok := hexToRGB(r.color); ok {
		pdf.SetTextColor(red, green, blue)
		return true
	}
	if r.link != "" {
		pdf.SetTextColor(pdfColorLink[0], pdfColorLink[1], pdfColorLink[2])
		return true
	}
	return false
}

// writeRunsPDF emits one paragraph built from formatted runs. fpdf has no
// rich-text cell, but Write flows text from the current position and wraps at
// the right margin, so switching the font between Write calls keeps every run
// on the same line instead of starting a new one per run (which MultiCell would
// do, shattering a formatted sentence across several lines).
func writeRunsPDF(pdf *fpdf.Fpdf, runs []exportRun, style string, size float64) {
	lineHeight := size * lineHeightRatio
	red, green, blue := pdf.GetTextColor()
	wrote := false
	for _, r := range runs {
		if r.text == "" {
			continue
		}
		recolored := applyRunStyle(pdf, style, size, r)
		startX, startY := pdf.GetXY()
		if r.link != "" {
			pdf.WriteLinkString(lineHeight, r.text, r.link)
		} else {
			pdf.Write(lineHeight, r.text)
		}
		if r.strike {
			strikeRun(pdf, startX, startY, lineHeight)
		}
		if recolored {
			pdf.SetTextColor(red, green, blue)
		}
		wrote = true
	}
	if wrote {
		// Close the flowed line; the caller adds any inter-block spacing.
		pdf.Ln(lineHeight)
	}
}

// strikeRun draws the line through a struck run. A run that wrapped onto another
// line is left alone: fpdf gives no per-line extents for flowed text, and one
// rule drawn across the wrap would cut through unrelated words.
func strikeRun(pdf *fpdf.Fpdf, startX, startY, lineHeight float64) {
	endX, endY := pdf.GetXY()
	if endY != startY || endX <= startX {
		return
	}
	strikeAt(pdf, startX, endX, startY+lineHeight*0.55)
}

// strikeAt draws the rule through struck text in the current text colour. The
// stroke colour is set explicitly and restored: a table row leaves the light
// grey border colour selected, which would make the rule all but invisible.
func strikeAt(pdf *fpdf.Fpdf, x0, x1, y float64) {
	r, g, b := pdf.GetTextColor()
	dr, dg, db := pdf.GetDrawColor()
	pdf.SetDrawColor(r, g, b)
	pdf.SetLineWidth(0.25)
	pdf.Line(x0, y, x1, y)
	pdf.SetDrawColor(dr, dg, db)
}

// --- code blocks ----------------------------------------------------------

// writeCodeBlockPDF draws a code block in a shaded, bordered box. The box is
// painted before the text (fpdf has no z-order), so the number of lines that fit
// on the current page is computed first; a long block therefore lays out as one
// box per page rather than one rule drawn over the page break.
func writeCodeBlockPDF(pdf *fpdf.Fpdf, b exportBlock) {
	left, _, _, _ := pdf.GetMargins()
	width := pdfContentWidth(pdf)
	inner := width - 2*codeBoxPad
	lineH := exportCodeSize * lineHeightRatio * 1.15

	var lines [][]exportRun
	for _, src := range codeLinesOf(b) {
		wrapped := wrapRuns(pdf, src, inner, "", exportCodeSize)
		if len(wrapped) == 0 {
			lines = append(lines, nil) // blank line inside the snippet
			continue
		}
		lines = append(lines, wrapped...)
	}
	if len(lines) == 0 {
		return
	}

	for i := 0; i < len(lines); {
		y := pdf.GetY()
		room := pdfPageBottom(pdf) - y - 2*codeBoxPad
		fit := int(room / lineH)
		if fit <= 0 {
			pdf.AddPage()
			y = pdf.GetY()
			room = pdfPageBottom(pdf) - y - 2*codeBoxPad
			fit = int(room / lineH)
			if fit <= 0 {
				fit = 1 // a page too short for one line would loop forever
			}
		}
		if fit > len(lines)-i {
			fit = len(lines) - i
		}
		boxH := float64(fit)*lineH + 2*codeBoxPad

		pdf.SetFillColor(pdfColorSoftFill[0], pdfColorSoftFill[1], pdfColorSoftFill[2])
		pdf.SetDrawColor(pdfColorRule[0], pdfColorRule[1], pdfColorRule[2])
		pdf.SetLineWidth(0.2)
		pdf.Rect(left, y, width, boxH, "FD")
		pdf.SetDrawColor(0, 0, 0)

		for k := 0; k < fit; k++ {
			drawRunsLine(pdf, lines[i+k], left+codeBoxPad, y+codeBoxPad+float64(k)*lineH, "", exportCodeSize, lineH)
		}
		pdf.SetXY(left, y+boxH)
		i += fit
	}
	pdf.Ln(exportCodeSize * paragraphGap)
}

// --- tables ---------------------------------------------------------------

// writeTablePDF draws a GFM table as a real cell grid: measured column widths,
// wrapped cell text that keeps its inline formatting, and a header row repeated
// whenever the table spills onto another page.
func writeTablePDF(pdf *fpdf.Fpdf, tbl *exportTable) {
	if tbl == nil {
		return
	}
	cols := tableColumnCount(tbl)
	if cols == 0 {
		return
	}
	left, _, _, _ := pdf.GetMargins()
	widths := tableColumnWidths(pdf, tbl, cols, pdfContentWidth(pdf))
	lineH := exportTableSize * lineHeightRatio * 1.2

	pdf.Ln(1)
	if len(tbl.header) > 0 {
		writeTableRowPDF(pdf, tbl, tbl.header, widths, left, lineH, true)
	}
	for _, row := range tbl.rows {
		if len(tbl.header) > 0 && !tableRowFits(pdf, row, widths, lineH) {
			pdf.AddPage()
			writeTableRowPDF(pdf, tbl, tbl.header, widths, left, lineH, true)
		}
		writeTableRowPDF(pdf, tbl, row, widths, left, lineH, false)
	}
	pdf.Ln(exportTableSize * paragraphGap)
}

// tableColumnCount is the widest row in the table; a ragged row is padded with
// blanks rather than dropped.
func tableColumnCount(tbl *exportTable) int {
	cols := len(tbl.header)
	for _, row := range tbl.rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if len(tbl.align) > cols {
		cols = len(tbl.align)
	}
	return cols
}

// tableColumnWidths measures every column's natural width and fits the set into
// the available page width. Columns that already fit keep their measurement;
// the overflow is taken proportionally from the ones wide enough to give it up,
// so a table of one long prose column and three short ones does not squeeze the
// short ones into vertical strips of single letters.
func tableColumnWidths(pdf *fpdf.Fpdf, tbl *exportTable, cols int, avail float64) []float64 {
	natural := make([]float64, cols)
	measure := func(cells []exportTableCell, bold bool) {
		for i, c := range cells {
			if i >= cols {
				break
			}
			style := ""
			if bold {
				style = "B"
			}
			w := runsWidth(pdf, c.runs, style, exportTableSize) + 2*cellPad
			if w > natural[i] {
				natural[i] = w
			}
		}
	}
	measure(tbl.header, true)
	for _, row := range tbl.rows {
		measure(row, false)
	}

	total := 0.0
	for i := range natural {
		if natural[i] < tableMinColWidth {
			natural[i] = tableMinColWidth
		}
		total += natural[i]
	}
	if total <= avail {
		// Spread the slack so the table spans the text column instead of hugging
		// the left margin.
		extra := (avail - total) / float64(cols)
		for i := range natural {
			natural[i] += extra
		}
		return natural
	}

	// Shrink only what is above the floor, proportionally to how far above it is.
	excess := total - avail
	slack := 0.0
	for _, w := range natural {
		slack += w - tableMinColWidth
	}
	if slack <= 0 {
		// Every column is already at the floor: share the page evenly and accept
		// that a very wide table wraps hard.
		for i := range natural {
			natural[i] = avail / float64(cols)
		}
		return natural
	}
	for i := range natural {
		natural[i] -= (natural[i] - tableMinColWidth) / slack * excess
	}
	return natural
}

// tableRowFits reports whether a row still has room on the current page.
func tableRowFits(pdf *fpdf.Fpdf, row []exportTableCell, widths []float64, lineH float64) bool {
	_, h := tableRowLines(pdf, row, widths, lineH, false)
	return pdf.GetY()+h <= pdfPageBottom(pdf)
}

// tableRowLines wraps every cell of a row and reports the wrapped lines plus the
// row height.
func tableRowLines(pdf *fpdf.Fpdf, row []exportTableCell, widths []float64, lineH float64, header bool) ([][][]exportRun, float64) {
	style := ""
	if header {
		style = "B"
	}
	lines := make([][][]exportRun, len(widths))
	maxLines := 1
	for i := range widths {
		if i >= len(row) {
			continue
		}
		lines[i] = wrapRuns(pdf, row[i].runs, widths[i]-2*cellPad, style, exportTableSize)
		if len(lines[i]) > maxLines {
			maxLines = len(lines[i])
		}
	}
	return lines, float64(maxLines)*lineH + 2*cellPad
}

// writeTableRowPDF draws one row: background, borders, then the wrapped cell
// text honouring each column's alignment.
func writeTableRowPDF(pdf *fpdf.Fpdf, tbl *exportTable, row []exportTableCell, widths []float64, left, lineH float64, header bool) {
	lines, height := tableRowLines(pdf, row, widths, lineH, header)
	if pdf.GetY()+height > pdfPageBottom(pdf) {
		pdf.AddPage()
	}
	y := pdf.GetY()

	style := ""
	if header {
		style = "B"
		pdf.SetFillColor(pdfColorSoftFill[0], pdfColorSoftFill[1], pdfColorSoftFill[2])
	}
	pdf.SetDrawColor(pdfColorRule[0], pdfColorRule[1], pdfColorRule[2])
	pdf.SetLineWidth(0.2)

	x := left
	for i, w := range widths {
		mode := "D"
		if header {
			mode = "FD"
		}
		pdf.Rect(x, y, w, height, mode)
		for k, line := range lines[i] {
			lineY := y + cellPad + float64(k)*lineH
			lineX := x + cellPad
			switch tableAlign(tbl, i) {
			case "C":
				lineX = x + (w-runsWidth(pdf, line, style, exportTableSize))/2
			case "R":
				lineX = x + w - cellPad - runsWidth(pdf, line, style, exportTableSize)
			}
			drawRunsLine(pdf, line, lineX, lineY, style, exportTableSize, lineH)
		}
		x += w
	}
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetXY(left, y+height)
}

// tableAlign is the alignment of column i, defaulting to left.
func tableAlign(tbl *exportTable, i int) string {
	if i < len(tbl.align) && tbl.align[i] != "" {
		return tbl.align[i]
	}
	return "L"
}

// --- images ---------------------------------------------------------------

// writeImagePDF places a picture, scaled to fit the text column and never taller
// than a comfortable fraction of the page. An image that could not be resolved
// locally degrades to its caption plus a link to the original source.
func writeImagePDF(pdf *fpdf.Fpdf, img *exportImage) {
	if img == nil {
		return
	}
	if !img.embeddable() {
		writeRunsPDF(pdf, []exportRun{{text: img.alt, italic: true, link: img.src}}, "", exportBodySize-1)
		pdf.Ln(exportBodySize * paragraphGap)
		return
	}

	left, _, _, _ := pdf.GetMargins()
	maxW := pdfContentWidth(pdf)
	_, pageH := pdf.GetPageSize()
	maxH := pageH * 0.55

	// fpdf measures in millimetres; treat the source as 96 dpi, the web default.
	const pxPerMM = 96.0 / 25.4
	w := float64(img.widthPx) / pxPerMM
	h := float64(img.heightPx) / pxPerMM
	if w > maxW {
		h *= maxW / w
		w = maxW
	}
	if h > maxH {
		w *= maxH / h
		h = maxH
	}

	// Key the registration by content, not by identity: the same picture used
	// twice then shares one embedded copy, and the output stays byte-identical
	// across runs (a pointer address would not be).
	sum := sha256.Sum256(img.data)
	name := hex.EncodeToString(sum[:8])
	opts := fpdf.ImageOptions{ImageType: strings.TrimPrefix(img.mime, "image/"), ReadDpi: false}
	pdf.RegisterImageOptionsReader(name, opts, bytes.NewReader(img.data))
	if pdf.Err() {
		// A picture fpdf cannot decode must not sink the whole export.
		pdf.ClearError()
		writeRunsPDF(pdf, []exportRun{{text: img.alt, italic: true, link: img.src}}, "", exportBodySize-1)
		return
	}

	pdf.Ln(2)
	if pdf.GetY()+h > pdfPageBottom(pdf) {
		pdf.AddPage()
	}
	pdf.ImageOptions(name, left, pdf.GetY(), w, h, true, opts, 0, "")
	if img.alt != "" && img.alt != img.src {
		pdf.SetTextColor(pdfColorMuted[0], pdfColorMuted[1], pdfColorMuted[2])
		writeRunsPDF(pdf, []exportRun{{text: img.alt, italic: true}}, "I", exportBodySize-2)
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.Ln(exportBodySize * paragraphGap)
}

// writeAttachmentsPDF renders the files uploaded on a turn.
func writeAttachmentsPDF(pdf *fpdf.Fpdf, atts []exportAttachment, media *exportMediaResolver) {
	if len(atts) == 0 {
		return
	}
	pdf.SetTextColor(pdfColorMuted[0], pdfColorMuted[1], pdfColorMuted[2])
	writeRunsPDF(pdf, []exportRun{{text: "Attachments", bold: true}}, "", exportBodySize-2)
	pdf.SetTextColor(0, 0, 0)
	for _, a := range atts {
		img := &exportImage{alt: a.Name, src: a.Path}
		if media != nil {
			media.fill(img)
		}
		if img.embeddable() {
			writeImagePDF(pdf, img)
			continue
		}
		writeRunsPDF(pdf, []exportRun{{text: "•  " + a.Name}}, "", exportBodySize-1)
	}
}

// --- run measurement and placement ----------------------------------------

// runsWidth measures the drawn width of a run sequence at the given base style
// and size.
func runsWidth(pdf *fpdf.Fpdf, runs []exportRun, style string, size float64) float64 {
	total := 0.0
	for _, r := range runs {
		family, st := runFont(style, r)
		pdf.SetFont(family, st, runSize(size, r))
		total += pdf.GetStringWidth(r.text)
	}
	return total
}

// wrapRuns breaks a run sequence into display lines no wider than width, keeping
// each fragment's own formatting. fpdf's SplitText works on a plain string and a
// single font, so it cannot wrap a sentence whose middle is bold or monospaced —
// which is exactly what a table cell or a highlighted code line holds.
func wrapRuns(pdf *fpdf.Fpdf, runs []exportRun, width float64, style string, size float64) [][]exportRun {
	if width <= 0 || len(runs) == 0 {
		return nil
	}
	var lines [][]exportRun
	var line []exportRun
	used := 0.0

	flush := func() {
		if len(line) > 0 {
			lines = append(lines, line)
			line = nil
		}
		used = 0
	}
	// add places one already-measured fragment, opening a new line when it no
	// longer fits. Leading whitespace is dropped at a wrap so the next line does
	// not start with an indent the source never had.
	add := func(r exportRun, text string, w float64) {
		if used > 0 && used+w > width {
			flush()
			text = strings.TrimLeft(text, " 	")
			if text == "" {
				return
			}
			family, st := runFont(style, r)
			pdf.SetFont(family, st, runSize(size, r))
			w = pdf.GetStringWidth(text)
		}
		frag := r
		frag.text = text
		line = append(line, frag)
		used += w
	}

	for _, r := range runs {
		if r.text == "" {
			continue
		}
		family, st := runFont(style, r)
		pdf.SetFont(family, st, runSize(size, r))
		for _, word := range splitWords(r.text) {
			w := pdf.GetStringWidth(word)
			if w <= width {
				add(r, word, w)
				continue
			}
			// A single fragment wider than the column (a URL, a long identifier)
			// has to be broken mid-word or it would overflow the cell.
			for _, piece := range breakWide(pdf, word, width, width-used) {
				add(r, piece, pdf.GetStringWidth(piece))
				if used >= width {
					flush()
				}
			}
		}
	}
	flush()
	return lines
}

// splitWords cuts text into wrap opportunities, keeping each run of spaces
// attached to the word before it so the gap survives on the line.
func splitWords(text string) []string {
	var out []string
	var cur strings.Builder
	inSpace := false
	for _, r := range text {
		isSpace := r == ' ' || r == '	'
		if isSpace {
			cur.WriteRune(r)
			inSpace = true
			continue
		}
		if inSpace {
			out = append(out, cur.String())
			cur.Reset()
			inSpace = false
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// breakWide chops an over-long word into pieces: the first fills whatever is
// left on the current line, the rest each fill a full one.
func breakWide(pdf *fpdf.Fpdf, word string, width, firstWidth float64) []string {
	if firstWidth < width/4 {
		firstWidth = width
	}
	var out []string
	var cur strings.Builder
	limit := firstWidth
	for _, r := range word {
		cur.WriteRune(r)
		if pdf.GetStringWidth(cur.String()) <= limit {
			continue
		}
		// One rune too many: emit what fit and restart with the overflowing rune.
		s := cur.String()
		runes := []rune(s)
		if len(runes) > 1 {
			out = append(out, string(runes[:len(runes)-1]))
			cur.Reset()
			cur.WriteRune(runes[len(runes)-1])
		} else {
			out = append(out, s)
			cur.Reset()
		}
		limit = width
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// drawRunsLine paints one already-wrapped line at an exact position. Cells are
// drawn with zero margin so the caller's coordinates are the ones that count.
func drawRunsLine(pdf *fpdf.Fpdf, runs []exportRun, x, y float64, style string, size, lineH float64) {
	if len(runs) == 0 {
		return
	}
	margin := pdf.GetCellMargin()
	pdf.SetCellMargin(0)
	defer pdf.SetCellMargin(margin)

	red, green, blue := pdf.GetTextColor()
	cx := x
	for _, r := range runs {
		if r.text == "" {
			continue
		}
		recolored := applyRunStyle(pdf, style, size, r)
		w := pdf.GetStringWidth(r.text)
		pdf.SetXY(cx, y)
		pdf.CellFormat(w, lineH, r.text, "", 0, "L", false, 0, r.link)
		if r.strike {
			strikeAt(pdf, cx, cx+w, y+lineH*0.55)
		}
		if recolored {
			pdf.SetTextColor(red, green, blue)
		}
		cx += w
	}
}
