//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/idecopy"
)

// foxxycodeIdeCopyBufferPost ingests a copied-fragment candidate pushed by an
// IDE extension with real clipboard events (POST /foxxycode/ide/copy-buffer,
// IntelliJ today). The chat composer later classifies pasted text against the
// ring via POST /foxxycode/ide/paste-classify.
func (s *Server) foxxycodeIdeCopyBufferPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Kind         string `json:"kind"`
		Path         string `json:"path"`
		StartLine    int    `json:"startLine"`
		EndLine      int    `json:"endLine"`
		TerminalName string `json:"terminalName"`
		Text         string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	if body.Kind != idecopy.KindFile && body.Kind != idecopy.KindTerminal {
		http.Error(w, `{"error":{"message":"kind must be \"file\" or \"terminal\""}}`, http.StatusBadRequest)
		return
	}
	idecopy.Offer(idecopy.Candidate{
		Kind:         body.Kind,
		PathAbs:      body.Path,
		StartLine:    body.StartLine,
		EndLine:      body.EndLine,
		TerminalName: body.TerminalName,
		Text:         body.Text,
	})
	w.WriteHeader(http.StatusNoContent)
}
