//go:build gateway || gateway.telegram

package telegram

import (
	"strings"
	"testing"
)

func TestBuildRichMarkdown_PreservesNativeMarkdown(t *testing.T) {
	// The agent's markdown is GFM-compatible Rich Markdown and must pass through
	// verbatim — headings, tables, and code blocks are NOT downgraded.
	in := "# Title\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n```go\nfmt.Println(\"hi\")\n```"
	got := buildRichMarkdown(in, nil)
	if got != in {
		t.Fatalf("markdown must pass through verbatim when no tools.\nwant:\n%s\ngot:\n%s", in, got)
	}
}

func TestBuildRichMarkdown_PerToolCollapsedBlocksWithOutput(t *testing.T) {
	got := buildRichMarkdown("Done.", []toolCall{
		{name: "read_file", result: "package main"},
		{name: "bash", result: "exit 0"},
	})
	// Each tool is its own collapsed <details> block (two here), and the body shows output.
	if n := strings.Count(got, "<details>"); n != 2 {
		t.Fatalf("expected one <details> per tool (2), got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "read_file") || !strings.Contains(got, "package main") {
		t.Fatalf("expected read_file with its output, got:\n%s", got)
	}
	if !strings.Contains(got, "bash") || !strings.Contains(got, "exit 0") {
		t.Fatalf("expected bash with its output, got:\n%s", got)
	}
	// Tool blocks run before the answer, so they precede the LLM text.
	if strings.Index(got, "<details>") >= strings.Index(got, "Done.") {
		t.Fatalf("expected tool blocks to precede the answer text, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "Done.") {
		t.Fatalf("expected the answer text to come last, got:\n%s", got)
	}
}

func TestBuildRichMarkdown_FailedToolMarked(t *testing.T) {
	got := buildRichMarkdown("", []toolCall{{name: "bash", result: "boom", failed: true}})
	if !strings.Contains(got, "❌") || !strings.Contains(got, "bash") {
		t.Fatalf("expected failed tool marked with ❌, got:\n%s", got)
	}
}

func TestBuildRichMarkdown_NoToolsNoBlock(t *testing.T) {
	got := buildRichMarkdown("Plain answer.", []toolCall{})
	if strings.Contains(got, "<details>") {
		t.Fatalf("no <details> block expected when no tools ran, got:\n%s", got)
	}
	if got != "Plain answer." {
		t.Fatalf("want %q got %q", "Plain answer.", got)
	}
}

func TestBuildRichMarkdown_ToolsOnlyNoText(t *testing.T) {
	got := buildRichMarkdown("", []toolCall{{name: "bash", result: "ok"}})
	if !strings.Contains(got, "<details>") || !strings.Contains(got, "bash") {
		t.Fatalf("expected a tools block even with empty LLM text, got:\n%s", got)
	}
}

func TestBuildRichDraftMarkdown_ThinkingTagWhileToolRuns(t *testing.T) {
	got := buildRichDraftMarkdown("Working on it", "read_file")
	// While a tool runs, the draft shows a native <tg-thinking> placeholder (draft-only block).
	if !strings.Contains(got, "<tg-thinking>") || !strings.Contains(got, "</tg-thinking>") {
		t.Fatalf("expected <tg-thinking> placeholder, got:\n%s", got)
	}
	if !strings.Contains(got, "read_file") {
		t.Fatalf("expected running tool name in the thinking block, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "Working on it") {
		t.Fatalf("expected accumulated text before the thinking block, got:\n%s", got)
	}
}

func TestBuildRichDraftMarkdown_NoThinkingTagWhenIdle(t *testing.T) {
	got := buildRichDraftMarkdown("Just text", "")
	if strings.Contains(got, "<tg-thinking>") {
		t.Fatalf("no thinking block expected when no tool is running, got:\n%s", got)
	}
	if got != "Just text" {
		t.Fatalf("want %q got %q", "Just text", got)
	}
}
