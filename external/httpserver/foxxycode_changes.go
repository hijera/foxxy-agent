//go:build http

package httpserver

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/gitws"
	"github.com/hijera/foxxycode-agent/internal/linediff"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/textenc"
	"github.com/hijera/foxxycode-agent/internal/workcopy"
)

// changedFileDTO is one file of the session's working copy changes. Patch,
// Before and After are pointers so an empty string still serialises: a binary
// file legitimately has an empty patch, and the caller must be able to tell
// that apart from a field it did not ask for.
type changedFileDTO struct {
	Path      string  `json:"path"`
	Status    string  `json:"status"`
	Additions int     `json:"additions"`
	Deletions int     `json:"deletions"`
	Binary    bool    `json:"binary"`
	Revision  string  `json:"revision"`
	Truncated bool    `json:"truncated"`
	Patch     *string `json:"patch,omitempty"`
	Before    *string `json:"before,omitempty"`
	After     *string `json:"after,omitempty"`
}

// registerChangesRoutes adds the Edits routes for Git and SVN working copies.
func (s *Server) registerChangesRoutes() {
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/changes", s.foxxycodeSessionChangesList)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/changes/file", s.foxxycodeSessionChangeFile)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/changes/revert", s.foxxycodeSessionChangesRevert)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/changes/open-in-ide", s.foxxycodeSessionChangeOpenInIDE)
}

// decodeSide turns file bytes into text. textenc owns the encoding decision for
// the whole tree, so a Windows-1251 source reads the same here as it does
// through the file tools. Undecodable content is reported as binary.
func decodeSide(data []byte) (text string, binary bool) {
	if data == nil {
		return "", false
	}
	if textenc.LooksBinary(data) {
		return "", true
	}
	decoded, _, err := textenc.DecodeToUTF8(data)
	if err != nil {
		return "", true
	}
	return decoded, false
}

// buildChangedFile renders one working copy change into its wire form.
func buildChangedFile(change workcopy.Change, withPatch, withContent bool) changedFileDTO {
	before, beforeBinary := decodeSide(change.Before)
	after, afterBinary := decodeSide(change.After)
	binary := change.Binary || beforeBinary || afterBinary

	dto := changedFileDTO{Path: filepath.ToSlash(change.Path), Status: change.Status, Binary: binary, Revision: changeRevision(change)}
	if !binary {
		dto.Additions, dto.Deletions = linediff.Stat(before, after)
	}
	if withPatch {
		patch := ""
		if !binary {
			var truncated bool
			patch, truncated = linediff.Unified(change.Path, before, after, linediff.DefaultContext)
			dto.Truncated = truncated
		}
		dto.Patch = &patch
	}
	if withContent {
		dto.Before = &before
		dto.After = &after
	}
	return dto
}

// sessionWorkspace resolves the folder a session works in, writing the HTTP
// error itself when the session is unknown or has none.
func (s *Server) sessionWorkspace(w http.ResponseWriter, r *http.Request, id string) (*session.State, string) {
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return nil, ""
	}
	cwd := strings.TrimSpace(st.GetCWD())
	if cwd == "" {
		writeSubagentsError(w, http.StatusBadRequest, "session has no workspace")
		return nil, ""
	}
	return st, cwd
}

// parseChangeIncludes reads the comma-separated `include` query parameter.
func parseChangeIncludes(raw string) (withPatch, withContent bool) {
	for _, part := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "patch":
			withPatch = true
		case "content":
			withContent = true
		}
	}
	return withPatch, withContent
}

// foxxycodeSessionChangesList handles GET /foxxycode/sessions/{id}/changes.
//
// Response:
//
//	{ "object": "foxxycode.session_changes", "sessionId": "...", "vcs": "git",
//	  "files": [ { "path": "...", "status": "modified", "additions": 2, "deletions": 1,
//	               "binary": false, "truncated": false } ],
//	  "totals": { "files": 1, "additions": 2, "deletions": 1 },
//	  "skipped": 0 }
//
// The set comes from Git HEAD or the working copy's SVN BASE revision.
// `vcs` is empty for a plain folder, and the set is then empty. `skipped` counts new
// files left out (past the cap, or too large to read). `?include=patch,content`
// adds the unified patch and the decoded file sides.
func (s *Server) foxxycodeSessionChangesList(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	_, cwd := s.sessionWorkspace(w, r, id)
	if cwd == "" {
		return
	}
	wc := s.changesFor(r.Context(), cwd)
	work, skipped, err := wc.list(r.Context())
	if err != nil {
		s.log.Error("read working copy changes", "session", id, "error", err)
		writeSubagentsError(w, http.StatusInternalServerError, "read failed")
		return
	}

	withPatch, withContent := parseChangeIncludes(r.URL.Query().Get("include"))
	files := make([]changedFileDTO, 0, len(work))
	totalAdd, totalDel := 0, 0
	for _, change := range work {
		dto := buildChangedFile(change, withPatch, withContent)
		totalAdd += dto.Additions
		totalDel += dto.Deletions
		files = append(files, dto)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.session_changes",
		"sessionId": id,
		"vcs":       wc.vcs,
		"files":     files,
		"totals": map[string]int{
			"files":     len(files),
			"additions": totalAdd,
			"deletions": totalDel,
		},
		"skipped": skipped,
	})
}

