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

func TestDialogueToolPicturesOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options ExportOptions
		want    int
	}{
		{"command no tools", ExportOptions{NoTools: true}, 0},
		{"panel dialogue", ExportOptions{NoTools: true, KeepToolPictures: true, NoThinking: true}, 2},
		{"full transcript", ExportOptions{KeepToolPictures: true}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			doc := BuildExportDocument(ExportInput{SessionDir: dir, Options: tc.options, Messages: []llm.Message{
				{Role: llm.RoleAssistant, Reasoning: "private reasoning", ToolCalls: []llm.ToolCall{{ID: "read_1", Name: "read", InputJSON: `{"path":"private argument"}`}}},
				{Role: llm.RoleTool, ToolCallID: "read_1", Content: "private result", CreatedAt: "first", ImageParts: []llm.ImagePart{{}, {Name: "first.png", FilePath: filepath.Join("old", "assets", "first.png")}}},
				{Role: llm.RoleTool, ToolCallID: "missing_call", Content: "orphan result", CreatedAt: "second", ImageParts: []llm.ImagePart{{Name: "second.png", FilePath: "second.png"}}},
				{Role: llm.RoleTool, Content: "no images", ImageParts: []llm.ImagePart{{}}},
			}})
			var names []string
			for _, row := range dialogueFromDocument(doc, "").Messages {
				for _, a := range row.Attachments {
					if a.Name != "" {
						names = append(names, a.Name)
					}
				}
			}
			if len(names) != tc.want {
				t.Fatalf("pictures = %v, want %d", names, tc.want)
			}
			if tc.options.NoTools && tc.options.KeepToolPictures {
				if strings.Join(names, ",") != "first.png,second.png" || len(doc.Entries) != 2 {
					t.Fatalf("picture order or empty rows: %+v", doc.Entries)
				}
				if doc.Entries[0].CreatedAt != "first" || doc.Entries[0].Attachments[0].Path != filepath.Join(dir, "assets", "first.png") {
					t.Fatalf("picture lost timestamp or moved bundle path: %+v", doc.Entries[0])
				}
				data, err := RenderExport(doc, ExportFormatMarkdown)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "first.png") || !strings.Contains(string(data), "second.png") || strings.Contains(string(data), "private") || strings.Contains(string(data), "orphan result") {
					t.Fatalf("dialogue markdown lost pictures or leaked tool details: %s", data)
				}
			}
		})
	}
}
