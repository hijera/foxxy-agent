//go:build gateway || gateway.telegram

package telegram

// Edge cases of the outbound rendering step. The happy path - an answer
// reaching the chat in Telegram's syntax without a word of Telegram in the
// prompt - is features/gateway_session_identity.feature.

import (
	"strings"
	"testing"
)

func TestMdToTelegramLeavesCodeBlocksAlone(t *testing.T) {
	in := "Before **bold**\n\n```go\nx := **p\n// # not a heading\n* not a bullet\n```\n\nAfter **bold**"
	got := mdToTelegram(in)
	want := "Before *bold*\n\n```go\nx := **p\n// # not a heading\n* not a bullet\n```\n\nAfter *bold*"
	if got != want {
		t.Fatalf("mdToTelegram(%q) =\n%q\nwant\n%q", in, got, want)
	}
}

// Every conversion rule the function applies is represented inside the block
// below, so a single failure names the rule that leaked into code.
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

func TestMdToTelegramConversions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"heading", "## Findings", "*Findings*"},
		{"deep heading", "###### Small", "*Small*"},
		{"double star", "a **b** c", "a *b* c"},
		{"double underscore", "a __b__ c", "a _b_ c"},
		{"bullet", "* one\n* two", "• one\n• two"},
		{"indented bullet", "- top\n  * nested", "- top\n  • nested"},
		{"rule", "a\n---\nb", "a\n────────────────\nb"},
		{"inline code untouched", "call `x_y` now", "call `x_y` now"},
		{"a hash that is not a heading", "issue #12 is open", "issue #12 is open"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mdToTelegram(c.in); got != c.want {
				t.Fatalf("mdToTelegram(%q) = %q, want %q", c.in, got, c.want)
			}
		})
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

func TestMdToTelegramFlattensTables(t *testing.T) {
	in := "| Key | Value |\n|-----|-------|\n| a   | 1     |"
	got := mdToTelegram(in)
	want := "Key  │  Value\na  │  1"
	if got != want {
		t.Fatalf("mdToTelegram(table) = %q, want %q", got, want)
	}
}

// An unclosed fence is what a truncated answer looks like. Nothing after it may
// be rewritten either: the model was writing code when it stopped.
func TestMdToTelegramKeepsAnUnclosedCodeBlock(t *testing.T) {
	in := "look:\n\n```go\nx := **p"
	if got := mdToTelegram(in); got != in {
		t.Fatalf("mdToTelegram(unclosed) = %q, want %q", got, in)
	}
}

// The placeholder is an internal token; none of it may reach Telegram.
func TestMdToTelegramLeavesNoPlaceholderBehind(t *testing.T) {
	got := mdToTelegram("```\ncode\n```\ntail with `inline` code\n")
	if strings.Contains(got, "CODE") || strings.ContainsRune(got, '\x00') {
		t.Fatalf("a placeholder token leaked into the output: %q", got)
	}
}

// splitMessage chunks on length, and a converter that appends a newline per
// call would drift the boundaries.
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

// The live preview is sent with no parse mode, so a marker there is punctuation
// on the reader's screen rather than formatting: emphasis is dropped instead.
func TestMdToPlainPreviewDropsEmphasis(t *testing.T) {
	in := "## Findings\n\n**two** of them"
	want := "Findings\n\ntwo of them"
	if got := mdToPlainPreview(in); got != want {
		t.Fatalf("mdToPlainPreview(%q) = %q, want %q", in, got, want)
	}
}

func TestBuildStreamPreviewRendersTheAccumulatedText(t *testing.T) {
	if got := buildStreamPreview("## Findings", ""); got != "Findings…" {
		t.Fatalf("preview without a tool = %q, want %q", got, "Findings…")
	}
	if got := buildStreamPreview("", "read"); got != "⚙️ read…" {
		t.Fatalf("preview with no text = %q", got)
	}
	if got := buildStreamPreview("## Findings", "read"); got != "Findings\n\n⚙️ read…" {
		t.Fatalf("preview with a tool = %q", got)
	}
}

// Inline code carries the same weight as a fenced block: an identifier in
// backticks is what the model meant, not emphasis to rewrite.
func TestMdToTelegramLeavesInlineCodeAlone(t *testing.T) {
	in := "Use `**p` for a pointer and `__name__` for the dunder, then **stress** it"
	want := "Use `**p` for a pointer and `__name__` for the dunder, then *stress* it"
	if got := mdToTelegram(in); got != want {
		t.Fatalf("mdToTelegram(%q) = %q, want %q", in, got, want)
	}
}

// A fence longer than three characters is how a model quotes Markdown that
// contains a fence of its own; closing on the inner one would spill the rest
// of the answer into the block and rewrite what followed as prose.
func TestMdToTelegramHonoursTheOpeningFenceLength(t *testing.T) {
	in := "A fence:\n\n````md\n```\n**inner**\n```\n````\n\ndone **after**"
	want := "A fence:\n\n````md\n```\n**inner**\n```\n````\n\ndone *after*"
	if got := mdToTelegram(in); got != want {
		t.Fatalf("mdToTelegram(long fence) = %q, want %q", got, want)
	}
}

func TestMdToTelegramHonoursTildeFences(t *testing.T) {
	in := "tilde:\n\n~~~go\nx := __p__\n~~~\n\nafter **bold**"
	want := "tilde:\n\n~~~go\nx := __p__\n~~~\n\nafter *bold*"
	if got := mdToTelegram(in); got != want {
		t.Fatalf("mdToTelegram(tilde fence) = %q, want %q", got, want)
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

func TestMaskCodeRoundTrips(t *testing.T) {
	in := "before\n```go\ncode one\n```\nmiddle `inline`\n```\ncode two\n```\nafter"
	masked, code := maskCode(in)

	if len(code) != 3 {
		t.Fatalf("maskCode() set aside %d pieces, want 3 (two blocks and a span)", len(code))
	}
	if strings.Contains(masked, "code one") || strings.Contains(masked, "code two") || strings.Contains(masked, "inline") {
		t.Fatalf("code was left in the masked text: %q", masked)
	}
	if restored := restoreCode(masked, code); restored != in {
		t.Fatalf("round trip changed the text:\n got: %q\nwant: %q", restored, in)
	}
}

// Ten or more blocks exercise the placeholder numbering past a single digit.
func TestMaskCodeKeepsManyBlocksDistinct(t *testing.T) {
	var b strings.Builder
	for i := range 12 {
		b.WriteString("```\n")
		b.WriteString(strings.Repeat("x", i+1))
		b.WriteString("\n```\n")
	}
	in := b.String()
	masked, code := maskCode(in)
	if len(code) != 12 {
		t.Fatalf("maskCode() set aside %d blocks, want 12", len(code))
	}
	if restored := restoreCode(masked, code); restored != in {
		t.Fatalf("round trip changed the text with 12 blocks:\n got: %q\nwant: %q", restored, in)
	}
}
