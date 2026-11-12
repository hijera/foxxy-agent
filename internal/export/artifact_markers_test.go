package export

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestExportStripsAssistantPlacementWithoutChangingUserTextOrHistory(t *testing.T) {
	marker := `<foxxycode_file id="abc"/>`
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Explain " + marker},
		{Role: llm.RoleAssistant, Content: "    Report ready.\n\n" + marker},
	}
	doc := BuildExportDocument(ExportInput{SessionID: "s", Messages: messages})
	if len(doc.Entries) != 2 || doc.Entries[0].Text != messages[0].Content {
		t.Fatalf("user-authored text changed: %+v", doc.Entries)
	}
	if doc.Entries[1].Text != "    Report ready.\n\n" {
		t.Fatalf("answer projection = %q", doc.Entries[1].Text)
	}
	if !strings.Contains(messages[1].Content, marker) {
		t.Fatal("export changed its source transcript")
	}
}
