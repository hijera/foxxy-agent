//go:build http

package httpserver

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// foxxycodeSessionExportGet renders a session transcript into one of the
// supported document formats (json, html, pdf, docx) and returns it as a
// downloadable attachment. The dialogue surface is the user/assistant turns
// plus any assistant reasoning blocks; tool and system rows are skipped so the
// exported document reads as a conversation.
//
// The export contract is documented in external/httpserver/openapi.go (and the
// served /docs/) and in docs/http-api.md — keep those in sync when the query
// parameter set or the response media types change.
func (s *Server) foxxycodeSessionExportGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	format := exportFormat(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))))
	if !isValidExportFormat(format) {
		http.Error(w, `{"error":{"message":"unsupported format; use one of: json, html, pdf, docx"}}`, http.StatusBadRequest)
		return
	}

	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}

	// Build the dialogue payload from the persisted transcript.
	title := strings.TrimSpace(st.GetTitlePinned())
	if title == "" {
		title = strings.TrimSpace(st.GetTitleAuto())
	}
	doc := buildExportDocument(id, title, st.GetMessages())

	// A title with no real assistant content still exports, but the UI hides the
	// action in that case; the server only guards against an entirely empty doc
	// to avoid shipping a blank file.
	if len(doc.Messages) == 0 {
		http.Error(w, `{"error":{"message":"session has no exportable messages"}}`, http.StatusNotFound)
		return
	}

	body, contentType, ext, err := renderExport(doc, format)
	if err != nil {
		http.Error(w, `{"error":{"message":"export rendering failed"}}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+exportFileName(doc.Title, id, ext)+"\"")
	w.Header().Set("Cache-Control", "private, max-age=0")
	_, _ = w.Write(body)
}

// isValidExportFormat reports whether the requested format is one we render.
func isValidExportFormat(f exportFormat) bool {
	switch f {
	case exportJSON, exportHTML, exportPDF, exportDOCX:
		return true
	}
	return false
}

// renderExport dispatches to the format renderer and returns the body plus the
// response content type and file extension.
func renderExport(doc exportDocument, format exportFormat) ([]byte, string, string, error) {
	switch format {
	case exportJSON:
		b, err := renderJSONExport(doc)
		return b, "application/json; charset=utf-8", "json", err
	case exportHTML:
		b, err := renderHTMLExport(doc)
		return b, "text/html; charset=utf-8", "html", err
	case exportPDF:
		b, err := renderPDFExport(doc)
		return b, "application/pdf", "pdf", err
	case exportDOCX:
		b, err := renderDOCXExport(doc)
		return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", err
	}
	return nil, "", "", nil
}

// exportFileName sanitizes the session title into a safe file name and falls
// back to the session id. Non-ASCII titles are percent-encoded for the
// filename* fallback so browsers preserve them.
func exportFileName(title, id, ext string) string {
	base := strings.TrimSpace(title)
	if base == "" {
		base = id
	}
	// Strip path separators and control chars, collapse to a readable base.
	base = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return -1
		}
		if r == ' ' {
			return '_'
		}
		return r
	}, base)
	if base == "" {
		base = id
	}
	return url.PathEscape(base) + "." + ext
}

// hasExportableAssistantAnswer reports whether the transcript contains at least
// one assistant turn with non-empty content. The UI mirrors this guard to show
// the export action only once an answer exists; the server keeps it as a shared
// helper so the rule lives in one place.
func hasExportableAssistantAnswer(msgs []llm.Message) bool {
	for _, m := range msgs {
		if m.Role == llm.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return true
		}
	}
	return false
}
