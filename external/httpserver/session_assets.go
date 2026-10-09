//go:build http

package httpserver

import (
	"io"
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

	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}

	// fork(session-asset-files): keep the fork's regular-file API while refusing
	// links. Root confinement and the identity check cover replacement races.
	root, err := os.OpenRoot(assetsDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		http.NotFound(w, r)
		return
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		http.NotFound(w, r)
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(head[:n]))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, name, opened.ModTime(), f)
}
