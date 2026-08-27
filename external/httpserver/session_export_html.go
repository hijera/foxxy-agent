//go:build http

package httpserver

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// The HTML export keeps goldmark's own renderer — it handles CommonMark's
// corner cases better than a hand-rolled emitter would, and GFM gives it tables,
// strikethrough and task lists for free. Two things are wired on top:
//
//   - fenced code blocks are rendered by chroma, so a snippet is highlighted the
//     way the chat window highlights it;
//   - image destinations that resolve to a local session asset are rewritten to
//     data: URIs, so the downloaded file shows its pictures with nothing else
//     alongside it.
//
// Everything is inlined (styles, fonts fall back to system stacks, images as
// data URIs) because the export is a single file a user mails or archives.

const htmlExportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  --fg: #1f2328;
  --fg-muted: #6e7781;
  --bg: #ffffff;
  --bg-soft: #f6f8fa;
  --border: #d0d7de;
  --user: #0969da;
  --assistant: #1a7f37;
  --reasoning-bg: #fffdf0;
  --reasoning-rule: #d0bc00;
  --mono: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
}
@media (prefers-color-scheme: dark) {
  :root {
    --fg: #e6edf3;
    --fg-muted: #9198a1;
    --bg: #0d1117;
    --bg-soft: #161b22;
    --border: #30363d;
    --user: #6cb6ff;
    --assistant: #57ab5a;
    --reasoning-bg: #1c1a10;
    --reasoning-rule: #ae9c3a;
  }
}
html { background: var(--bg); }
body {
  font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  max-width: 860px; margin: 32px auto; padding: 0 16px;
  color: var(--fg); background: var(--bg);
}
h1 { font-size: 1.7em; border-bottom: 1px solid var(--border); padding-bottom: .3em; }
h1, h2, h3, h4, h5, h6 { line-height: 1.25; }
.doc-meta { color: var(--fg-muted); font-size: .85em; margin: -.5em 0 2em; }
.turn { margin: 1.5em 0; }
.turn-role { font-weight: 600; text-transform: capitalize; margin: 0 0 .25em; }
.turn-role.user { color: var(--user); }
.turn-role.assistant { color: var(--assistant); }
.turn-role.reasoning { color: var(--fg-muted); font-style: italic; }
.turn-body { border-left: 3px solid var(--border); padding-left: 12px; overflow-x: auto; }
.turn-body.reasoning {
  border-left-color: var(--reasoning-rule); background: var(--reasoning-bg);
  padding: 8px 12px; border-radius: 4px;
}
.turn-body > :first-child { margin-top: 0; }
.turn-body > :last-child { margin-bottom: 0; }
.meta { color: var(--fg-muted); font-size: .85em; font-weight: 400; }

pre, code, kbd, samp { font-family: var(--mono); }
{{.CodeCSS}}
/* Wins over chroma's own .chroma background rule on specificity, so the code
   box keeps the export's surface colour in both themes. */
pre, pre.chroma {
  background: var(--bg-soft); padding: 12px; border-radius: 6px;
  border: 1px solid var(--border); font-size: .9em; line-height: 1.45;
  white-space: pre-wrap; overflow-wrap: anywhere;
}
code { background: var(--bg-soft); padding: .15em .35em; border-radius: 4px; font-size: .9em; }
pre code { background: none; padding: 0; font-size: inherit; }

table { border-collapse: collapse; width: 100%; margin: 1em 0; font-size: .95em; }
th, td { border: 1px solid var(--border); padding: 6px 12px; text-align: left; vertical-align: top; overflow-wrap: anywhere; }
th { background: var(--bg-soft); font-weight: 600; }
tbody tr:nth-child(even) { background: var(--bg-soft); }

blockquote {
  color: var(--fg-muted); border-left: 3px solid var(--border);
  margin: 1em 0; padding: 0 0 0 12px;
}
hr { border: 0; border-top: 1px solid var(--border); margin: 1.5em 0; }
img { max-width: 100%; height: auto; border-radius: 4px; }
a { color: var(--user); }
del { opacity: .7; }
ul, ol { padding-left: 1.6em; }
li > input[type="checkbox"] { margin-right: .4em; }
li:has(> input[type="checkbox"]) { list-style: none; margin-left: -1.4em; }
.attachments { margin: .6em 0 0; font-size: .9em; color: var(--fg-muted); }
.attachments-label { font-weight: 600; }
.attachment { display: inline-block; margin: .35em .5em 0 0; vertical-align: top; }
.attachment img { max-height: 220px; display: block; }
.attachment-name { display: block; font-size: .85em; }

