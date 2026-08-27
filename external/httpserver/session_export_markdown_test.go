//go:build http

package httpserver

import (
	"strings"
	"testing"
)

// The block model is what every readable format walks, so a construct missing
// here is a construct missing from HTML, PDF and DOCX at once. These tests pin
// the constructs the chat window renders with remark-gfm — before GFM was wired
// in, a pipe table parsed as a paragraph of pipes and reached the documents as
// literal "| a | b |" text.

func TestMarkdownToBlocksParsesTables(t *testing.T) {
	md := strings.Join([]string{
		"| Format | Size | Notes |",
		"|--------|-----:|:-----:|",
		"| pdf    |  1 MB| fixed |",
		"| docx   | 40 kB| edit  |",
	}, "\n")

	blocks := markdownToBlocks(md, nil)
	if len(blocks) != 1 || blocks[0].kind != blockTable {
		t.Fatalf("expected one table block, got %+v", blocks)
	}
	tbl := blocks[0].table
	if tbl == nil {
		t.Fatal("table block carries no table")
	}
	if got := runsText(tbl.header[0].runs); got != "Format" {
		t.Errorf("header cell 0 = %q, want %q", got, "Format")
	}
	if len(tbl.header) != 3 {
		t.Errorf("header has %d cells, want 3", len(tbl.header))
	}
	if len(tbl.rows) != 2 {
		t.Fatalf("table has %d body rows, want 2", len(tbl.rows))
	}
	if got := runsText(tbl.rows[1][0].runs); got != "docx" {
		t.Errorf("row 1 cell 0 = %q, want %q", got, "docx")
	}
	// The delimiter row's colons carry the per-column alignment.
	want := []string{"L", "R", "C"}
	for i, w := range want {
		if got := tableAlign(tbl, i); got != w {
			t.Errorf("column %d alignment = %q, want %q", i, got, w)
		}
	}
}

// A pipe table must not survive as prose anywhere in the model — that was the
// old behaviour and it is what made the exported documents unreadable.
func TestMarkdownToBlocksLeavesNoPipeProse(t *testing.T) {
	blocks := markdownToBlocks("| a | b |\n|---|---|\n| 1 | 2 |", nil)
	for _, b := range blocks {
		if strings.Contains(runsText(b.runs), "|") {
			t.Fatalf("table leaked into prose runs: %q", runsText(b.runs))
		}
	}
}

func TestMarkdownToBlocksNestsLists(t *testing.T) {
	md := strings.Join([]string{
		"- top",
		"  - middle",
		"    - deep",
		"- back to top",
	}, "\n")

	blocks := markdownToBlocks(md, nil)
	if len(blocks) != 4 {
		t.Fatalf("expected 4 list items, got %d: %+v", len(blocks), blocks)
	}
	for i, want := range []struct {
		text   string
		indent int
	}{{"top", 0}, {"middle", 1}, {"deep", 2}, {"back to top", 0}} {
		if blocks[i].kind != blockListItem {
			t.Errorf("block %d is %q, want a list item", i, blocks[i].kind)
		}
		if got := runsText(blocks[i].runs); got != want.text {
			t.Errorf("item %d text = %q, want %q", i, got, want.text)
		}
		if blocks[i].indent != want.indent {
			t.Errorf("item %d indent = %d, want %d", i, blocks[i].indent, want.indent)
		}
	}
}

func TestMarkdownToBlocksReadsTaskLists(t *testing.T) {
	blocks := markdownToBlocks("- [x] done\n- [ ] pending\n- plain", nil)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 items, got %d", len(blocks))
	}
	if blocks[0].checked == nil || !*blocks[0].checked {
		t.Error("first item should be a checked task")
	}
	if blocks[1].checked == nil || *blocks[1].checked {
		t.Error("second item should be an unchecked task")
	}
	if blocks[2].checked != nil {
		t.Error("a plain bullet must not be reported as a task")
	}
	// The checkbox is state on the item, not text inside it.
	if got := runsText(blocks[0].runs); got != "done" {
		t.Errorf("task text = %q, want %q", got, "done")
	}
}

func TestMarkdownToBlocksKeepsInlineFormatting(t *testing.T) {
	blocks := markdownToBlocks("plain ~~gone~~ **bold** [link](https://example.com) <https://auto.example>", nil)
	if len(blocks) != 1 {
		t.Fatalf("expected one paragraph, got %d", len(blocks))
	}
	var strike, bold, linked, auto bool
	for _, r := range blocks[0].runs {
		switch {
		case r.text == "gone":
			strike = r.strike
		case r.text == "bold":
			bold = r.bold
		case r.text == "link":
			linked = r.link == "https://example.com"
		case strings.Contains(r.text, "auto.example"):
			auto = r.link != ""
		}
	}
	if !strike {
		t.Error("strikethrough was lost")
	}
	if !bold {
		t.Error("bold was lost")
	}
	if !linked {
		t.Error("link destination was dropped")
	}
	if !auto {
		t.Error("autolink was not recognised")
	}
}

// A blockquote used to be flattened into one run of prose, which glued its
// paragraphs together and swallowed any list inside it.
func TestMarkdownToBlocksKeepsQuoteStructure(t *testing.T) {
	blocks := markdownToBlocks("> first paragraph\n>\n> second paragraph", nil)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 quoted paragraphs, got %d: %+v", len(blocks), blocks)
	}
	for i, b := range blocks {
		if b.quote != 1 {
			t.Errorf("block %d quote depth = %d, want 1", i, b.quote)
		}
	}
	if got := runsText(blocks[0].runs); got != "first paragraph" {
		t.Errorf("first quoted paragraph = %q", got)
	}
}

func TestMarkdownToBlocksReadsThematicBreakAndFenceLanguage(t *testing.T) {
	blocks := markdownToBlocks("---\n\n```python\nprint(1)\n```", nil)
	if len(blocks) != 2 {
		t.Fatalf("expected a rule and a code block, got %+v", blocks)
	}
	if blocks[0].kind != blockRule {
		t.Errorf("first block is %q, want %q", blocks[0].kind, blockRule)
	}
	if blocks[1].kind != blockCode || blocks[1].lang != "python" {
		t.Errorf("code block = %q lang %q", blocks[1].kind, blocks[1].lang)
	}
}

func TestMarkdownToBlocksPromotesStandaloneImages(t *testing.T) {
	blocks := markdownToBlocks("![a diagram](shot.png)", nil)
	if len(blocks) != 1 || blocks[0].kind != blockImage {
		t.Fatalf("expected one image block, got %+v", blocks)
	}
	img := blocks[0].image
	if img == nil || img.alt != "a diagram" || img.src != "shot.png" {
		t.Fatalf("image block lost its metadata: %+v", img)
	}
	// An image sharing its line with text stays inline instead.
	inline := markdownToBlocks("see ![a diagram](shot.png) here", nil)
	if len(inline) != 1 || inline[0].kind != blockParagraph {
		t.Fatalf("an image beside text should stay in the paragraph, got %+v", inline)
	}
}
