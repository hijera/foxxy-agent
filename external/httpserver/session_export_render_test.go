//go:build http

package httpserver

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// sampleExportDoc builds a document with a markdown-rich assistant turn so the
// renderers exercise headings, code blocks, emphasis, and a non-Latin code
// point (em dash) that would have crashed fpdf's core Latin-1 fonts.
func sampleExportDoc() exportDocument {
	return buildExportDocument("sess_test", "My Chat", []llm.Message{
		{Role: llm.RoleUser, Content: "Explain exports."},
		{Role: llm.RoleAssistant, Content: "# Heading\n\nSome **bold** and `code` — with an em dash.\n\n```go\nfmt.Println(\"hi\")\n```", Reasoning: "Thinking it over."},
		{Role: llm.RoleTool, Content: "tool output should be skipped"},
	})
}

// unicodeExportDoc carries Cyrillic + punctuation so PDF/DOCX renderers are
// exercised on the non-Latin code points that matter for the Russian UI.
func unicodeExportDoc() exportDocument {
	return buildExportDocument("sess_ru", "Привет, мир", []llm.Message{
		{Role: llm.RoleUser, Content: "Объясни, как работает экспорт."},
		{Role: llm.RoleAssistant, Content: "# Заголовок\n\n**Жирный** текст и `код` — со знаком тире.", Reasoning: "Размышляю над ответом."},
	})
}

func TestBuildExportDocumentSkipsToolRows(t *testing.T) {
	doc := sampleExportDoc()
	for _, m := range doc.Messages {
		if m.Role == "tool" {
			t.Fatalf("tool row leaked into export: %+v", m)
		}
	}
	if len(doc.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(doc.Messages))
	}
}

func TestRenderJSONExport(t *testing.T) {
	b, err := renderJSONExport(sampleExportDoc())
	if err != nil {
		t.Fatalf("renderJSONExport: %v", err)
	}
	if !bytes.Contains(b, []byte(`"role": "user"`)) || !bytes.Contains(b, []byte(`"role": "assistant"`)) {
		t.Fatalf("JSON missing role entries: %s", b)
	}
	if bytes.Contains(b, []byte("tool output")) {
		t.Fatalf("tool content leaked into JSON export")
	}
}

func TestRenderHTMLExport(t *testing.T) {
	b, err := renderHTMLExport(sampleExportDoc())
	if err != nil {
		t.Fatalf("renderHTMLExport: %v", err)
	}
	body := string(b)
	if !strings.HasPrefix(body, "<!DOCTYPE html>") {
		t.Fatalf("HTML missing doctype")
	}
	if !strings.Contains(body, "<h1>") {
		t.Fatalf("HTML missing rendered heading")
	}
	if !strings.Contains(body, "<code>") {
		t.Fatalf("HTML missing rendered code")
	}
}

func TestRenderPDFExport(t *testing.T) {
	b, err := renderPDFExport(sampleExportDoc())
	if err != nil {
		t.Fatalf("renderPDFExport: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("PDF missing header, got: %q", b[:min(8, len(b))])
	}
}

func TestRenderDOCXExport(t *testing.T) {
	b, err := renderDOCXExport(sampleExportDoc())
	if err != nil {
		t.Fatalf("renderDOCXExport: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("DOCX is not a valid zip: %v", err)
	}
	var body, styles string
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, required := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml", "word/numbering.xml"} {
		if !names[required] {
			t.Fatalf("DOCX missing required part %q", required)
		}
	}
	// The fenced code block must carry its content (a regression guard: an
	// earlier version read the wrong node and emitted an empty code block).
	if err := readDocxPart(zr, "word/document.xml", &body); err != nil {
		t.Fatalf("read document.xml: %v", err)
	}
	if !strings.Contains(body, `fmt.Println`) {
		t.Fatalf("DOCX document.xml missing code-block content (got empty code block)")
	}
	// Named paragraph styles referenced from the body must be defined.
	if err := readDocxPart(zr, "word/styles.xml", &styles); err != nil {
		t.Fatalf("read styles.xml: %v", err)
	}
	for _, styleID := range []string{`w:styleId="Title"`, `w:styleId="Code"`, `w:styleId="Quote"`, `w:styleId="ListParagraph"`} {
		if !strings.Contains(styles, styleID) {
			t.Fatalf("styles.xml missing style %q", styleID)
		}
	}
}

// readDocxPart reads a single file out of an opened DOCX zip into the pointed
// string. Used by render tests that need to assert on part contents.
func readDocxPart(zr *zip.Reader, name string, out *string) error {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		buf, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		*out = string(buf)
		return nil
	}
	return fmt.Errorf("part %q not found", name)
}

