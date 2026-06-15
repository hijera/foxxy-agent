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

func TestBuildRichMarkdown_AppendsCollapsibleToolsBlock(t *testing.T) {
	got := buildRichMarkdown("Done.", []string{"read_file", "bash"})
	// A collapsible <details> block lists the executed tools with the count in the summary.
	if !strings.Contains(got, "<details>") || !strings.Contains(got, "</details>") {
		t.Fatalf("expected a <details> block, got:\n%s", got)
	}
	if !strings.Contains(got, "<summary>") || !strings.Contains(got, "(2)") {
		t.Fatalf("expected a <summary> with tool count (2), got:\n%s", got)
	}
	if !strings.Contains(got, "read_file") || !strings.Contains(got, "bash") {
		t.Fatalf("expected both tool names listed, got:\n%s", got)
	}
	if !strings.HasPrefix(got, "Done.") {
		t.Fatalf("expected LLM text to precede the tools block, got:\n%s", got)
	}
}

func TestBuildRichMarkdown_NoToolsNoBlock(t *testing.T) {
	got := buildRichMarkdown("Plain answer.", []string{})
	if strings.Contains(got, "<details>") {
		t.Fatalf("no <details> block expected when no tools ran, got:\n%s", got)
	}
	if got != "Plain answer." {
		t.Fatalf("want %q got %q", "Plain answer.", got)
	}
}

func TestBuildRichMarkdown_ToolsOnlyNoText(t *testing.T) {
	got := buildRichMarkdown("", []string{"bash"})
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
