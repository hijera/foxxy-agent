//go:build gateway || gateway.telegram

package telegram

import (
	"fmt"
	"strings"
)

// Rich Messages (Bot API 10.1) let the gateway send the agent's native Markdown
// almost verbatim — headings, tables, task lists, fenced code, footnotes and LaTeX
// all render natively, instead of being downgraded to Telegram legacy Markdown.
//
// Two builders produce the InputRichMessage.markdown string:
//   - buildRichMarkdown   → the final, persistent message (uses a collapsible
//     <details> block to list executed tools).
//   - buildRichDraftMarkdown → the ephemeral streaming preview sent via
//     sendRichMessageDraft (uses the draft-only <tg-thinking> block while a tool runs).

// toolCall is one tool execution captured during a turn: its name, the JSON args,
// the result preview, and whether it failed.
type toolCall struct {
	name   string
	args   string
	result string
	failed bool
}

// buildRichMarkdown assembles the final rich-message markdown from the accumulated
// LLM text and the tools that ran during the turn. Tool blocks come first (they ran
// before the answer), each as its own collapsed-by-default <details> block showing its
// output; the LLM answer follows, passed through verbatim (already GitHub-flavored Markdown).
func buildRichMarkdown(llmText string, tools []toolCall) string {
	text := strings.TrimSpace(llmText)
	details := richToolsDetails(tools)
	if details == "" {
		return text
	}
	if text == "" {
		return details
	}
	return details + "\n\n" + text
}

// richToolsDetails renders every executed tool as its own collapsed <details> block:
// the summary is the tool name (❌ on failure), the body is the tool's output (or its
// args when no output was captured) in a fenced code block. Returns "" when no tools ran.
func richToolsDetails(tools []toolCall) string {
	if len(tools) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range tools {
		icon := "🛠"
		if t.failed {
			icon = "❌"
		}
		name := strings.TrimSpace(t.name)
		if name == "" {
			name = "tool"
		}
		fmt.Fprintf(&b, "<details><summary>%s %s</summary>\n\n", icon, name)
		body := strings.TrimSpace(t.result)
		if body == "" {
			body = strings.TrimSpace(t.args)
		}
		if body != "" {
			// Keep messages within Telegram limits and never let the body's own
			// fences break out of the surrounding code block.
			body = strings.ReplaceAll(truncate(body, 1500), "```", "ʼʼʼ")
			fmt.Fprintf(&b, "```\n%s\n```\n\n", body)
		}
		b.WriteString("</details>\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildRichDraftMarkdown assembles the streaming-preview markdown. While a tool is
// executing, a draft-only <tg-thinking> placeholder is appended below the accumulated
// LLM text so the user sees a native "Thinking…" animation. When no tool is running,
// the accumulated text is returned as-is.
func buildRichDraftMarkdown(llmText, currentTool string) string {
	text := strings.TrimRight(llmText, " \t\n")
	if strings.TrimSpace(currentTool) == "" {
		return strings.TrimSpace(text)
	}
	thinking := "<tg-thinking>⚙️ " + currentTool + "…</tg-thinking>"
	if strings.TrimSpace(text) == "" {
		return thinking
	}
	return text + "\n\n" + thinking
}
