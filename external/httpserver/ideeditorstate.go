//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/idecopy"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
)

// ideEditorSelection mirrors the optional selection object on editor-state.
type ideEditorSelection struct {
	File      string `json:"file"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Text      string `json:"text"`
}

// foxxycodeIdeEditorState ingests editor state pushed by IDE extensions
// (POST /foxxycode/ide/editor-state) and stores the latest snapshot so it can
// be injected into subsequent agent turns. The IDE reports the currently open
// tabs, the focused file, and optionally the current text selection; there is
// one foxxycode process per workspace, so the snapshot is process-global.
// A reported selection is also offered to the paste-to-chip copy ring: recent
// selections are the copy heuristic for IDEs without clipboard events (VS Code).
func (s *Server) foxxycodeIdeEditorState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		OpenFiles  []string            `json:"openFiles"`
		ActiveFile string              `json:"activeFile"`
		Selection  *ideEditorSelection `json:"selection"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	var sel *ideenv.Selection
	if body.Selection != nil {
		sel = &ideenv.Selection{
			File:      body.Selection.File,
			StartLine: body.Selection.StartLine,
			EndLine:   body.Selection.EndLine,
			Text:      body.Selection.Text,
		}
	}
	ideenv.Set(body.OpenFiles, body.ActiveFile, sel)
	if stored := ideenv.Get().Selection; stored != nil {
		idecopy.Offer(idecopy.Candidate{
			Kind:      idecopy.KindFile,
			PathAbs:   stored.File,
			StartLine: stored.StartLine,
			EndLine:   stored.EndLine,
			Text:      stored.Text,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}
