package export

// The agent appends an IDE block (active file, open tabs) and a terminal
// summary to each user message, so they ride along in the transcript even
// though nobody typed them. A document meant to be read drops them; the
// machine-readable JSON keeps them, because a re-import wants what the model
// actually saw.

import (
	"strings"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// stripAmbientContext returns a copy of doc with the injected environment
// blocks removed from every user entry, and drops entries left with nothing.
func stripAmbientContext(doc ExportDocument) ExportDocument {
	out := doc
	out.Entries = make([]ExportEntry, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		if e.Type == ExportEntryUser {
			e.Text = strings.TrimSpace(session.StripContextBlocks(
				e.Text,
				session.TagIDEContext,
				session.TagTerminalContext,
				session.TagSessionAssets,
			))
			if e.Text == "" && len(e.Attachments) == 0 {
				continue // the turn held nothing but ambient context
			}
		}
		out.Entries = append(out.Entries, e)
	}
	return out
}
