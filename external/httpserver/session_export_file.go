//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// An editor webview cannot save a blob the way a browser tab can:
//
//   - IntelliJ hosts the SPA in JCEF, which silently drops any download no
//     CefDownloadHandler claims.
//   - The VS Code panel hosts the SPA in a cross-origin iframe whose webview
//     grants no download permission, and the extension host cannot script into
//     that frame to intercept one.
//
// So inside an editor the panel asks the server to materialise the document
// instead, and a connected plugin reveals it in the OS file manager. The path
// is computed here and never accepted from the caller, mirroring the plan
// "Show in IDE" route, so this cannot be used to reveal an arbitrary file.

// exportTempSubdir is the directory under the OS temp dir that holds rendered
// transcripts. Each session gets its own child so two chats with the same title
// cannot overwrite one another, while re-exporting one chat in one format keeps
// replacing the same file instead of piling up copies.
const exportTempSubdir = "foxxycode"

// exportFileNameMaxRunes caps the stem taken from a session title so a long
// title cannot push the path past a filesystem limit.
const exportFileNameMaxRunes = 80

// foxxycodeSessionExportFilePost renders the transcript exactly like the
// download route, writes it under the OS temp directory, and asks any connected
// editor plugin to reveal it. Responds with the absolute path so the panel can
// show it even when no plugin is listening.
func (s *Server) foxxycodeSessionExportFilePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	rendered, ok := s.renderSessionExport(w, r, id)
	if !ok {
		return
	}

	path, err := writeExportTempFile(id, rendered)
	if err != nil {
		http.Error(w, `{"error":{"message":"could not write the export file"}}`, http.StatusInternalServerError)
		return
	}

	delivered := ideEvents.hasSubscribers()
	ideEvents.broadcast(ideEvent{Type: "reveal_file", SessionID: id, Path: path})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"path":      path,
		"delivered": delivered,
	})
}

// writeExportTempFile stores a rendered document at
// <temp>/foxxycode/exports/<sessionId>/<title>.<ext> and returns its absolute
// path. The file is rewritten on every export of the same chat and format.
func writeExportTempFile(sessionID string, rendered exportRendered) (string, error) {
	dir := filepath.Join(os.TempDir(), exportTempSubdir, "exports", exportBaseName(sessionID, "session"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := truncateRunes(exportBaseName(rendered.title, sessionID), exportFileNameMaxRunes) + "." + rendered.ext
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, rendered.body, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// truncateRunes shortens s to at most n runes, never splitting one in half.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return strings.TrimRight(string(runes[:n]), "._")
}
