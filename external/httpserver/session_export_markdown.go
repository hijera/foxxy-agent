//go:build http

package httpserver

import (
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// This file turns a markdown fragment into the flat block/inline model the PDF
// and DOCX renderers walk. HTML uses goldmark's own renderer (see
// session_export_html.go); this model exists for the document formats that
// cannot consume HTML directly.
//
// The model is deliberately flat: containers (lists, blockquotes) are unrolled
// into a sequence of blocks carrying their nesting depth, because neither fpdf
// nor a WordprocessingML body is a tree the way HTML is. Both renderers walk
// the same slice, so a construct supported here is supported everywhere.

// exportRun is an inline text span with optional formatting. link carries a
// destination URL (empty when the span is not part of a link) and color a
// "RRGGBB" hex from the syntax highlighter (empty means the renderer's default).
type exportRun struct {
	text   string
	link   string
	color  string
	bold   bool
	italic bool
	code   bool
	strike bool
}

// Block kinds. A renderer that meets an unknown kind falls through to the
// paragraph path, so adding one here never breaks an existing format outright.
const (
	blockHeading   = "heading"
	blockParagraph = "paragraph"
	blockListItem  = "list_item"
	blockCode      = "code_block"
	blockTable     = "table"
	blockRule      = "rule"
	blockImage     = "image"
)

// exportBlock is a single rendered markdown block. Only the fields its kind
// needs are populated.
type exportBlock struct {
	kind      string
	level     int           // heading level 1..6
	runs      []exportRun   // inline content for text-ish kinds
	text      string        // raw text for code blocks
	lang      string        // fence info string of a code block
	codeLines [][]exportRun // highlighted code, one entry per source line
	ordered   bool          // list item ordered vs bullet
	number    int           // 1-based ordinal of an ordered list item
	indent    int           // list nesting depth, 0-based
	checked   *bool         // GFM task-list state, nil when not a task item

	// quote is the blockquote nesting depth, 0 outside a quote. A quote is a
	// depth on the blocks it holds rather than a kind of its own, so a list or a
	// code fence inside one keeps rendering as a list or a code fence.
	quote int
	table *exportTable // kind == blockTable
	image *exportImage // kind == blockImage
}

// exportTable is a GFM table: an optional header row plus body rows, and one
// alignment per column.
type exportTable struct {
	align  []string // "L" | "C" | "R" per column
	header []exportTableCell
	rows   [][]exportTableCell
}

// exportTableCell is one cell's inline content.
type exportTableCell struct {
	runs []exportRun
}

// exportMarkdown is the single parser/renderer configuration the export uses.
// GFM buys tables, strikethrough, task lists and autolinks — everything the SPA
// renders with remark-gfm, so a transcript exports the way it reads on screen.
var exportMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// markdownToBlocks parses a markdown string into the block model shared by the
// PDF and DOCX renderers. resolver may be nil; when set it is asked to turn
// image destinations into embeddable bytes.
func markdownToBlocks(md string, resolver *exportMediaResolver) []exportBlock {
	if strings.TrimSpace(md) == "" {
		return nil
	}
	source := []byte(md)
	doc := exportMarkdown.Parser().Parse(text.NewReader(source))
	w := blockWalker{source: source, resolver: resolver}
	w.children(doc, blockContext{})
	return w.blocks
}

// blockContext carries the nesting state an inner block inherits from the
// container it sits in.
type blockContext struct {
	indent int
	quote  int
}

// blockWalker accumulates blocks while descending the AST. Unlike a flat
// ast.Walk it recurses explicitly, which is what lets a list inside a list (or
// a paragraph inside a blockquote) keep its own identity instead of being
// flattened into its parent's inline runs.
type blockWalker struct {
	source   []byte
	resolver *exportMediaResolver
	blocks   []exportBlock
}

func (w *blockWalker) push(b exportBlock, ctx blockContext) {
	b.indent += ctx.indent
	b.quote = ctx.quote
	w.blocks = append(w.blocks, b)
}

// children descends into every child of n as a block-level node.
func (w *blockWalker) children(n gast.Node, ctx blockContext) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		w.node(c, ctx)
	}
}