// foxxycodeSessionChangeFile handles GET /foxxycode/sessions/{id}/changes/file?path=...
//
// The path must be in the version control report. Only the requested file is read.
func (s *Server) foxxycodeSessionChangeFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	path := r.URL.Query().Get("path")
	if strings.TrimSpace(path) == "" {
		writeSubagentsError(w, http.StatusBadRequest, "path is required")
		return
	}
	_, cwd := s.sessionWorkspace(w, r, id)
	if cwd == "" {
		return
	}
	change, err := s.changesFor(r.Context(), cwd).read(r.Context(), path)
	if err != nil {
		s.log.Error("read working copy change", "session", id, "error", err)
		writeSubagentsError(w, http.StatusInternalServerError, "read failed")
		return
	}
	if change == nil {
		writeSubagentsError(w, http.StatusNotFound, "file is not among the working copy changes")
		return
	}
	dto := buildChangedFile(*change, true, true)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.session_change",
		"sessionId": id,
		"path":      dto.Path,
		"status":    dto.Status,
		"additions": dto.Additions,
		"deletions": dto.Deletions,
		"binary":    dto.Binary,
		"truncated": dto.Truncated,
		"revision":  dto.Revision,
		"patch":     dto.Patch,
		"before":    dto.Before,
		"after":     dto.After,
	})
}

// changeRevision binds a decision to the exact raw contents on both sides.
func changeRevision(change workcopy.Change) string {
	h := sha256.New()
	for _, field := range [][]byte{[]byte(change.Path), []byte(change.Status), change.Before, change.After} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(field)))
		_, _ = h.Write(size[:])
		_, _ = h.Write(field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Server) foxxycodeSessionChangeOpenInIDE(w http.ResponseWriter, r *http.Request) {
	if !isDirectLoopbackClient(r) {
		writeSubagentsError(w, http.StatusForbidden, "native diffs require a local editor")
		return
	}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		writeSubagentsError(w, http.StatusUnsupportedMediaType, "expected application/json")
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || dec.Decode(&struct{}{}) != io.EOF || strings.TrimSpace(req.Path) == "" {
		writeSubagentsError(w, http.StatusBadRequest, "path is required")
		return
	}
	id := r.PathValue("id")
	_, cwd := s.sessionWorkspace(w, r, id)
	if cwd == "" {
		return
	}
	change, err := s.changesFor(r.Context(), cwd).read(r.Context(), req.Path)
	if err != nil {
		s.log.Error("read native diff", "session", id, "error", err)
		writeSubagentsError(w, http.StatusInternalServerError, "read failed")
		return
	}
	if change == nil {
		writeSubagentsError(w, http.StatusNotFound, "file is not among the working copy changes")
		return
	}
	if len(change.Before)+len(change.After) > 4<<20 {
		writeSubagentsError(w, http.StatusUnprocessableEntity, "native diff exceeds the size limit")
		return
	}
	before, beforeBinary := decodeSide(change.Before)
	after, afterBinary := decodeSide(change.After)
	if change.Binary || beforeBinary || afterBinary {
		writeSubagentsError(w, http.StatusUnprocessableEntity, "native diff requires a text file")
		return
	}
	if !ideEvents.HasSubscribers() {
		writeSubagentsError(w, http.StatusConflict, "no local editor is connected")
		return
	}
	ideEvents.Broadcast(ideEvent{
		Type: "open_diff", SessionID: id, ToolCallID: changeRevision(*change),
		Path: filepath.Join(cwd, change.Path), Before: before, After: after,
	})
	w.WriteHeader(http.StatusNoContent)
}

// discardRequest is the body of a discard: the files to put back, or
// all of them. It never defaults to everything - an empty body, both keys, an
// empty list or an unknown key is refused - because the call deletes the new
// files it covers.
type discardRequest struct {
	// Raw, so a "paths" that is there but null or empty is told apart from
	// one that is not there at all.
	Paths    json.RawMessage `json:"paths"`
	All      bool            `json:"all"`
	Revision string          `json:"revision,omitempty"`
}

