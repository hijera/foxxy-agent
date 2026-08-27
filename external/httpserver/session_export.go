//go:build http

package httpserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
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
	rendered, ok := s.renderSessionExport(w, r, id)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", rendered.contentType)
	w.Header().Set("Content-Disposition", exportContentDisposition(rendered.title, id, rendered.ext))
	w.Header().Set("Cache-Control", "private, max-age=0")
	_, _ = w.Write(rendered.body)
}

// exportRendered is a rendered transcript plus the metadata both delivery
// routes need: the browser download names a file from it, and the editor route
// writes it to disk under the same name.
type exportRendered struct {
	body        []byte
	contentType string
	ext         string
	title       string
}

// renderSessionExport performs the work both export routes share: validate the
// requested format, load the session, apply the assistant-answer guard, and
// render the document. It writes the HTTP error itself and reports ok=false
// when the caller must stop.
func (s *Server) renderSessionExport(w http.ResponseWriter, r *http.Request, id string) (exportRendered, bool) {
	format := exportFormat(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))))
	if !isValidExportFormat(format) {
		http.Error(w, `{"error":{"message":"unsupported format; use one of: json, html, pdf, docx"}}`, http.StatusBadRequest)
		return exportRendered{}, false
	}

	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return exportRendered{}, false
	}

	// Build the dialogue payload from the persisted transcript.
	title := strings.TrimSpace(st.GetTitlePinned())
	if title == "" {
		title = strings.TrimSpace(st.GetTitleAuto())
	}
	msgs := st.GetMessages()

	// There is nothing worth downloading before the assistant has answered, and
	// the panel hides the action until then. Enforcing the same rule here keeps a
	// direct request from producing a document that holds only the question.
	if !hasExportableAssistantAnswer(msgs) {
		http.Error(w, `{"error":{"message":"session has no exportable messages"}}`, http.StatusNotFound)
		return exportRendered{}, false
	}
	doc := buildExportDocument(id, title, msgs)
	// The readable formats embed pictures the session stored on disk; a session
	// that was never persisted simply has none to embed.
	if sd := strings.TrimSpace(st.GetPersistedSessionDir()); sd != "" {
		doc.assetsDir = session.AssetsPath(sd)
	}

	body, contentType, ext, err := renderExport(doc, format)
	if err != nil {
		http.Error(w, `{"error":{"message":"export rendering failed"}}`, http.StatusInternalServerError)
		return exportRendered{}, false
	}
	return exportRendered{body: body, contentType: contentType, ext: ext, title: doc.Title}, true
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
	// Everything but JSON is meant to be read, so it drops the editor's ambient
	// context; JSON stays verbatim for re-import. See readableExportDocument.
	if format != exportJSON {
		doc = readableExportDocument(doc)
	}
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

// exportBaseName sanitizes the session title into a safe file name stem and
// falls back to the session id. The result still carries the original
// characters — narrowing to ASCII happens only for the compatibility fallback
// in exportContentDisposition.
func exportBaseName(title, id string) string {
	base := strings.TrimSpace(title)
	if base == "" {
		base = id
	}
	// Strip path separators and control chars, collapse to a readable base.
	base = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return -1
		}
		if r == ' ' {
			return '_'
		}
		return r
	}, base)
	base = strings.Trim(base, "._")
	if base == "" {
		base = id
	}
	return base
}

// exportContentDisposition builds the attachment header. RFC 6266 wants a plain
// ASCII `filename` every client understands plus an RFC 8187 `filename*` that
// carries the real title; percent-encoding the plain parameter instead (as an
// earlier version did) makes a Cyrillic title arrive as literal %D0%9E… in every
// client that does not decode it.
func exportContentDisposition(title, id, ext string) string {
	base := exportBaseName(title, id)
	ascii := exportASCIIName(base, id) + "." + ext
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", ascii, rfc5987Encode(base+"."+ext))
}

// exportASCIIName degrades a file stem to the ASCII subset the plain `filename`
// parameter can carry. A stem that keeps no letters or digits carries no
// information, so it falls back to the session id — folded the same way, because
// the id arrives from the request path and must not reach the header unchecked.
func exportASCIIName(base, id string) string {
	if out := asciiFoldFilename(base); out != "" {
		return out
	}
	if out := asciiFoldFilename(id); out != "" {
		return out
	}
	return "session"
}

// asciiFoldFilename collapses each run of characters unusable in a plain
// `filename` into one underscore, and returns "" when nothing informative is
// left. Header separators are folded too: inside a quoted string they stay
// legal, but a lenient client parsing the header by hand would read them as the
// start of another parameter.
func asciiFoldFilename(s string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		if r == '_' || r > 127 || !isSafeFilenameASCII(r) {
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
			continue
		}
		b.WriteRune(r)
		lastUnderscore = false
	}
	out := strings.Trim(b.String(), "._")
	hasAlnum := strings.ContainsFunc(out, func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	})
	if !hasAlnum {
		return ""
	}
	return out
}

// isSafeFilenameASCII reports whether a rune may appear verbatim in the plain
// `filename` parameter.
func isSafeFilenameASCII(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("-_.()[]+", r)
}

// rfc5987Encode percent-encodes a file name for the `filename*` parameter.
// Everything outside RFC 8187's attr-char set is escaped, so header separators
// such as ';' inside a session title cannot split the header into extra
// parameters.
func rfc5987Encode(s string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0f])
	}
	return b.String()
}

// hasExportableAssistantAnswer reports whether the transcript contains at least
// one assistant turn with non-empty content. The handler refuses to export
// anything else and the UI mirrors the same guard to hide the action, so the
// rule lives in one place.
func hasExportableAssistantAnswer(msgs []llm.Message) bool {
	for _, m := range msgs {
		if m.Role == llm.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return true
		}
	}
	return false
}