// node dispatches one block-level node.
func (w *blockWalker) node(n gast.Node, ctx blockContext) {
	switch v := n.(type) {
	case *gast.Heading:
		w.push(exportBlock{kind: blockHeading, level: v.Level, runs: w.inline(v)}, ctx)
	case *gast.Paragraph:
		w.paragraph(v, ctx)
	case *gast.TextBlock:
		// Loose paragraph text (list items in a tight list). Same treatment as a
		// paragraph so the content still renders.
		w.paragraph(v, ctx)
	case *gast.List:
		w.list(v, ctx)
	case *gast.ListItem:
		// A list item reached without its parent list (defensive): treat it as a
		// bullet at the current depth.
		w.listItem(v, ctx, false, 0)
	case *gast.FencedCodeBlock:
		w.push(w.codeBlock(v, string(v.Language(w.source))), ctx)
	case *gast.CodeBlock:
		w.push(w.codeBlock(v, ""), ctx)
	case *gast.Blockquote:
		w.children(v, blockContext{indent: ctx.indent, quote: ctx.quote + 1})
	case *gast.ThematicBreak:
		w.push(exportBlock{kind: blockRule}, ctx)
	case *east.Table:
		w.table(v, ctx)
	default:
		// Unknown container: keep descending so its content is not lost.
		w.children(n, ctx)
	}
}

// paragraph emits a paragraph, promoting an image-only paragraph to its own
// image block so the renderers can size and place the picture.
func (w *blockWalker) paragraph(n gast.Node, ctx blockContext) {
	if img := w.soleImage(n); img != nil {
		w.push(exportBlock{kind: blockImage, image: w.image(img)}, ctx)
		return
	}
	runs := w.inline(n)
	if len(runs) == 0 {
		return
	}
	w.push(exportBlock{kind: blockParagraph, runs: runs}, ctx)
}

// list walks a list, numbering ordered items from the list's own start value.
func (w *blockWalker) list(n *gast.List, ctx blockContext) {
	number := n.Start
	if number == 0 {
		number = 1
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*gast.ListItem)
		if !ok {
			w.node(c, ctx)
			continue
		}
		w.listItem(item, ctx, n.IsOrdered(), number)
		number++
	}
}

// listItem emits the item's own line, then descends into whatever it contains
// (nested lists, code blocks, extra paragraphs) one indent level deeper.
func (w *blockWalker) listItem(n *gast.ListItem, ctx blockContext, ordered bool, number int) {
	block := exportBlock{kind: blockListItem, ordered: ordered, number: number}
	inner := blockContext{indent: ctx.indent + 1, quote: ctx.quote}

	first := true
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		// The item's first paragraph is the item's own text; everything after it
		// is nested content that belongs one level in.
		if first {
			first = false
			switch c.(type) {
			case *gast.TextBlock, *gast.Paragraph:
				block.checked = taskState(c)
				block.runs = w.inline(c)
				w.push(block, ctx)
				continue
			}
			// An item that opens with something other than text (a nested list, a
			// code fence) still needs its marker drawn.
			w.push(block, ctx)
		}
		w.node(c, inner)
	}
	if first {
		// Empty item: the marker alone still belongs in the document.
		w.push(block, ctx)
	}
}

// codeBlock reads a code block's source and runs it through the highlighter.
func (w *blockWalker) codeBlock(n gast.Node, lang string) exportBlock {
	src := codeBlockText(n, w.source)
	return exportBlock{
		kind:      blockCode,
		text:      src,
		lang:      lang,
		codeLines: highlightCode(lang, src),
	}
}

// table converts a GFM table node into the flat model.
func (w *blockWalker) table(n *east.Table, ctx blockContext) {
	tbl := &exportTable{}
	for _, a := range n.Alignments {
		tbl.align = append(tbl.align, alignCode(a))
	}
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		cells := w.tableRow(row)
		if _, isHeader := row.(*east.TableHeader); isHeader {
			tbl.header = cells
			continue
		}
		tbl.rows = append(tbl.rows, cells)
	}
	if len(tbl.header) == 0 && len(tbl.rows) == 0 {
		return
	}
	w.push(exportBlock{kind: blockTable, table: tbl}, ctx)
}

func (w *blockWalker) tableRow(row gast.Node) []exportTableCell {
	var cells []exportTableCell
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		cells = append(cells, exportTableCell{runs: w.inline(c)})
	}
	return cells
}

// alignCode maps goldmark's per-column alignment onto the single-letter code
// fpdf and WordprocessingML both understand.
func alignCode(a east.Alignment) string {
	switch a {
	case east.AlignCenter:
		return "C"
	case east.AlignRight:
		return "R"
	default:
		return "L"
	}
}

// taskState reports a GFM task-list checkbox on the item's first line, or nil
// when the item is an ordinary one.
func taskState(n gast.Node) *bool {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if cb, ok := c.(*east.TaskCheckBox); ok {
			checked := cb.IsChecked
			return &checked
		}
	}
	return nil
}

