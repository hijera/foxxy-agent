//go:build http

package httpserver

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// This file owns the exported document model and the JSON renderer. The three
// readable formats live beside it:
//
//   session_export_markdown.go  markdown -> the shared block/inline model
//   session_export_code.go      syntax highlighting for code blocks
//   session_export_media.go     local image resolution
//   session_export_html.go      HTML
//   session_export_pdf.go       PDF
//   session_export_docx.go      DOCX
//
// The dialogue surface is the user/assistant turns plus any assistant reasoning
// blocks; tool/system rows are skipped so the exported document reads as a
// conversation.

// exportFormat is one of the supported render targets.
type exportFormat string

const (
	exportJSON exportFormat = "json"
	exportHTML exportFormat = "html"
	exportPDF  exportFormat = "pdf"
	exportDOCX exportFormat = "docx"
)

// exportAttachment is one file the user uploaded on a turn, recovered from the
// <foxxycode_session_assets> wrapper the agent appends to the message.
type exportAttachment struct {
	Name string
	Path string
}

// exportMessage is one turn in the exported dialogue.
type exportMessage struct {
	Role        string             `json:"role"`
	Content     string             `json:"content"`
	Reasoning   string             `json:"reasoning,omitempty"`
	CreatedAt   string             `json:"created_at,omitempty"`
	Attachments []exportAttachment `json:"-"`
}

// exportDocument is the structured payload rendered into every format.
type exportDocument struct {
	SessionID  string          `json:"session_id"`
	Title      string          `json:"title"`
	ExportedAt string          `json:"exported_at"`
	Messages   []exportMessage `json:"messages"`

	// assetsDir is the session's on-disk assets directory, used to embed
	// referenced images. Unexported so it never reaches the JSON export; empty
	// for a session that was never persisted.
	assetsDir string
}

// media builds the image resolver for this document. Returns nil when the
// session has no assets directory, which every caller treats as "embed nothing".
func (d exportDocument) media() *exportMediaResolver {
	if d.assetsDir == "" {
		return nil
	}
	return newExportMediaResolver(d.assetsDir)
}

// buildExportDocument filters the persisted transcript down to the dialogue
// surface and tags it with metadata. Only user and assistant roles are kept;
// for assistant messages a non-empty reasoning block is exported as its own
// labeled turn so the reader can follow the model's thinking.
func buildExportDocument(sessionID, title string, msgs []llm.Message) exportDocument {
	out := exportDocument{
		SessionID:  sessionID,
		Title:      title,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Messages:   make([]exportMessage, 0, len(msgs)),
	}
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			out.Messages = append(out.Messages, exportMessage{
				Role:      "user",
				Content:   m.Content,
				CreatedAt: m.CreatedAt,
			})
		case llm.RoleAssistant:
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(m.Reasoning) == "" {
				continue
			}
			out.Messages = append(out.Messages, exportMessage{
				Role:      "assistant",
				Content:   m.Content,
				Reasoning: strings.TrimSpace(m.Reasoning),
				CreatedAt: m.CreatedAt,
			})
		default:
			// system / tool rows are not part of the exported conversation.
		}
	}
	return out
}

// readableExportDocument returns a copy of doc prepared for a document meant to
// be read. The agent appends an IDE block (active file, open tabs) and a
// terminal summary to each user message, so they show up in the transcript even
// though nobody typed them — in a document meant to be read they are noise.
//
// The uploads wrapper records something the user actually did, so it is kept —
// but as structured attachments rather than as the raw XML block, which would
// otherwise be printed verbatim in the middle of the page.
//
// The JSON export deliberately does not go through here: it is the
// machine-readable one, and a re-import wants what the model actually saw.
func readableExportDocument(doc exportDocument) exportDocument {
	out := doc
	out.Messages = make([]exportMessage, 0, len(doc.Messages))
	for _, m := range doc.Messages {
		m.Attachments = parseExportAttachments(m.Content)
		m.Content = strings.TrimSpace(session.StripContextBlocks(
			m.Content,
			session.TagIDEContext,
			session.TagTerminalContext,
			session.TagSessionAssets,
		))
		if m.Content == "" && strings.TrimSpace(m.Reasoning) == "" && len(m.Attachments) == 0 {
			continue // the turn held nothing but ambient context
		}
		out.Messages = append(out.Messages, m)
	}
	return out
}

// parseExportAttachments recovers the uploaded files listed in a
// <foxxycode_session_assets> block. The producer (filePathsNote in
// internal/agent/react.go) writes one "- <abs path>" line per file, with an
// optional " (<display name>)" suffix when the upload's name differs from the
// file it was saved as.
func parseExportAttachments(content string) []exportAttachment {
	body, ok := extractTagBody(content, session.TagSessionAssets)
	if !ok {
		return nil
	}
	var out []exportAttachment
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if line == "" {
			continue
		}
		att := exportAttachment{Path: line}
		if open := strings.LastIndex(line, " ("); open >= 0 && strings.HasSuffix(line, ")") {
			att.Path = strings.TrimSpace(line[:open])
			att.Name = strings.TrimSpace(line[open+2 : len(line)-1])
		}
		if att.Path == "" {
			continue
		}
		if att.Name == "" {
			att.Name = filepath.Base(att.Path)
		}
		out = append(out, att)
	}
	return out
}

// extractTagBody returns the text between <tag> and </tag>, dropping the
// producer's leading prose on the opening line.
func extractTagBody(content, tag string) (string, bool) {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(content, open)
	if start < 0 {
		return "", false
	}
	rest := content[start+len(open):]
	end := strings.Index(rest, close)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// renderJSONExport writes the machine-readable export.
func renderJSONExport(doc exportDocument) ([]byte, error) {
	return json.MarshalIndent(doc, "", "  ")
}

// exportRoleLabel is the human-facing name of a turn.
func exportRoleLabel(role string) string {
	if role == "" {
		return ""
	}
	return strings.ToUpper(role[:1]) + role[1:]
}
