//go:build http

package httpserver

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Syntax highlighting for exported code blocks. The SPA highlights code with
// highlight.js; this is the server-side equivalent, and it feeds all three
// readable formats from one place: PDF colours runs with SetTextColor and DOCX
// with <w:color>, both from the runs this file returns; HTML goes through
// chroma's own class-based formatter (see session_export_html.go) so it can ship
// a light and a dark palette. Either way one tokeniser decides what a Go snippet
// looks like, whichever format the user picks.

// The palettes the export paints code with. PDF and DOCX always sit on a white
// page, so they only ever use the light one; HTML ships both and lets the
// reader's system pick, because a light palette on a dark page is unreadable —
// "github" blue (#0550AE) on #161b22 lands around 1.6:1 contrast.
const (
	exportChromaStyle     = "github"
	exportChromaStyleDark = "github-dark"
)

// highlightCode tokenises source with the lexer for lang and returns one slice
// of coloured runs per source line.
//
// It returns nil when no lexer recognises the language, which tells the callers
// to draw the block as plain monospaced text: a fallback lexer would paint the
// whole snippet one flat colour and merely look broken.
func highlightCode(lang, source string) [][]exportRun {
	if strings.TrimSpace(source) == "" {
		return nil
	}
	lexer := lexers.Get(strings.TrimSpace(lang))
	if lexer == nil {
		// No fence info string, or a language chroma does not know. Do not fall
		// back to lexers.Analyse: on a short snippet it guesses badly, and a
		// wrongly-coloured block reads worse than an uncoloured one.
		return nil
	}
	lexer = chroma.Coalesce(lexer)
	style := styles.Get(exportChromaStyle)
	if style == nil {
		style = styles.Fallback
	}
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return nil
	}

	lines := [][]exportRun{{}}
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		// Token values carry their own newlines; split so each output line holds
		// exactly the runs that belong on it.
		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				lines = append(lines, []exportRun{})
			}
			if part == "" {
				continue
			}
			last := len(lines) - 1
			lines[last] = append(lines[last], exportRun{
				text:   part,
				code:   true,
				color:  chromaColour(entry),
				bold:   entry.Bold == chroma.Yes,
				italic: entry.Italic == chroma.Yes,
			})
		}
	}
	// Tokenised source ends with a newline more often than not; an empty trailing
	// line would draw as a blank row inside the code box.
	for len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// chromaColour renders a style entry's foreground as "RRGGBB", or "" when the
// entry leaves the colour to the renderer's default.
func chromaColour(entry chroma.StyleEntry) string {
	if !entry.Colour.IsSet() {
		return ""
	}
	return fmt.Sprintf("%02X%02X%02X", entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue())
}

// codeLinesOf returns the highlighted lines of a code block, falling back to
// plain uncoloured runs when the language had no lexer. Every renderer goes
// through here so an unhighlighted block still lays out line by line.
func codeLinesOf(b exportBlock) [][]exportRun {
	if len(b.codeLines) > 0 {
		return b.codeLines
	}
	var out [][]exportRun
	for _, line := range strings.Split(b.text, "\n") {
		if line == "" {
			out = append(out, nil)
			continue
		}
		out = append(out, []exportRun{{text: line, code: true}})
	}
	return out
}

// hexToRGB converts an "RRGGBB" colour into the byte triple fpdf wants. A
// malformed or empty value reports ok=false so the caller keeps its default.
func hexToRGB(hex string) (r, g, b int, ok bool) {
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	var v int
	if _, err := fmt.Sscanf(hex, "%06X", &v); err != nil {
		return 0, 0, 0, false
	}
	return v >> 16 & 0xFF, v >> 8 & 0xFF, v & 0xFF, true
}