// TestRenderPDFUnicode ensures the PDF path no longer panics on non-Latin-1
// code points (Cyrillic, em dash) once the DejaVu Sans cuts are embedded.
func TestRenderPDFUnicode(t *testing.T) {
	b, err := renderPDFExport(unicodeExportDoc())
	if err != nil {
		t.Fatalf("renderPDFExport on Unicode content: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("PDF missing header")
	}
	if len(b) < 20000 {
		t.Fatalf("PDF suspiciously small (%d bytes); embedded font likely missing", len(b))
	}
}

// TestRenderDOCXUnicode ensures Cyrillic round-trips into the DOCX body and
// that the em dash survives the XML escaping.
func TestRenderDOCXUnicode(t *testing.T) {
	b, err := renderDOCXExport(unicodeExportDoc())
	if err != nil {
		t.Fatalf("renderDOCXExport on Unicode content: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("DOCX is not a valid zip: %v", err)
	}
	var body string
	if err := readDocxPart(zr, "word/document.xml", &body); err != nil {
		t.Fatalf("read document.xml: %v", err)
	}
	if !strings.Contains(body, "Жирный") {
		t.Fatalf("DOCX body missing Cyrillic content")
	}
	if !strings.Contains(body, "Привет, мир") {
		t.Fatalf("DOCX body missing the title")
	}
}

// TestMarkdownToBlocksCodeContent guards the goldmark Lines() fix: fenced and
// indented code blocks must surface their text, not an empty string.
func TestMarkdownToBlocksCodeContent(t *testing.T) {
	blocks := markdownToBlocks("```go\nfmt.Println(\"x\")\n```\n")
	if len(blocks) != 1 || blocks[0].kind != "code_block" {
		t.Fatalf("expected one code block, got %+v", blocks)
	}
	if !strings.Contains(blocks[0].text, `fmt.Println("x")`) {
		t.Fatalf("code block lost its content: %q", blocks[0].text)
	}
}

func TestHasExportableAssistantAnswer(t *testing.T) {
	if hasExportableAssistantAnswer([]llm.Message{{Role: llm.RoleUser, Content: "hi"}}) {
		t.Fatal("should be false with no assistant message")
	}
	if !hasExportableAssistantAnswer([]llm.Message{{Role: llm.RoleAssistant, Content: "answer"}}) {
		t.Fatal("should be true with assistant content")
	}
	if hasExportableAssistantAnswer([]llm.Message{{Role: llm.RoleAssistant, Content: "   "}}) {
		t.Fatal("should be false with whitespace-only assistant content")
	}
}

func TestExportFileName(t *testing.T) {
	cases := map[string]string{
		"My Chat":          "My_Chat.json",
		"a/b:c":            "abc.json",
		"":                 "sess_x.json",
		"Привет":           "%D0%9F%D1%80%D0%B8%D0%B2%D0%B5%D1%82.json",
	}
	for in, want := range cases {
		if got := exportFileName(in, "sess_x", "json"); got != want {
			t.Errorf("exportFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsValidExportFormat(t *testing.T) {
	for _, ok := range []exportFormat{exportJSON, exportHTML, exportPDF, exportDOCX} {
		if !isValidExportFormat(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []exportFormat{"rtf", "", "csv"} {
		if isValidExportFormat(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
