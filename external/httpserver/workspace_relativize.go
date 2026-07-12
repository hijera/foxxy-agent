//go:build http

package httpserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type relativizeRequest struct {
	Paths []string `json:"paths"`
	URIs  []string `json:"uris"`
}

type relativizeItem struct {
	PathRel string `json:"path_rel"`
	OK      bool   `json:"ok"`
}

// foxxycodeWorkspaceRelativizePost converts absolute filesystem paths (or file:// URIs)
// into workspace-relative POSIX paths under the session cwd. Paths outside the workspace
// (or that fail to resolve) are returned with ok:false. It backs the IDE drag-and-drop
// flow: a file dropped on the composer arrives as an absolute path and is turned into an
// @-mention relative to the session cwd.
func (s *Server) foxxycodeWorkspaceRelativizePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req relativizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON body"}}`, http.StatusBadRequest)
		return
	}

	cwdAbs, ok := s.resolveSlashListCWD(w, r)
	if !ok {
		return
	}

	inputs := make([]string, 0, len(req.Paths)+len(req.URIs))
	inputs = append(inputs, req.Paths...)
	for _, u := range req.URIs {
		if p := fileURIToPath(u); p != "" {
			inputs = append(inputs, p)
		}
	}

	items := make([]relativizeItem, 0, len(inputs))
	for _, in := range inputs {
		items = append(items, relativizeUnderCWD(cwdAbs, in))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "foxxycode.workspace_relativize",
		"items":  items,
	})
}

// relativizeUnderCWD resolves input to a POSIX path relative to cwdAbs, rejecting paths
// that escape the workspace (or that are the workspace root itself).
func relativizeUnderCWD(cwdAbs, input string) relativizeItem {
	in := strings.TrimSpace(input)
	if in == "" {
		return relativizeItem{OK: false}
	}
	abs, err := filepath.Abs(in)
	if err != nil {
		return relativizeItem{OK: false}
	}
	rel, err := filepath.Rel(cwdAbs, abs)
	if err != nil {
		return relativizeItem{OK: false}
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return relativizeItem{OK: false}
	}
	return relativizeItem{PathRel: rel, OK: true}
}

// fileURIToPath converts a file:// / vscode-file:// URI to a filesystem path, or "".
func fileURIToPath(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "file://") && !strings.HasPrefix(lower, "vscode-file://") {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Path == "" {
		return ""
	}
	p := u.Path
	// Windows drive URIs decode to "/C:/x" — drop the leading slash.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}