// soleImage returns the image node when n holds exactly one image and no other
// visible content, so it can be promoted to a block of its own.
func (w *blockWalker) soleImage(n gast.Node) *gast.Image {
	var found *gast.Image
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *gast.Image:
			if found != nil {
				return nil
			}
			found = v
		case *gast.Text:
			if strings.TrimSpace(string(v.Segment.Value(w.source))) != "" {
				return nil
			}
		default:
			return nil
		}
	}
	return found
}

// image builds the media descriptor for an image node, asking the resolver for
// bytes when one is configured.
func (w *blockWalker) image(n *gast.Image) *exportImage {
	img := &exportImage{
		alt: inlineText(n, w.source),
		src: string(n.Destination),
	}
	if img.alt == "" {
		img.alt = img.src
	}
	if w.resolver != nil {
		w.resolver.fill(img)
	}
	return img
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

// inlineStyle is the formatting a run inherits from the inline nodes wrapping it.
type inlineStyle struct {
	bold   bool
	italic bool
	code   bool
	strike bool
	link   string
}

// run materialises one span with the ambient style applied.
func (st inlineStyle) run(text string) exportRun {
	return exportRun{
		text:   text,
		link:   st.link,
		bold:   st.bold,
		italic: st.italic,
		code:   st.code,
		strike: st.strike,
	}
}

// inline walks the inline children of a block into formatted runs.
func (w *blockWalker) inline(n gast.Node) []exportRun {
	var runs []exportRun
	var walk func(node gast.Node, st inlineStyle)
	walk = func(node gast.Node, st inlineStyle) {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *gast.Text:
				runs = append(runs, st.run(string(v.Segment.Value(w.source))))
				if v.SoftLineBreak() || v.HardLineBreak() {
					// A wrapped source line is still one paragraph; without the space
					// the two lines would be glued into one word.
					runs = append(runs, st.run(" "))
				}
			case *gast.String:
				runs = append(runs, st.run(string(v.Value)))
			case *gast.CodeSpan:
				inner := st
				inner.code = true
				walk(v, inner)
			case *gast.Emphasis:
				inner := st
				if v.Level >= 2 {
					inner.bold = true
				} else {
					inner.italic = true
				}
				walk(v, inner)
			case *east.Strikethrough:
				inner := st
				inner.strike = true
				walk(v, inner)
			case *east.TaskCheckBox:
				// The checkbox is reported on the list item itself (see taskState);
				// drawing it again here would double the marker.
			case *gast.Link:
				inner := st
				inner.link = string(v.Destination)
				walk(v, inner)
			case *gast.AutoLink:
				url := string(v.URL(w.source))
				inner := st
				inner.link = url
				runs = append(runs, inner.run(string(v.Label(w.source))))
			case *gast.Image:
				// An image sharing a line with text cannot be laid out as a block;
				// carry its alt text plus a link to the source instead.
				alt := inlineText(v, w.source)
				if alt == "" {
					alt = string(v.Destination)
				}
				inner := st
				inner.italic = true
				inner.link = string(v.Destination)
				runs = append(runs, inner.run(alt))
			case *gast.RawHTML, *gast.HTMLBlock:
				// Raw HTML is not rendered in the document formats; goldmark keeps it
				// escaped in HTML for the same reason.
			default:
				walk(c, st)
			}
		}
	}
	walk(n, inlineStyle{})
	return coalesceRuns(runs)
}

// inlineText flattens an inline subtree to plain text (used for image alt text).
func inlineText(n gast.Node, source []byte) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*gast.Text); ok {
			sb.Write(t.Segment.Value(source))
			continue
		}
		sb.WriteString(inlineText(c, source))
	}
	return strings.TrimSpace(sb.String())
}

// coalesceRuns merges adjacent runs with identical formatting so the renderers
// emit fewer spans without changing output. Empty runs are dropped.
func coalesceRuns(runs []exportRun) []exportRun {
	out := make([]exportRun, 0, len(runs))
	for _, r := range runs {
		if r.text == "" {
			continue
		}
		if len(out) > 0 {
			last := &out[len(out)-1]
			if last.bold == r.bold && last.italic == r.italic && last.code == r.code &&
				last.strike == r.strike && last.link == r.link && last.color == r.color {
				last.text += r.text
				continue
			}
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// runsText flattens runs back to plain text, which the measuring code and the
// plain-text fallbacks need.
func runsText(runs []exportRun) string {
	var sb strings.Builder
	for _, r := range runs {
		sb.WriteString(r.text)
	}
	return sb.String()
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
