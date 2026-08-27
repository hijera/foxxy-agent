//go:build http

package httpserver

import (
	"strings"
	"testing"
)

func TestHighlightCodeColoursKnownLanguages(t *testing.T) {
	lines := highlightCode("go", "package main\n\nfunc main() {}\n")
	if len(lines) == 0 {
		t.Fatal("Go source produced no highlighted lines")
	}
	var coloured, keyword bool
	for _, line := range lines {
		for _, r := range line {
			if !r.code {
				t.Errorf("highlighted run %q is not marked as code", r.text)
			}
			if r.color != "" {
				coloured = true
			}
			if strings.TrimSpace(r.text) == "package" && r.color != "" {
				keyword = true
			}
		}
	}
	if !coloured {
		t.Error("no run carries a colour")
	}
	if !keyword {
		t.Error("the package keyword was not highlighted")
	}
}

// A language chroma does not know must fall through to plain text rather than
// being painted one flat colour by the fallback lexer, which looks broken.
func TestHighlightCodeSkipsUnknownLanguages(t *testing.T) {
	for _, lang := range []string{"", "   ", "not-a-real-language"} {
		if got := highlightCode(lang, "some text\n"); got != nil {
			t.Errorf("lang %q produced %d highlighted lines, want none", lang, len(got))
		}
	}
}

// Highlighting must never lose or reorder source; the reader has to be able to
// copy the snippet back out of the document.
func TestHighlightCodePreservesEverySourceLine(t *testing.T) {
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}"
	lines := highlightCode("go", src)

	want := strings.Split(src, "\n")
	if len(lines) != len(want) {
		t.Fatalf("highlighted %d lines, source has %d", len(lines), len(want))
	}
	for i := range want {
		if got := runsText(lines[i]); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}
}

// codeLinesOf is what the renderers call; it has to lay out an unhighlighted
// block line by line too, or an unknown language would render as one long line.
func TestCodeLinesOfFallsBackToPlainLines(t *testing.T) {
	b := exportBlock{kind: blockCode, text: "one\n\nthree"}
	lines := codeLinesOf(b)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if runsText(lines[0]) != "one" || runsText(lines[2]) != "three" {
		t.Errorf("plain fallback mangled the source: %q / %q", runsText(lines[0]), runsText(lines[2]))
	}
	if len(lines[1]) != 0 {
		t.Error("the blank line should carry no runs")
	}
}

func TestHexToRGB(t *testing.T) {
	r, g, b, ok := hexToRGB("1A7F37")
	if !ok || r != 0x1A || g != 0x7F || b != 0x37 {
		t.Errorf("hexToRGB(1A7F37) = %d,%d,%d ok=%v", r, g, b, ok)
	}
	for _, bad := range []string{"", "abc", "1A7F3", "1A7F37F"} {
		if _, _, _, ok := hexToRGB(bad); ok {
			t.Errorf("hexToRGB(%q) should have failed", bad)
		}
	}
}