// foxxycodeSessionChangesRevert handles POST /foxxycode/sessions/{id}/changes/revert.
//
// The version control backend restores selected files to HEAD or BASE and
// deletes selected new files. Ignored files stay in place.
func (s *Server) foxxycodeSessionChangesRevert(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	// Exactly one JSON object: whatever follows it would make a destructive
	// request mean something other than what its first half says.
	var req discardRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		writeSubagentsError(w, http.StatusBadRequest, `body must be {"paths":[...]} or {"all":true}`)
		return
	}
	var paths []string
	if len(req.Paths) > 0 {
		if err := json.Unmarshal(req.Paths, &paths); err != nil || len(paths) == 0 {
			writeSubagentsError(w, http.StatusBadRequest, `"paths" must name at least one file`)
			return
		}
	}
	if req.All == (len(req.Paths) > 0) {
		writeSubagentsError(w, http.StatusBadRequest, `name the files in "paths" or set "all", not both and not neither`)
		return
	}
	if req.Revision != "" && len(paths) != 1 {
		writeSubagentsError(w, http.StatusBadRequest, "revision requires exactly one path")
		return
	}
	st, cwd := s.sessionWorkspace(w, r, id)
	if cwd == "" {
		return
	}
	// Rewriting files under an agent that is mid-edit would leave the folder in
	// a state neither side expects - this chat's agent, or that of any other
	// chat working in the same folder, or in one inside or around it.
	if s.sessionTurnActive(id) {
		writeSubagentsError(w, http.StatusConflict, "session "+id+" has a turn in flight")
		return
	}
	if busy := s.turnInFolder(id, cwd); busy != "" {
		writeSubagentsError(w, http.StatusConflict, "session "+busy+" has a turn in flight in this folder")
		return
	}
	unlock, err := s.mgr.AcquireComposerTurnLock(id, st)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, session.ErrSessionTurnBusy) {
			status = http.StatusConflict
		}
		writeSubagentsError(w, status, err.Error())
		return
	}
	defer unlock()

	wc := s.changesFor(r.Context(), cwd)
	if req.Revision != "" {
		current, err := wc.read(r.Context(), paths[0])
		if err != nil {
			s.log.Error("read change before rejection", "session", id, "error", err)
			writeSubagentsError(w, http.StatusInternalServerError, "read failed")
			return
		}
		if current == nil || changeRevision(*current) != req.Revision {
			writeSubagentsError(w, http.StatusConflict, "the file changed since review")
			return
		}
		paths[0] = current.Path
	}

	if err := wc.discard(r.Context(), paths); err != nil {
		switch {
		case errors.Is(err, gitws.ErrNotRepository):
			writeSubagentsError(w, http.StatusBadRequest, "the session's folder is not a Git or SVN working copy")
		case errors.Is(err, workcopy.ErrNotChanged):
			// The client asked for a file its list showed and version control no longer
			// reports: committed, or put back, since it last read.
			writeSubagentsError(w, http.StatusConflict, err.Error())
		default:
			s.log.Error("discard working copy changes", "session", id, "error", err)
			// A failure halfway may have put some files back already, so
			// every window reads the folder again.
			s.publishSessionChanges(id)
			writeSubagentsError(w, http.StatusInternalServerError, "discard failed")
		}
		return
	}
	// Another window showing this session reads the new state as well.
	s.publishSessionChanges(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.session_changes_reverted",
		"sessionId": id,
		"at":        time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// turnInFolder names another session of this process with a turn running in
// cwd, in a folder inside it or in one around it, or returns "". A discard
// rewrites the folder, so their agents would find their files changed under
// them.
func (s *Server) turnInFolder(self, cwd string) string {
	for _, other := range s.mgr.ActiveTurnSessionIDs() {
		if other == self {
			continue
		}
		st := s.mgr.SessionByID(other)
		if st == nil {
			continue
		}
		if foldersOverlap(cwd, st.GetCWD()) {
			return other
		}
	}
	return ""
}

// foldersOverlap reports whether one folder is the other or lies inside it,
// compared in their canonical form (links resolved, case folded where the
// filesystem does).
func foldersOverlap(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	if session.SameWorkspacePath(a, b) {
		return true
	}
	ca, cb := session.CanonicalWorkspacePath(a), session.CanonicalWorkspacePath(b)
	inside := func(root, p string) bool {
		rel, err := filepath.Rel(root, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
	}
	return inside(ca, cb) || inside(cb, ca)
}
