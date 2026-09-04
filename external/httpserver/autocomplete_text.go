//go:build http

package httpserver

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The pure text rules of inline completion: what a suggestion may look like, where it must
// stop, and when it is not worth showing. Everything here is deterministic and unit-tested; the
// handler in autocomplete.go only wires it to the model.

// autocompleteMaxLines caps a block suggestion regardless of the model's token budget. A block
// longer than this is never what the user wanted mid-typing, and rendering it buries the code
// below the caret.
const autocompleteMaxLines = 12

// autocompleteMaxStops is the most stop sequences sent with one request. OpenAI's own API
// accepts four; open-model servers accept more, but four is enough for the line stops plus the
// FIM family's end tokens that matter.
const autocompleteMaxStops = 4

// caretLineRaw returns the (possibly partial) line the caret sits on, indentation included.
func caretLineRaw(prefix string) string {
	if i := strings.LastIndexByte(prefix, '\n'); i >= 0 {
		return prefix[i+1:]
	}
	return prefix
}

// caretLine is caretLineRaw without its leading indentation - the fragment a model is most
// likely to repeat back before its actual continuation.
func caretLine(prefix string) string {
	return strings.TrimLeft(caretLineRaw(prefix), " \t")
}

// caretIndent is the indentation of the caret's line.
func caretIndent(prefix string) string {
	return indentOf(caretLineRaw(prefix))
}

func indentOf(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// firstNonBlankLine returns the first line of s with content, indentation kept, trailing
// whitespace dropped.
func firstNonBlankLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimRight(line, " \t\r")
		}
	}
	return ""
}

// decideMultiLine says whether the caret position invites a block rather than the rest of a
// line. Copilot's rule, in short: never grow past the caret line when there is code to the right
// of the caret; offer a block only where the line just opened one, or the line is empty.
func decideMultiLine(prefix, suffix string) bool {
	rest := suffix
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[:i]
	}
	if strings.TrimSpace(rest) != "" {
		return false
	}
	line := strings.TrimSpace(caretLineRaw(prefix))
	if line == "" {
		return true
	}
	for _, opener := range []string{"{", ":", "(", "[", "=>", "->", "=", ","} {
		if strings.HasSuffix(line, opener) {
			return true
		}
	}
	// Keywords that open a block without punctuation ("} else", "do", "then"): judged by the
	// last word, since the line usually starts with the closing of the previous block.
	words := strings.Fields(line)
	switch words[len(words)-1] {
	case "else", "then", "do", "try", "finally", "begin":
		return true
	}
	return false
}

// lineStops are the stop sequences that end a suggestion at its natural boundary. A single-line
// suggestion ends at the line break. A block ends when the model reaches the line the suffix
// already holds, or after a run of blank lines, which is a model starting the next function.
//
// The suffix line is sent with its exact indentation so an inner "}" does not trip it - but a
// line that is nothing but a closing bracket is not used at all: chat models often do not indent
// the block they write, and a "\n}" stop would then end the suggestion on the block's own
// closing brace, leaving "if a > b {" with no body. Such lines are handled by the suffix-overlap
// trim after the fact instead.
func lineStops(multiLine bool, suffix string) []string {
	if !multiLine {
		return []string{"\n"}
	}
	stops := []string{}
	if next := firstNonBlankLine(suffix); next != "" && !isClosingOnly(next) {
		stops = append(stops, "\n"+next)
	}
	return append(stops, "\n\n\n")
}

