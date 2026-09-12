//go:build gateway || gateway.telegram

package telegram

import (
	"regexp"
	"strconv"
	"strings"
)

// mdToTelegram converts a standard-markdown string to Telegram legacy-Markdown format.
//
// Telegram legacy Markdown supports: *bold*, _italic_, `inline code`, ```pre blocks```, [text](url).
// It does NOT support ## headers, **double-star bold**, tables, or horizontal rules.
//
// Conversion rules applied (code blocks are always preserved verbatim):
//   - Fenced code blocks (```...```) → kept byte for byte, fence line included
//   - ATX headers (# … ######) → *Header text*
//   - Double-star bold **text** / __text__ → *text* / _text_
//   - Bullet asterisk "* item" at line start → "• item"
//   - Markdown tables → best-effort plain text (pipes stripped, alignment rows removed)
//   - Horizontal rules (--- / === / ***) → a plain separator line
var (
	reHeader     = regexp.MustCompile(`(?m)^#{1,6} +(.+)$`)
	reDoubleStar = regexp.MustCompile(`\*\*(.+?)\*\*`)
	// The replacement below MUST brace the group: in Go's regexp syntax `_` is a
	// name character, so `_$1_` names the group "1_" — which does not exist, and
	// __text__ collapses to a bare "_". A stray underscore then also unbalances
	// the legacy-Markdown parse for the rest of the message.
	reDoubleUnder = regexp.MustCompile(`__(.+?)__`)
	reBulletStar  = regexp.MustCompile(`(?m)^\* `)
	reHRule       = regexp.MustCompile(`(?m)^(\*{3,}|-{3,}|={3,})$`)
	reTableAlign  = regexp.MustCompile(`(?m)^\|?[\s\-:|]+\|[\s\-:|]*\|?$`) // alignment row
)

// mdToTelegram converts text from standard Markdown to Telegram legacy-Markdown format.
// Returns the converted string; always safe to send with ParseMode="Markdown".
func mdToTelegram(text string) string {
	// Step 1: swap fenced code blocks for placeholders so the rules below cannot
	// reach their content. Every rule would otherwise fire inside a block: a
	// leading "#" becomes bold, "* " becomes a bullet, "---" becomes a separator,
	// and a line starting with "|" is reflowed as a table row — rewriting the very
	// source the user asked to see.
	blocks, stripped := extractCodeBlocks(text)

	// Step 2: apply conversions to the prose that is left.
	stripped = reHeader.ReplaceAllString(stripped, "*$1*")
	stripped = reDoubleStar.ReplaceAllString(stripped, "*$1*")
	stripped = reDoubleUnder.ReplaceAllString(stripped, "_${1}_")
	stripped = reBulletStar.ReplaceAllString(stripped, "• ")
	stripped = reHRule.ReplaceAllString(stripped, "────────────────")
	stripped = convertTables(stripped)

	// Step 3: put the untouched blocks back.
	return restoreCodeBlocks(stripped, blocks)
}

// blockPlaceholder is the token that stands in for one code block while the prose
// rules run. It is wrapped in NUL bytes, which prose from a model does not carry, and none
// of the conversion patterns match it: no leading "#", "* ", "|", "---", "**" or
// "__", so it reaches restoreCodeBlocks intact.
func blockPlaceholder(idx int) string {
	return "\x00BLOCK" + strconv.Itoa(idx) + "\x00"
}

// extractCodeBlocks replaces fenced code blocks with placeholder tokens and
// returns a map of placeholder → original block plus the text with the blocks
// removed. An unterminated block is captured too: a truncated reply is exactly
// when the raw text matters most.
func extractCodeBlocks(text string) (map[string]string, string) {
	blocks := map[string]string{}
	idx := 0
	lines := strings.Split(text, "\n")
	// Collected and joined rather than written with a trailing newline per line:
	// appending one would lengthen every message by a byte and drift the 4096-char
	// boundaries splitMessage cuts on.
	out := make([]string, 0, len(lines))
	inBlock := false
	var blockLines []string
	for _, line := range lines {
		if !inBlock && strings.HasPrefix(line, "```") {
			inBlock = true
			blockLines = []string{line}
			continue
		}
		if inBlock {
			blockLines = append(blockLines, line)
			if strings.TrimSpace(line) == "```" {
				key := blockPlaceholder(idx)
				blocks[key] = strings.Join(blockLines, "\n")
				out = append(out, key)
				blockLines = nil
				inBlock = false
				idx++
			}
			continue
		}
		out = append(out, line)
	}
	if len(blockLines) > 0 {
		key := blockPlaceholder(idx)
		blocks[key] = strings.Join(blockLines, "\n")
		out = append(out, key)
	}
	return blocks, strings.Join(out, "\n")
}

// restoreCodeBlocks puts the extracted blocks back where their placeholders sit.
// Placeholders are distinct, so the order of the map does not matter.
func restoreCodeBlocks(text string, blocks map[string]string) string {
	for key, block := range blocks {
		text = strings.ReplaceAll(text, key, block)
	}
	return text
}

// convertTables removes Markdown table alignment rows and strips pipe characters,
// turning table rows into plain comma-separated or space-aligned text.
func convertTables(text string) string {
	// Remove alignment rows like |---|:---:|
	text = reTableAlign.ReplaceAllString(text, "")
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "|") && strings.HasPrefix(trimmed, "|") {
			// Strip leading/trailing pipes and split into cells.
			inner := strings.Trim(trimmed, "|")
			cells := strings.Split(inner, "|")
			for i := range cells {
				cells[i] = strings.TrimSpace(cells[i])
			}
			out = append(out, strings.Join(cells, "  │  "))
		} else {
			out = append(out, line)
		}
	}
	// Remove consecutive blank lines left by alignment row removal.
	return collapseBlankLines(strings.Join(out, "\n"))
}

func collapseBlankLines(s string) string {
	re := regexp.MustCompile(`\n{3,}`)
	return re.ReplaceAllString(s, "\n\n")
}

// telegramFormattingHint is prepended to the first message of a new gateway session
// so the agent knows to use Telegram-compatible formatting.
const telegramFormattingHint = `[System note – format your replies for Telegram chat:
• Use *bold* (single asterisks, e.g. *word*) for emphasis
• Use _italic_ (underscores) for secondary emphasis
• Use ` + "`code`" + ` for inline code and ` + "```lang\n...\n```" + ` for blocks
• No markdown tables — use plain text, bullet lists, or ` + "`code`" + ` blocks instead
• No # headings — use *bold* text as a section title instead
• Use - or • for bullet lists, not * (asterisk bullets break formatting)
This note is invisible to the user.]

`