@media print {
  body { max-width: none; margin: 0; padding: 0; }
  :root { --bg: #ffffff; }
  pre, table, blockquote, .turn { break-inside: avoid; page-break-inside: avoid; }
  h1, h2, h3, h4 { break-after: avoid; page-break-after: avoid; }
  a { text-decoration: none; }
  * { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
}
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<p class="doc-meta">Exported {{.ExportedAt}}</p>
{{.Rows}}
</body>
</html>`

func renderHTMLExport(doc exportDocument) ([]byte, error) {
	// One resolver for the whole document: it caches decoded assets and enforces
	// the per-document image cap, both of which a fresh instance per turn would
	// silently reset.
	media := doc.media()
	md := newHTMLMarkdown(media)
	var rows strings.Builder
	for _, m := range doc.Messages {
		if m.Reasoning != "" {
			fmt.Fprintf(&rows, `<div class="turn"><p class="turn-role reasoning">Reasoning</p><div class="turn-body reasoning">%s</div></div>`+"\n",
				markdownToHTML(md, m.Reasoning))
		}
		extra := ""
		if m.CreatedAt != "" {
			extra = fmt.Sprintf(` <span class="meta">%s</span>`, template.HTMLEscapeString(m.CreatedAt))
		}
		fmt.Fprintf(&rows, `<div class="turn"><p class="turn-role %s">%s%s</p><div class="turn-body">%s%s</div></div>`+"\n",
			template.HTMLEscapeString(m.Role), template.HTMLEscapeString(exportRoleLabel(m.Role)), extra,
			markdownToHTML(md, m.Content), attachmentsHTML(m.Attachments, media))
	}
	var buf bytes.Buffer
	tmpl := template.Must(template.New("export").Parse(htmlExportTemplate))
	if err := tmpl.Execute(&buf, map[string]interface{}{
		"Title":      doc.Title,
		"ExportedAt": doc.ExportedAt,
		"CodeCSS":    template.CSS(chromaStyleSheet()),
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
	if strings.TrimSpace(source) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(source), &buf); err != nil {
		return template.HTMLEscapeString(source)
	}
	return buf.String()
}

// attachmentsHTML renders the files uploaded on a turn: a thumbnail for every
// picture the server can read, a plain name for anything else.
func attachmentsHTML(atts []exportAttachment, media *exportMediaResolver) string {
	if len(atts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<div class="attachments"><span class="attachments-label">Attachments:</span> `)
	for _, a := range atts {
		img := &exportImage{alt: a.Name, src: a.Path}
		if media != nil {
			media.fill(img)
		}
		sb.WriteString(`<span class="attachment">`)
		if img.embeddable() {
			fmt.Fprintf(&sb, `<img src="%s" alt="%s">`, dataURI(img), template.HTMLEscapeString(a.Name))
		}
		fmt.Fprintf(&sb, `<span class="attachment-name">%s</span></span>`, template.HTMLEscapeString(a.Name))
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// dataURI encodes an embeddable image for an <img src>.
func dataURI(img *exportImage) string {
	return "data:" + img.mime + ";base64," + base64.StdEncoding.EncodeToString(img.data)
}

// newHTMLMarkdown builds the goldmark instance used for the HTML export: the
// shared GFM configuration plus the chroma code renderer and the image
// transformer. media may be nil, in which case no image is inlined.
func newHTMLMarkdown(media *exportMediaResolver) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithASTTransformers(
			util.Prioritized(&inlineImageTransformer{media: media}, 100),
		)),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(&chromaCodeRenderer{}, 100)),
		),
	)
}

// inlineImageTransformer rewrites image destinations that resolve to a local
// session asset into data: URIs, leaving remote ones untouched.
type inlineImageTransformer struct {
	media *exportMediaResolver
}

