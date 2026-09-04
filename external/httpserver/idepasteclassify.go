//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/idecopy"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
)

const (
	// pasteClassifyMaxBytes bounds the classified paste; anything larger is
	// plain text by definition (idecopy drops oversize candidates too).
	pasteClassifyMaxBytes = 64 * 1024
	// pasteClassifyMinSingleLineChars gates single-line pastes: a short line
	// ("true", "x := 1") matches editor selections far too easily to chip.
	pasteClassifyMinSingleLineChars = 16
)

type pasteClassifyResponse struct {
	Kind         string `json:"kind"`
	PathRel      string `json:"pathRel,omitempty"`
	StartLine    int    `json:"startLine,omitempty"`
	EndLine      int    `json:"endLine,omitempty"`
	TerminalName string `json:"terminalName,omitempty"`
}

// foxxycodeIdePasteClassifyPost decides whether text pasted into the chat
// composer is a fragment recently copied in the IDE (POST
// /foxxycode/ide/paste-classify). Match precedence: exact match against the
// copy-buffer ring, then against the current editor selection, then substring
// match against terminal buffers. File paths outside the workspace return
// kind "none" — the composer then performs a plain text paste.
func (s *Server) foxxycodeIdePasteClassifyPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	cwdAbs, ok := s.resolveSlashListCWD(w, r)
	if !ok {
		return
	}
	writeJSON := func(res pasteClassifyResponse) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}

	text := body.Text
	norm := idecopy.Normalize(text)
	if norm == "" || len(text) > pasteClassifyMaxBytes ||
		(!strings.Contains(norm, "\n") && len(norm) < pasteClassifyMinSingleLineChars) {
		writeJSON(pasteClassifyResponse{Kind: "none"})
		return
	}

	if c, found := idecopy.MatchFile(text); found {
		if item := relativizeUnderCWD(cwdAbs, c.PathAbs); item.OK {
			writeJSON(pasteClassifyResponse{Kind: "file", PathRel: item.PathRel, StartLine: c.StartLine, EndLine: c.EndLine})
			return
		}
	}
	if sel := ideenv.Get().Selection; sel != nil && idecopy.Normalize(sel.Text) == norm {
		if item := relativizeUnderCWD(cwdAbs, sel.File); item.OK {
			writeJSON(pasteClassifyResponse{Kind: "file", PathRel: item.PathRel, StartLine: sel.StartLine, EndLine: sel.EndLine})
			return
		}
	}
	if c, found := idecopy.MatchTerminal(text); found {
		writeJSON(pasteClassifyResponse{Kind: "terminal", TerminalName: addressableTerminalName(c.TerminalName)})
		return
	}
	if term, found := terminalBufferContaining(norm); found {
		name := addressableTerminalName(term.Name)
		if name != "" || term.Active {
			// A terminal whose name cannot appear in "@terminal:<name>" is only
			// reachable through the bare "@terminal" token, which resolves to
			// the active terminal — so an inactive unaddressable match is none.
			writeJSON(pasteClassifyResponse{Kind: "terminal", TerminalName: name})
			return
		}
	}
	writeJSON(pasteClassifyResponse{Kind: "none"})
}

// terminalBufferContaining reports the first terminal (active first) whose
// buffered output contains the normalized text.
func terminalBufferContaining(norm string) (ideterm.Terminal, bool) {
	snap := ideterm.Get()
	ordered := make([]ideterm.Terminal, 0, len(snap.Terminals))
	for _, t := range snap.Terminals {
		if t.Active {
			ordered = append(ordered, t)
		}
	}
	for _, t := range snap.Terminals {
		if !t.Active {
			ordered = append(ordered, t)
		}
	}
	for _, t := range ordered {
		if strings.Contains(idecopy.Normalize(t.Output), norm) {
			return t, true
		}
	}
	return ideterm.Terminal{}, false
}

// addressableTerminalName returns name when it can appear in an
// "@terminal:<name>" mention (whitespace-free), else "" so the composer falls
// back to the bare "@terminal" token.
func addressableTerminalName(name string) string {
	if name == "" || strings.ContainsAny(name, " \t") {
		return ""
	}
	return name
}
