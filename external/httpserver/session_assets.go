//go:build http

package httpserver

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// foxxycodeSessionAssetGet serves a single file from a session's assets directory
// (screenshots from the browser tool, pasted images, etc.). The name is a bare file
// name — path separators and traversal segments are rejected so a request can never
// escape the assets directory.
func (s *Server) foxxycodeSessionAssetGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || strings.ContainsAny(name, `/\`) || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		http.Error(w, `{"error":{"message":"invalid asset name"}}`, http.StatusBadRequest)
		return
	}

	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	sd := strings.TrimSpace(st.GetPersistedSessionDir())
	if sd == "" {
		http.Error(w, `{"error":{"message":"assets unavailable"}}`, http.StatusServiceUnavailable)
		return
	}

	assetsDir := session.AssetsPath(sd)
	full := filepath.Join(assetsDir, name)
	// Defence in depth: the resolved path must stay within the assets directory.
	if rel, err := filepath.Rel(assetsDir, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.Error(w, `{"error":{"message":"invalid asset name"}}`, http.StatusBadRequest)
		return
	}

	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, full)
}
