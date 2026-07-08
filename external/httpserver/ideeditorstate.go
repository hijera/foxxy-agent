//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/ideenv"
)

// foxxycodeIdeEditorState ingests editor state pushed by IDE extensions
// (POST /foxxycode/ide/editor-state) and stores the latest snapshot so it can
// be injected into subsequent agent turns. The IDE reports the currently open
// tabs and the focused file; there is one foxxycode process per workspace, so
// the snapshot is process-global.
func (s *Server) foxxycodeIdeEditorState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		OpenFiles  []string `json:"openFiles"`
		ActiveFile string   `json:"activeFile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	ideenv.Set(body.OpenFiles, body.ActiveFile)
	w.WriteHeader(http.StatusNoContent)
}
