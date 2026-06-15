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

// buildRichMarkdown assembles the final rich-message markdown from the accumulated
// LLM text and the list of tools that ran during the turn. The LLM text is passed
// through verbatim (it is already GitHub-flavored Markdown); the tools, when any,
// are appended as a collapsed-by-default <details> disclosure block.
func buildRichMarkdown(llmText string, tools []string) string {
	text := strings.TrimRight(llmText, " \t\n")
	details := richToolsDetails(tools)
	if details == "" {
		return strings.TrimSpace(text)
	}
	if strings.TrimSpace(text) == "" {
		return details
	}
	return text + "\n\n" + details
}

// richToolsDetails renders the executed tools as a collapsible <details> block.
// Returns "" when no tools ran. The block is collapsed by default (no open attribute)
// so it stays out of the way but remains available to the user.
func richToolsDetails(tools []string) string {
	if len(tools) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<details><summary>🛠 Tools used (%d)</summary>\n\n", len(tools))
	for _, name := range tools {
		fmt.Fprintf(&b, "- `%s`\n", name)
	}
	b.WriteString("\n</details>")
	return b.String()
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

// richFormattingHint is prepended to the first message of a new session when Rich
// Messages are enabled, so the agent leans into full Markdown instead of the
// restricted Telegram legacy subset used by telegramFormattingHint.
const richFormattingHint = `[System note – your replies are rendered as Telegram Rich Messages:
• Use the full GitHub-flavored Markdown you would use normally
• Headings (#, ##, ###), **bold**, _italic_, ~~strikethrough~~, ` + "`code`" + `
• Tables, ordered/unordered/task lists, > block quotes and fenced ` + "```lang" + ` code blocks all render natively
• Footnotes ([^1]) and LaTeX ($x^2$, $$E=mc^2$$) are supported
Do not avoid tables or headings — they look good here.
This note is invisible to the user.]

`
