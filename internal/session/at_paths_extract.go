package session

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// markdownFenceBeforeIndex reports whether caretExclusive sits inside fenced code per draftSlash parity.
func markdownFenceBeforeIndex(text string, caretExclusive int) bool {
	if caretExclusive < 0 {
		caretExclusive = 0
	}
	if caretExclusive > len(text) {
		caretExclusive = len(text)
	}
	head := text[:caretExclusive]
	inFence := false
	for _, line := range strings.Split(head, "\n") {
		trimmedLead := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmedLead, "```") {
			inFence = !inFence
		}
	}
	return inFence
}

func blockQuoteLineStrict(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(t, ">")
}

func isFilePathRune(r rune) bool {
	switch r {
	case '.', '/', '_', '-', '\\':
		return true
	default:
		return unicode.IsLetter(r) || unicode.IsNumber(r)
	}
}

// continuationLooksLikeMorePath skips ASCII space inside paths like "readme here.md".
func continuationLooksLikeMorePath(afterSpaces string) bool {
	trim := strings.TrimLeft(afterSpaces, " \t")
	if trim == "" {
		return false
	}
	r0, sz := utf8.DecodeRuneInString(trim)
	if r0 == utf8.RuneError || sz == 0 {
		return false
	}
	if !unicode.IsLetter(r0) && !unicode.IsNumber(r0) && r0 != '_' {
		return false
	}
	// Consume same class as [\p{L}\p{N}_][\p{L}\p{N}_.\-]* (first segment after space).
	var end int
	for end = 0; end < len(trim); {
		r, sz2 := utf8.DecodeRuneInString(trim[end:])
		if r == utf8.RuneError {
			break
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-' || r == '.' {
			end += sz2
			continue
		}
		break
	}
	word := trim[:end]
	return strings.ContainsAny(word, "/.")
}

// ExtractAtFilePathsFromText returns workspace-relative paths from plain @mentions.
// Mirrors external/ui draftAt.extractAtFileAttachments (file tokens only).
func ExtractAtFilePathsFromText(text string) []string {
	var out []string
	seen := make(map[string]struct{})
	n := len(text)

	for i := 0; i < n; {
		j := strings.Index(text[i:], "@")
		if j < 0 {
			break
		}
		j += i

		fenceIdx := j + 1
		if fenceIdx > len(text) {
			fenceIdx = len(text)
		}
		if markdownFenceBeforeIndex(text, fenceIdx) {
			i = j + 1
			continue
		}

		lineStart := strings.LastIndex(text[:j], "\n") + 1
		lineEnd := strings.Index(text[j:], "\n")
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += j
		}
		line := text[lineStart:lineEnd]
		if blockQuoteLineStrict(line) {
			i = j + 1
			continue
		}
		if j > 0 {
			prev, _ := utf8.DecodeLastRuneInString(text[:j])
			if !unicode.IsSpace(prev) && prev != utf8.RuneError {
				i = j + 1
				continue
			}
		}

		k := j + 1
		for k < n {
			r, size := utf8.DecodeRuneInString(text[k:])
			if r == utf8.RuneError || size == 0 {
				break
			}
			if r == '\r' || r == '\n' {
				break
			}
			if r == '@' {
				break
			}
			if isFilePathRune(r) {
				k += size
				continue
			}
			if unicode.IsSpace(r) {
				tail := text[k+size:]
				if continuationLooksLikeMorePath(tail) {
					k += size
					continue
				}
				break
			}
			break
		}

		raw := strings.TrimRight(text[j+1:k], " \t")
		i = k
		if raw == "" || strings.Contains(raw, "..") {
			continue
		}
		if strings.HasSuffix(filepath.ToSlash(raw), "/") {
			continue
		}
		key := filepath.ToSlash(raw)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, raw)
	}
	return out
}
