//go:build http && miniapps

package httpserver

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/external/miniapps"
)

func (s *Server) miniAppsUploadPost(w http.ResponseWriter, r *http.Request) {
	state := s.miniAppsHTTPState()
	var app miniapps.MiniApp
	var err error
	if version := r.URL.Query().Get("version"); version != "" {
		app, err = state.store.GetRelease(r.PathValue("id"), version)
	} else {
		app, err = state.store.GetDraft(r.PathValue("id"))
	}
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	if revision := r.URL.Query().Get("revision"); revision == "" || revision != app.Revision {
		writeMiniAppsError(w, http.StatusConflict, "revision_conflict", "select the file for the current draft revision or released version")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, miniapps.MaxUploadBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		writeMiniAppsError(w, 400, "invalid_upload", "expected multipart/form-data with one file part")
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		writeMiniAppsError(w, 400, "invalid_upload", "a file is required")
		return
	}
	defer func() { _ = part.Close() }()
	if part.FormName() != "file" || part.FileName() == "" {
		writeMiniAppsError(w, 400, "invalid_upload", "expected a file part named file")
		return
	}
	upload, err := state.store.PutUpload(app, r.PathValue("input_id"), part.FileName(), part)
	if err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) || errors.Is(err, miniapps.ErrUploadTooLarge) {
			writeMiniAppsError(w, 413, "upload_too_large", "file exceeds upload limit")
		} else {
			s.writeMiniAppsServiceError(w, err)
		}
		return
	}
	if _, err := reader.NextPart(); !errors.Is(err, io.EOF) {
		_ = state.store.DeleteUpload(app.ID, upload.ID)
		writeMiniAppsError(w, 400, "invalid_upload", "upload exactly one file per request")
		return
	}
	writeMiniAppsJSON(w, 201, upload)
}

func (s *Server) miniAppsUploadDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.miniAppsHTTPState().store.DeleteUpload(r.PathValue("id"), r.PathValue("upload_id")); err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) miniAppsRunFileGet(w http.ResponseWriter, r *http.Request) {
	file, item, err := s.miniAppsHTTPState().store.OpenRunFile(r.PathValue("run_id"), r.PathValue("file_id"))
	if err != nil {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "result file is unavailable")
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		writeMiniAppsError(w, 404, "not_found", "result file is unavailable")
		return
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && (strings.HasPrefix(item.MediaType, "audio/") || strings.HasPrefix(item.MediaType, "video/") || strings.HasPrefix(item.MediaType, "image/") && item.MediaType != "image/svg+xml") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", item.MediaType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": item.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", item.SHA256))
	http.ServeContent(w, r, item.Name, info.ModTime(), file)
}
