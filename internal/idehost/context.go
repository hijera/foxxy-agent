// Package idehost owns editor host context and event delivery, independently of the agent and HTTP.
package idehost

import (
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
)

// ideEnvMaxTabs caps how many open tabs are listed in the IDE context block to
// bound token usage on workspaces with many editors open.
const ideEnvMaxTabs = 50

// ideEnvMaxSelectionBytes caps the selection text included in the IDE context
// block. Kept short — a paste-to-chip attachment carries the full fragment on
// demand; this is ambient context only.
const ideEnvMaxSelectionBytes = 2 * 1024

// EditorNote builds an XML annotation describing the files the user currently
// has open in their IDE (the focused tab plus every open tab), mirroring the
// environment context other coding agents inject each turn. Paths are made
// relative to cwd when they live under it. Returns an empty string when no IDE
// has reported any editor state.
//
// The tag is stripped from the user-visible bubble by the SPA's
// stripFoxxyCodeAttachmentsForUserDisplay function.
func EditorNote(cwd string, snap ideenv.Snapshot) string {
	if snap.ActiveFile == "" && len(snap.OpenFiles) == 0 && snap.Selection == nil {
		return ""
	}
	rel := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" {
			return ""
		}
		if cwd != "" {
			if r, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(r, "..") {
				return filepath.ToSlash(r)
			}
		}
		return filepath.ToSlash(p)
	}
	var b strings.Builder
	b.WriteString("<foxxycode_ide_context>\n# Active File\n")
	if af := rel(snap.ActiveFile); af != "" {
		b.WriteString(af)
	} else {
		b.WriteString("(none)")
	}
	b.WriteString("\n\n# Open Tabs\n")
	if len(snap.OpenFiles) == 0 {
		b.WriteString("(none)")
	} else {
		n := len(snap.OpenFiles)
		if n > ideEnvMaxTabs {
			n = ideEnvMaxTabs
		}
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(rel(snap.OpenFiles[i]))
		}
	}
	if sel := snap.Selection; sel != nil {
		text := tailBytes(sel.Text, ideEnvMaxSelectionBytes)
		b.WriteString("\n\n# Selection\n")
		fmt.Fprintf(&b, "%s:%d-%d\n", rel(sel.File), sel.StartLine, sel.EndLine)
		b.WriteString(text)
	}
	b.WriteString("\n</foxxycode_ide_context>")
	return b.String()
}

// terminalEnvMaxTerminals caps how many terminals are summarized in the
// always-on terminal context block, bounding token usage.
const terminalEnvMaxTerminals = 8

// terminalEnvMaxContextBytes caps the per-terminal output tail included in the
// always-on context block. It is kept short on purpose — the @terminal mention
// (terminalMentionNote) pulls the fuller buffer on demand.
const terminalEnvMaxContextBytes = 2 * 1024

// TerminalNote builds an XML annotation summarizing the IDE terminals the
// user currently has open (each with a short tail of recent output), mirroring
// ideEnvNote. The active terminal is listed first. Returns "" when no IDE has
// reported any terminal state.
//
// The tag is stripped from the user-visible bubble by the SPA's
// stripFoxxyCodeAttachmentsForUserDisplay function.
func TerminalNote(snap ideterm.Snapshot) string {
	if len(snap.Terminals) == 0 {
		return ""
	}
	ordered := terminalsActiveFirst(snap.Terminals)
	n := len(ordered)
	if n > terminalEnvMaxTerminals {
		n = terminalEnvMaxTerminals
	}
	blocks := make([]string, 0, n)
	for i := 0; i < n; i++ {
		tm := ordered[i]
		var b strings.Builder
		if tm.Active {
			b.WriteString("# Active Terminal: ")
		} else {
			b.WriteString("# Terminal: ")
		}
		b.WriteString(tm.Name)
		if lc := strings.TrimSpace(tm.LastCommand); lc != "" {
			b.WriteString("\n$ ")
			b.WriteString(lc)
		}
		if out := strings.TrimRight(tailBytes(tm.Output, terminalEnvMaxContextBytes), "\n"); out != "" {
			b.WriteByte('\n')
			b.WriteString(out)
		}
		blocks = append(blocks, b.String())
	}
	return "<foxxycode_terminal_context>\n" + strings.Join(blocks, "\n\n") + "\n</foxxycode_terminal_context>"
}

