package autocomplete

import (
	"strings"
	"testing"
)

func TestDecideMultiLine(t *testing.T) {
	cases := []struct {
		name           string
		prefix, suffix string
		want           bool
	}{
		{"code right of the caret forces a single line", "foo(", ")\n}", false},
		{"end of a line that opened a block", "func f() {", "\n}", true},
		{"python block opener", "def f():", "\n", true},
		{"empty line inside a body", "func f() {\n\t", "\n}", true},
		{"middle of a statement", "\treturn ", "\n}", false},
		{"bare else keyword", "} else", "\n", true},
		{"trailing comma in a literal", "x := []int{1,", "\n}", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideMultiLine(tc.prefix, tc.suffix); got != tc.want {
				t.Fatalf("decideMultiLine() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLineStops(t *testing.T) {
	if got := lineStops(false, "\n}"); len(got) != 1 || got[0] != "\n" {
		t.Fatalf("single-line stops = %q", got)
	}
	got := lineStops(true, "\n\treturn nil\n}\n")
	if len(got) != 2 || got[0] != "\n\treturn nil" || got[1] != "\n\n\n" {
		t.Fatalf("block stops = %q, want the exact next suffix line then a blank-line run", got)
	}
	if got := lineStops(true, "   \n"); len(got) != 1 || got[0] != "\n\n\n" {
		t.Fatalf("block stops without a suffix line = %q", got)
	}
	// A lone closing bracket would end an unindented block on its own "}".
	if got := lineStops(true, "\n}\n\nfunc g() {}"); len(got) != 1 || got[0] != "\n\n\n" {
		t.Fatalf("block stops with a bare closer next = %q", got)
	}
	if got := capStops([]string{"a", "", "a", "b", "c", "d", "e"}, 4); strings.Join(got, ",") != "a,b,c,d" {
		t.Fatalf("capStops() = %q", got)
	}
}

func TestTrimSuffixOverlap(t *testing.T) {
	cases := []struct {
		name, out, suffix, want string
	}{
		{"closing brace already present", "a + b\n}", "\n}", "a + b"},
		{"closing paren already present", "arg)", ")", "arg"},
		{"multi-char overlap", "foo(bar", "bar)", "foo("},
		{"no overlap", "a + b", "\nreturn x", "a + b"},
		{"single letter is not an overlap", "return a", "a\n}", "return a"},
		{"differently indented closer", "x\n\t}", "\n}", "x\n\t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimSuffixOverlap(tc.out, tc.suffix); got != tc.want {
				t.Fatalf("trimSuffixOverlap(%q, %q) = %q, want %q", tc.out, tc.suffix, got, tc.want)
			}
		})
	}
}

func TestTruncateBlockStopsAtDedent(t *testing.T) {
	lines := []string{"", "\t\tdo()", "\t}", "}", "", "func next() {"}
	got := truncateBlock(lines, "\t")
	if strings.Join(got, "|") != "|\t\tdo()|\t}" {
		t.Fatalf("truncateBlock() = %q", got)
	}
	long := make([]string, 30)
	for i := range long {
		long[i] = "x"
	}
	if got := truncateBlock(long, ""); len(got) != autocompleteMaxLines {
		t.Fatalf("cap = %d lines", len(got))
	}
}

func TestCutoffReached(t *testing.T) {
	if cutoffReached("a + ", false, "") {
		t.Fatal("single-line: still on the first line")
	}
	if !cutoffReached("a + b\nfmt.Pr", false, "") {
		t.Fatal("single-line: a line break after content ends it")
	}
	if cutoffReached("\n\t\tdo()\n\t}\n", true, "\t") {
		t.Fatal("block: still inside the caret's scope")
	}
	if !cutoffReached("\n\t\tdo()\n\t}\n}\n", true, "\t") {
		t.Fatal("block: a dedented complete line ends it")
	}
	if cutoffReached("```go\n\t\tdo()\n", true, "\t") {
		t.Fatal("block: an opening fence must not count as a dedent")
	}
}

func TestCleanCompletion(t *testing.T) {
	cases := []struct {
		name           string
		raw            string
		prefix, suffix string
		multiLine      bool
		want           string
	}{
		{"plain continuation", "a + b", "\treturn ", "\n}", false, "a + b"},
		{"fence stripped", "```go\nreturn a + b\n```", "\t", "\n}", false, "return a + b"},
		{"caret-line echo dropped", "if err != nil {", "\tif err ", "\n", false, "!= nil {"},
		{"indentation preserved on an empty caret line", "\treturn x", "func f() {\n", "\n}", true, "\treturn x"},
		{"typed indentation is not doubled", "\t\treturn x", "func f() {\n\t\t", "\n}", true, "return x"},
		{"leading space after a trailing space is dropped", " a + b", "\treturn ", "\n}", false, "a + b"},
		{"closing brace in the suffix is not re-typed", "a + b\n}", "\treturn ", "\n}", true, "a + b"},
		{"single-line mode keeps the first line", "a + b\nfmt.Println(a)", "\treturn ", "\n}", false, "a + b"},
		{"single-line mode yields nothing when the model starts on the next line", "\nfoo()", "bar()", "\n", false, ""},
		{"block cut at the enclosing scope", "\n\t\tdo()\n\t}\n}\n\nfunc g() {}", "\tif x {", "\n}", true, "\n\t\tdo()\n\t}"},
		{"closing punctuation only is nothing", ")\n}", "foo(a", ")\n}", true, ""},
		{"a repeat of the next line is nothing", "return nil", "\tx := 1\n\t", "\n\treturn nil\n}", true, ""},
		{"trailing blank lines trimmed", "return a\n\n\n", "\t", "", true, "return a"},
		{"empty reply", "   \n  ", "\t", "", true, ""},
		{"CRLF normalised", "return a\r\n\treturn b", "func f() {\n\t", "", true, "return a\n\treturn b"},
		{"a second line at column zero is outside the caret's scope", "return a\nfunc g() {}", "func f() {\n\t", "", true, "return a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanCompletion(tc.raw, tc.prefix, tc.suffix, tc.multiLine); got != tc.want {
				t.Fatalf("cleanCompletion() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSmallHelpers(t *testing.T) {
	if !startsMidIdentifier("bar()") || startsMidIdentifier(")") || startsMidIdentifier("") || startsMidIdentifier(" x") {
		t.Fatal("startsMidIdentifier")
	}
	if !isClosingOnly(" )\n}") || isClosingOnly("x)") || isClosingOnly("") {
		t.Fatal("isClosingOnly")
	}
	if commentLeader("python") != "#" || commentLeader("go") != "//" || commentLeader("sql") != "--" || commentLeader("") != "//" {
		t.Fatal("commentLeader")
	}
	if caretLine("func f() {\n\tif err ") != "if err " || caretIndent("a\n\t\tx") != "\t\t" {
		t.Fatal("caretLine / caretIndent")
	}
}

func TestTailBytesKeepsWholeLines(t *testing.T) {
	if got := tailBytes("aaa one\ntwo\nthree", 12); got != "two\nthree" {
		t.Fatalf("tailBytes() = %q", got)
	}
	if got := tailBytes(strings.Repeat("b", 20)+"\nc", 12); got != "bbbbbbbbbb\nc" {
		t.Fatalf("tailBytes() over-trimmed a long fragment: %q", got)
	}
	if got := tailBytes("short", 100); got != "short" {
		t.Fatalf("tailBytes() under the cap = %q", got)
	}
	if got := tailBytes("ЖЖЖЖ", 5); !strings.HasSuffix("ЖЖЖЖ", got) {
		t.Fatalf("tailBytes() mangled a rune: %q", got)
	}
}

func TestHeadBytesKeepsWholeLines(t *testing.T) {
	if got := headBytes("one\ntwo\nthree\nfour", 16); got != "one\ntwo\nthree" {
		t.Fatalf("headBytes() = %q", got)
	}
	if got := headBytes("a\n"+strings.Repeat("b", 20), 12); got != "a\nbbbbbbbbbb" {
		t.Fatalf("headBytes() over-trimmed a long fragment: %q", got)
	}
	if got := headBytes("ЖЖЖЖ", 5); !strings.HasPrefix("ЖЖЖЖ", got) {
		t.Fatalf("headBytes() mangled a rune: %q", got)
	}
}