// capStops keeps the first n stop sequences, dropping duplicates and empties.
func capStops(stops []string, n int) []string {
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for _, s := range stops {
		if s == "" || seen[s] || len(out) >= n {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// startsMidIdentifier reports that the caret sits inside a word: the next character continues
// an identifier. A suggestion there would have to guess the rest of a name the user is
// already typing, and is almost always wrong.
func startsMidIdentifier(suffix string) bool {
	r, size := utf8.DecodeRuneInString(suffix)
	if size == 0 {
		return false
	}
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// stripFence removes a markdown code fence a chat model wrapped its answer in. Leading blank
// space before the fence carries no meaning; indentation inside it does and is kept.
func stripFence(out string) string {
	lead := strings.TrimLeft(out, " \t\n")
	if !strings.HasPrefix(lead, "```") {
		return out
	}
	return enhanceFenceRe.ReplaceAllString(lead, "")
}

// trimSuffixOverlap drops the tail of a suggestion that re-types what already follows the
// caret, so accepting it never produces "))" or a duplicated closing brace. The overlap is
// matched against the suffix both as written and with its leading whitespace removed. A
// one-character overlap counts only for closing punctuation: trimming a lone letter would
// mutilate suggestions whose last character merely coincides with the next line's first.
func trimSuffixOverlap(out, suffix string) string {
	if out == "" || suffix == "" {
		return out
	}
	for _, cand := range []string{suffix, strings.TrimLeft(suffix, " \t\n")} {
		if cand == "" {
			continue
		}
		longest := len(out)
		if len(cand) < longest {
			longest = len(cand)
		}
		for k := longest; k >= 1; k-- {
			if out[len(out)-k:] != cand[:k] {
				continue
			}
			if k == 1 && !strings.ContainsRune(")]};,", rune(out[len(out)-1])) {
				break
			}
			return out[:len(out)-k]
		}
	}
	return out
}

// truncateBlock cuts a multi-line suggestion where the block it started ends: at the line
// count cap, or at the first later line indented less than the caret's line, which is code
// belonging to the enclosing scope (the next function, the closing of an outer block).
func truncateBlock(lines []string, indent string) []string {
	if len(lines) > autocompleteMaxLines {
		lines = lines[:autocompleteMaxLines]
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if ind := indentOf(lines[i]); len(ind) < len(indent) && strings.HasPrefix(indent, ind) {
			return lines[:i]
		}
	}
	return lines
}

// isClosingOnly reports a suggestion made of nothing but closing punctuation - the model
// finishing a statement the suffix already finishes.
func isClosingOnly(s string) bool {
	seen := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if !strings.ContainsRune(")]};,", r) {
			return false
		}
		seen = true
	}
	return seen
}

// cleanCompletion turns a model reply into text that can be inserted verbatim at the caret.
//
// Models wrap the answer in a markdown fence, re-type the line the caret sits on, re-type the
// indentation of an empty line, and carry on past where existing code takes over, far more
// often than any instruction prevents. All of that is handled here once rather than in every
// editor plugin. The result is empty when nothing worth showing survives.
func cleanCompletion(raw, prefix, suffix string, multiLine bool) string {
	out := strings.ReplaceAll(raw, "\r\n", "\n")
	out = stripFence(out)

	lineRaw := caretLineRaw(prefix)
	lineTrim := strings.TrimLeft(lineRaw, " \t")
	if strings.TrimSpace(lineRaw) == "" {
		// An empty (or indentation-only) caret line: leading line breaks would insert blank
		// lines, and the model often re-types the indentation the user already has.
		out = strings.TrimLeft(out, "\n")
		if lineRaw != "" && strings.HasPrefix(out, lineRaw) {
			out = out[len(lineRaw):]
		}
	} else {
		// A single leading line break is meaningful ("start on the next line"); more are not.
		if trimmed := strings.TrimLeft(out, "\n"); len(trimmed) < len(out) {
			out = "\n" + trimmed
		}
		// A re-echo of the caret's own line: with prefix "\tif err " a reply of "if err != nil {"
		// must insert only "!= nil {". Two characters at least, so a one-letter prefix that
		// happens to start the suggestion is not mistaken for an echo.
		if len(lineTrim) >= 2 && strings.HasPrefix(strings.TrimLeft(out, " \t"), lineTrim) {
			out = strings.TrimLeft(out, " \t")[len(lineTrim):]
		}
		// The prefix already ends in whitespace: a leading space in the reply would double it.
		if strings.HasSuffix(prefix, " ") || strings.HasSuffix(prefix, "\t") {
			out = strings.TrimLeft(out, " \t")
		}
	}

	out = trimSuffixOverlap(out, suffix)
	out = strings.TrimRight(out, " \t\n")
	if out == "" {
		return ""
	}

	lines := strings.Split(out, "\n")
	if !multiLine {
		lines = lines[:1]
	} else {
		lines = truncateBlock(lines, caretIndent(prefix))
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	out = strings.TrimRight(strings.Join(lines, "\n"), " \t")

	if strings.TrimSpace(out) == "" || isClosingOnly(out) {
		return ""
	}
	// The model repeated the line right below the caret instead of adding one.
	if next := strings.TrimSpace(firstNonBlankLine(suffix)); next != "" && strings.TrimSpace(out) == next {
		return ""
	}
	return out
}

// cutoffReached decides, on a partially streamed reply, whether generation can stop now. It is
// the streaming counterpart of cleanCompletion's truncation: a stop sequence the model ignores,
// a block that has run past its scope, or a suggestion longer than will ever be shown are all
// cut here instead of waiting for the token budget to run out.
func cutoffReached(text string, multiLine bool, indent string) bool {
	text = stripFence(text)
	if !multiLine {
		i := strings.IndexByte(text, '\n')
		return i >= 0 && strings.TrimSpace(text[:i]) != ""
	}
	lines := strings.Split(text, "\n")
	complete := lines[:len(lines)-1] // the last element is a line still being written
	if len(complete) > autocompleteMaxLines {
		return true
	}
	return len(truncateBlock(complete, indent)) < len(complete)
}

// commentLeader is the line-comment marker for a language id, used to hand neighbouring files
// to models whose FIM convention has no file separator token.
func commentLeader(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "python", "ruby", "shell", "bash", "sh", "zsh", "shellscript", "yaml", "toml", "perl", "r",
		"makefile", "dockerfile", "elixir", "powershell", "nim", "crystal", "julia", "ini", "properties":
		return "#"
	case "sql", "lua", "haskell", "elm", "ada", "vhdl", "plsql":
		return "--"
	case "lisp", "clojure", "scheme", "racket":
		return ";;"
	case "erlang", "prolog":
		return "%"
	case "vb", "vbnet", "visualbasic":
		return "'"
	}
	return "//"
}

// tailBytes keeps at most max bytes from the end of s, snapped forward to a line boundary when one
// is close by, so the model never receives a half-truncated first line.
func tailBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := s[len(s)-max:]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[1:]
	}
	if i := strings.IndexByte(cut, '\n'); i >= 0 && i < len(cut)/4 {
		cut = cut[i+1:]
	}
	return cut
}

// headBytes keeps at most max bytes from the start of s, snapped back to a line boundary when one
// is close by, so the model never receives a half-truncated last line.
func headBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	if i := strings.LastIndexByte(cut, '\n'); i >= 0 && i > len(cut)*3/4 {
		cut = cut[:i]
	}
	return cut
}
