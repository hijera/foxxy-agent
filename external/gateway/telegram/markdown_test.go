//go:build gateway || gateway.telegram

package telegram

import (
	"strings"
	"testing"
)

// The converter's contract is "code blocks are always preserved verbatim": what
// the model put between fences is source, not prose, and rewriting it changes
// the code the user is being shown. Every conversion rule the function applies
// is represented inside the block below, so a single failure names the rule that
// leaked in.
func TestMdToTelegramKeepsCodeBlockContentVerbatim(t *testing.T) {
	block := "```go\n" +
		"# not a heading\n" +
		"**not bold**\n" +
		"__not italic__\n" +
		"* not a bullet\n" +
		"---\n" +
		"| not | a table |\n" +
		"```"
	got := mdToTelegram(block)

	for _, verbatim := range []string{
		"# not a heading",
		"**not bold**",
		"__not italic__",
		"* not a bullet",
		"\n---\n",
		"| not | a table |",
	} {
		if !strings.Contains(got, verbatim) {
			t.Errorf("code block content %q was rewritten; got:\n%s", verbatim, got)
		}
	}
}

// Protecting code must not switch the conversions off for the prose around it.
func TestMdToTelegramConvertsProseAroundACodeBlock(t *testing.T) {
	in := "# Title\n" +
		"**bold** before\n" +
		"```\n# untouched\n```\n" +
		"**bold** after\n" +
		"* bullet after\n"
	got := mdToTelegram(in)

	for _, want := range []string{"*Title*", "*bold* before", "*bold* after", "• bullet after"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in the converted prose; got:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "# untouched") {
		t.Errorf("the code block was rewritten; got:\n%s", got)
	}
}

// Two blocks must come back in their own places rather than swapped or merged:
// the placeholder has to be unique per block.
func TestMdToTelegramRestoresSeveralBlocksInOrder(t *testing.T) {
	in := "```\nfirst **one**\n```\nmiddle **two**\n```\nthird **three**\n```\n"
	got := mdToTelegram(in)

	first := strings.Index(got, "first **one**")
	middle := strings.Index(got, "middle *two*")
	third := strings.Index(got, "third **three**")
	if first < 0 || middle < 0 || third < 0 {
		t.Fatalf("missing a segment (first=%d middle=%d third=%d); got:\n%s", first, middle, third, got)
	}
	if first >= middle || middle >= third {
		t.Fatalf("segments came back out of order (first=%d middle=%d third=%d); got:\n%s", first, middle, third, got)
	}
}

// A block the model never closed still has to survive: truncation is exactly when
// the user most wants to see the raw text.
func TestMdToTelegramKeepsAnUnclosedCodeBlock(t *testing.T) {
	in := "intro **bold**\n```\n# still code\n**still code**\n"
	got := mdToTelegram(in)

	if !strings.Contains(got, "intro *bold*") {
		t.Errorf("prose before an unclosed block was not converted; got:\n%s", got)
	}
	for _, verbatim := range []string{"# still code", "**still code**"} {
		if !strings.Contains(got, verbatim) {
			t.Errorf("unclosed block content %q was rewritten; got:\n%s", verbatim, got)
		}
	}
}

// The placeholder is an internal token; none of it may reach Telegram.
func TestMdToTelegramLeavesNoPlaceholderBehind(t *testing.T) {
	got := mdToTelegram("```\ncode\n```\ntail\n")
	if strings.Contains(got, "BLOCK") || strings.ContainsRune(got, '\x00') {
		t.Fatalf("a placeholder token leaked into the output: %q", got)
	}
}

// Round-tripping through the extractor must not grow or shrink the message:
// splitMessage chunks on length, and a converter that appends a newline per call
// would drift the boundaries.
func TestMdToTelegramDoesNotAddTrailingNewlines(t *testing.T) {
	cases := []string{
		"plain text",
		"```\ncode\n```",
		"text\n```\ncode\n```",
		"",
	}
	for _, in := range cases {
		got := mdToTelegram(in)
		if strings.HasSuffix(in, "\n") {
			continue
		}
		if strings.HasSuffix(got, "\n") {
			t.Errorf("mdToTelegram(%q) grew a trailing newline: %q", in, got)
		}
	}
}

func TestMdToTelegramConvertsProse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"atx header", "## Section\n", "*Section*"},
		{"double star bold", "a **b** c", "a *b* c"},
		{"double underscore italic", "a __b__ c", "a _b_ c"},
		{"asterisk bullet", "* item\n", "• item"},
		{"horizontal rule", "before\n---\nafter", "────────────────"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mdToTelegram(tc.in); !strings.Contains(got, tc.want) {
				t.Fatalf("mdToTelegram(%q) = %q, want it to contain %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestConvertTablesFlattensRowsAndDropsTheAlignmentRow(t *testing.T) {
	in := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	got := convertTables(in)

	if strings.Contains(got, "|") {
		t.Errorf("pipes survived the table conversion: %q", got)
	}
	if strings.Contains(got, "---") {
		t.Errorf("the alignment row survived: %q", got)
	}
	for _, want := range []string{"a  │  b", "1  │  2"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected row %q in %q", want, got)
		}
	}
}

// A line that merely mentions a pipe is prose, not a table row.
func TestConvertTablesLeavesProseWithPipesAlone(t *testing.T) {
	in := "run a | b to pipe it\n"
	if got := convertTables(in); !strings.Contains(got, "a | b") {
		t.Fatalf("prose containing a pipe was reflowed as a table row: %q", got)
	}
}

func TestCollapseBlankLines(t *testing.T) {
	if got := collapseBlankLines("a\n\n\n\n\nb"); got != "a\n\nb" {
		t.Fatalf("collapseBlankLines() = %q, want %q", got, "a\n\nb")
	}
	if got := collapseBlankLines("a\n\nb"); got != "a\n\nb" {
		t.Fatalf("collapseBlankLines() changed an already-collapsed gap: %q", got)
	}
}

func TestExtractCodeBlocksRoundTrips(t *testing.T) {
	in := "before\n```go\ncode one\n```\nmiddle\n```\ncode two\n```\nafter"
	blocks, stripped := extractCodeBlocks(in)

	if len(blocks) != 2 {
		t.Fatalf("extractCodeBlocks() found %d blocks, want 2", len(blocks))
	}
	if strings.Contains(stripped, "code one") || strings.Contains(stripped, "code two") {
		t.Fatalf("block bodies were left in the stripped text: %q", stripped)
	}
	restored := restoreCodeBlocks(stripped, blocks)
	if restored != in {
		t.Fatalf("round trip changed the text:\n got: %q\nwant: %q", restored, in)
	}
}

// Ten or more blocks exercise the placeholder numbering past a single digit.
func TestExtractCodeBlocksKeepsManyBlocksDistinct(t *testing.T) {
	var b strings.Builder
	for i := range 12 {
		b.WriteString("```\n")
		b.WriteString(strings.Repeat("x", i+1))
		b.WriteString("\n```\n")
	}
	in := b.String()
	blocks, stripped := extractCodeBlocks(in)
	if len(blocks) != 12 {
		t.Fatalf("extractCodeBlocks() found %d blocks, want 12", len(blocks))
	}
	if restored := restoreCodeBlocks(stripped, blocks); restored != in {
		t.Fatalf("round trip changed the text with 12 blocks:\n got: %q\nwant: %q", restored, in)
	}
}
