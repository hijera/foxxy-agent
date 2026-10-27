package export

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestExportSessionEmbedsToolPictures(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNGFixture(t, assets, "shot.png", 32, 16)
	in := ExportInput{CWD: dir, SessionDir: dir, Messages: []llm.Message{
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "read_1", Name: "read", InputJSON: `{"path":"shot.png"}`}}},
		{Role: llm.RoleTool, ToolCallID: "read_1", Content: "PNG image", ImageParts: []llm.ImagePart{{Name: "shot.png", FilePath: "shot.png"}}},
	}}
	for _, format := range []string{"html", "pdf", "docx"} {
		t.Run(format, func(t *testing.T) {
			result, err := ExportSession(in, ExportRequest{Target: "chat." + format})
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(result.Target.Path)
			if err != nil {
				t.Fatal(err)
			}
			switch format {
			case "html":
				if !bytes.Contains(data, []byte("data:image/png;base64,")) {
					t.Fatal("exported HTML lost the tool picture")
				}
			case "pdf":
				if !bytes.Contains(data, []byte("/Subtype /Image")) {
					t.Fatal("exported PDF lost the tool picture")
				}
			case "docx":
				z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range z.File {
					if strings.HasPrefix(f.Name, "word/media/") {
						return
					}
				}
				t.Fatal("exported DOCX lost the tool picture")
			}
		})
	}
}

func TestExportToolPictureFollowsMovedSessionBundle(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writePNGFixture(t, assets, "shot.png", 32, 16)
	doc := BuildExportDocument(ExportInput{
		SessionID: "sess_picture", SessionDir: dir,
		Messages: []llm.Message{
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "read_1", Name: "read", InputJSON: `{"path":"shot.png"}`}}},
			{Role: llm.RoleTool, ToolCallID: "read_1", Content: "PNG image", ImageParts: []llm.ImagePart{{Name: "shot.png", FilePath: filepath.Join("old-bundle", "assets", "shot.png")}}},
		},
	}).WithAssetsDir(assets)
	if got := doc.Entries[0].ToolCalls[0].Pictures[0].Path; got != path {
		t.Fatalf("picture path = %q, want %q", got, path)
	}
	for _, format := range []ExportFormat{ExportFormatHTML, ExportFormatJSON, ExportFormatMarkdown, ExportFormatPDF, ExportFormatDOCX} {
		data, err := RenderExport(doc, format)
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: bytes=%d error=%v", format, len(data), err)
		}
		if format == ExportFormatHTML && !strings.Contains(string(data), "data:image/png;base64,") {
			t.Fatal("HTML export did not embed the tool picture")
		}
	}
}