// terminalMentionRe matches an @terminal mention: bare `@terminal` or
// `@terminal:<name>` (name runs to the next whitespace). A leading boundary
// avoids matching inside another token (e.g. an email-like `x@terminal`).
var terminalMentionRe = regexp.MustCompile(`(?:^|\s)@terminal(?::(\S+))?`)

// TerminalMentionNote expands @terminal / @terminal:<name> mentions found in the
// user text into a fuller <foxxycode_terminal_output> block carrying the
// complete captured buffer of the referenced terminal (the active terminal for
// a bare @terminal). Returns "" when there is no mention or no matching
// terminal. The tag is stripped from the user-visible bubble by the SPA.
func TerminalMentionNote(userText string, snap ideterm.Snapshot) string {
	matches := terminalMentionRe.FindAllStringSubmatch(userText, -1)
	if len(matches) == 0 {
		return ""
	}
	if len(snap.Terminals) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	blocks := make([]string, 0, len(matches))
	for _, m := range matches {
		tm := pickTerminal(snap.Terminals, strings.TrimSpace(m[1]))
		if tm == nil {
			continue
		}
		key := tm.ID + "\x00" + tm.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		var b strings.Builder
		b.WriteString(`<foxxycode_terminal_output name="`)
		b.WriteString(html.EscapeString(tm.Name))
		b.WriteString("\">\n")
		if out := strings.TrimRight(tm.Output, "\n"); out != "" {
			b.WriteString(out)
			b.WriteByte('\n')
		}
		b.WriteString("</foxxycode_terminal_output>")
		blocks = append(blocks, b.String())
	}
	if len(blocks) == 0 {
		return ""
	}
	return strings.Join(blocks, "\n\n")
}

// terminalsActiveFirst returns the terminals reordered so the active one(s)
// come first, preserving relative order otherwise.
func terminalsActiveFirst(ts []ideterm.Terminal) []ideterm.Terminal {
	out := make([]ideterm.Terminal, 0, len(ts))
	for _, t := range ts {
		if t.Active {
			out = append(out, t)
		}
	}
	for _, t := range ts {
		if !t.Active {
			out = append(out, t)
		}
	}
	return out
}

// pickTerminal returns the terminal matching name (case-insensitive), or the
// active terminal (falling back to the first) when name is empty. Returns nil
// when a named terminal is not found.
func pickTerminal(ts []ideterm.Terminal, name string) *ideterm.Terminal {
	if name == "" {
		for i := range ts {
			if ts[i].Active {
				return &ts[i]
			}
		}
		if len(ts) > 0 {
			return &ts[0]
		}
		return nil
	}
	for i := range ts {
		if strings.EqualFold(ts[i].Name, name) {
			return &ts[i]
		}
	}
	return nil
}

// tailBytes returns the last maxBytes bytes of s (trimmed to a rune boundary)
// when s exceeds the cap, otherwise s unchanged.
func tailBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	tail := s[len(s)-maxBytes:]
	for i := 0; i < len(tail) && i < 4; i++ {
		if tail[i]&0xC0 != 0x80 {
			return tail[i:]
		}
	}
	return tail
}

// ContextNotes snapshots the current host once and renders the three prompt annotations.
func ContextNotes(cwd, userText string) []string {
	editor, terminals := ideenv.Get(), ideterm.Get()
	return []string{EditorNote(cwd, editor), TerminalNote(terminals), TerminalMentionNote(userText, terminals)}
}