func (t *inlineImageTransformer) Transform(doc *gast.Document, reader text.Reader, _ parser.Context) {
	if t.media == nil {
		return
	}
	_ = gast.Walk(doc, func(n gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}
		node, ok := n.(*gast.Image)
		if !ok {
			return gast.WalkContinue, nil
		}
		img := &exportImage{src: string(node.Destination)}
		t.media.fill(img)
		if img.embeddable() {
			node.Destination = []byte(dataURI(img))
		}
		return gast.WalkContinue, nil
	})
}

// chromaCodeRenderer replaces goldmark's plain <pre><code> output with
// chroma-highlighted markup. Styles are inlined rather than emitted as classes
// so the exported file needs no stylesheet beyond the one in the template.
type chromaCodeRenderer struct{}

func (r *chromaCodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(gast.KindFencedCodeBlock, r.renderFenced)
	reg.Register(gast.KindCodeBlock, r.renderIndented)
}

func (r *chromaCodeRenderer) renderFenced(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	n, ok := node.(*gast.FencedCodeBlock)
	if !ok {
		return gast.WalkContinue, nil
	}
	writeHighlightedCode(w, string(n.Language(source)), codeBlockText(n, source))
	return gast.WalkSkipChildren, nil
}

func (r *chromaCodeRenderer) renderIndented(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	writeHighlightedCode(w, "", codeBlockText(node, source))
	return gast.WalkSkipChildren, nil
}

// writeHighlightedCode emits one <pre><code> block, highlighted when chroma
// knows the language and plain-escaped when it does not.
//
// Highlighting is emitted as CSS classes, not inline styles: the page ships both
// a light and a dark token palette (see chromaStyleSheet) and lets the reader's
// system choose. Inline styles would freeze one palette into the markup, and the
// light one is illegible on a dark page.
func writeHighlightedCode(w util.BufWriter, lang, source string) {
	lexer := lexers.Get(strings.TrimSpace(lang))
	if lexer == nil {
		writePlainCode(w, source)
		return
	}
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		writePlainCode(w, source)
		return
	}
	var buf bytes.Buffer
	formatter := chromahtml.New(chromahtml.WithClasses(true), chromahtml.PreventSurroundingPre(true))
	if err := formatter.Format(&buf, chromaStyle(exportChromaStyle), iterator); err != nil {
		writePlainCode(w, source)
		return
	}
	_, _ = w.WriteString(`<pre class="chroma"><code>`)
	_, _ = w.Write(buf.Bytes())
	_, _ = w.WriteString("</code></pre>\n")
}

// writePlainCode emits an unhighlighted block, used for a language chroma does
// not know.
func writePlainCode(w util.BufWriter, source string) {
	_, _ = w.WriteString("<pre><code>")
	_, _ = w.WriteString(template.HTMLEscapeString(source))
	_, _ = w.WriteString("</code></pre>\n")
}

// chromaStyle looks a palette up by name, falling back rather than returning nil.
func chromaStyle(name string) *chroma.Style {
	if style := styles.Get(name); style != nil {
		return style
	}
	return styles.Fallback
}

// chromaStyleSheet renders both token palettes: the light one unconditionally,
// the dark one inside a prefers-color-scheme query so it wins for a reader whose
// system is dark. Generated once at first use — the rule set is fixed.
func chromaStyleSheet() string {
	chromaCSSOnce.Do(func() {
		formatter := chromahtml.New(chromahtml.WithClasses(true))
		var light, dark bytes.Buffer
		_ = formatter.WriteCSS(&light, chromaStyle(exportChromaStyle))
		_ = formatter.WriteCSS(&dark, chromaStyle(exportChromaStyleDark))
		// Both palettes are scoped, rather than shipping the light one bare and
		// letting the dark one override it: the two styles do not define the same
		// token set, so an unscoped light rule for a token the dark palette leaves
		// alone (Go identifiers, class .nx) survived into dark mode and painted
		// near-black text on a near-black box. A browser with no stated preference
		// reports "light", so the light block is still the default.
		chromaCSS = "@media (prefers-color-scheme: light) {\n" + light.String() + "}\n" +
			"\n@media (prefers-color-scheme: dark) {\n" + dark.String() + "}\n"
	})
	return chromaCSS
}

var (
	chromaCSSOnce sync.Once
	chromaCSS     string
)
