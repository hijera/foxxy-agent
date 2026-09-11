package export

// The bridge between the two halves of this package: the format-agnostic
// ExportDocument that /export and `foxxycode sessions export` build, and the
// dialogue model the PDF, DOCX and HTML renderers walk.
//
// Both surfaces render from ExportDocument, so a transcript reads the same
// whether it is downloaded from the panel or written into the workspace. They
// differ only in their defaults: the panel exports the conversation alone, the
// command keeps the tool calls (see ExportOptions).

import (
	"fmt"
	"strings"
)

const (
	// exportRoleHeader labels the metadata block the document opens with, and
	// exportRoleTool a tool call folded into the dialogue. The renderers treat
	// both like any other turn; only the label differs.
	exportRoleHeader = "header"
	exportRoleTool   = "tool"
)

// dialogueFromDocument projects an ExportDocument onto the dialogue model the
// document renderers walk. Entries the options already dropped are not in doc,
// so this only reshapes what survived.
func dialogueFromDocument(doc ExportDocument, assetsDir string) exportDocument {
	out := exportDocument{
		SessionID:  doc.Session.ID,
		Title:      doc.Session.Title,
		ExportedAt: doc.Session.ExportedAt,
		assetsDir:  assetsDir,
	}
	if head := documentHeaderMarkdown(doc); head != "" {
		out.Messages = append(out.Messages, exportMessage{
			Role:    exportRoleHeader,
			Content: head,
		})
	}
	for _, e := range doc.Entries {
		switch e.Type {
		case ExportEntryUser:
			msg := exportMessage{Role: "user", Content: e.Text, CreatedAt: e.CreatedAt}
			for _, a := range e.Attachments {
				msg.Attachments = append(msg.Attachments, exportAttachment{Name: a.Name, Path: a.Path})
			}
			out.Messages = append(out.Messages, msg)
		case ExportEntryAssistant:
			if strings.TrimSpace(e.Text) != "" || strings.TrimSpace(e.Reasoning) != "" {
				out.Messages = append(out.Messages, exportMessage{
					Role:      "assistant",
					Content:   e.Text,
					Reasoning: e.Reasoning,
					CreatedAt: e.CreatedAt,
				})
			}
			for _, tc := range e.ToolCalls {
				if body := toolCallMarkdown(tc); body != "" {
					out.Messages = append(out.Messages, exportMessage{
						Role:      exportRoleTool,
						Content:   body,
						CreatedAt: e.CreatedAt,
					})
				}
			}
		case ExportEntryCompactionSummary, ExportEntrySystem:
			if strings.TrimSpace(e.Text) != "" {
				out.Messages = append(out.Messages, exportMessage{
					Role:      e.Type,
					Content:   e.Text,
					CreatedAt: e.CreatedAt,
				})
			}
		}
	}
	return out
}

// documentHeaderMarkdown renders the session metadata as a short list. Empty
// fields are left out rather than shown blank.
func documentHeaderMarkdown(doc ExportDocument) string {
	s := doc.Session
	rows := [][2]string{
		{"Workspace", s.CWD},
		{"Git branch", s.GitBranch},
		{"Model", s.Model},
		{"Started", s.StartedAt},
	}
	var b strings.Builder
	for _, row := range rows {
		if strings.TrimSpace(row[1]) == "" {
			continue
		}
		fmt.Fprintf(&b, "- **%s:** %s\n", row[0], row[1])
	}
	if s.MessageCount > 0 {
		fmt.Fprintf(&b, "- **Messages:** %d (%d user turns)\n", s.MessageCount, s.UserTurns)
	}
	if u := s.TokenUsage; u != nil && u.TotalTokens > 0 {
		fmt.Fprintf(&b, "- **Tokens:** %d in, %d out, %d total\n",
			u.InputTokens, u.OutputTokens, u.TotalTokens)
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolCallMarkdown renders one tool call the way the readable formats show it:
// the tool name in bold, then its arguments and result as code blocks.
func toolCallMarkdown(tc ExportToolCall) string {
	name := strings.TrimSpace(tc.Name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**\n", name)
	if args := strings.TrimSpace(string(tc.Input)); args != "" && args != "null" {
		fmt.Fprintf(&b, "\n```json\n%s\n```\n", args)
	}
	if tc.Result != nil {
		if res := strings.TrimSpace(*tc.Result); res != "" {
			fmt.Fprintf(&b, "\n```\n%s\n```\n", res)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
