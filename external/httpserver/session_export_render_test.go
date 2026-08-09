//go:build http

package httpserver

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
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

// ambientDoc carries the wrappers the agent appends to a user turn: editor state
// nobody typed, plus the two blocks that do record a user action.
func ambientDoc() exportDocument {
	return buildExportDocument("sess_amb", "Ambient", []llm.Message{
		{Role: llm.RoleUser, Content: "Why does it fail?" +
			"\n\n<foxxycode_ide_context>\n# Active File\nmain.go\n\n# Open Tabs\nmain.go\nutil.go\n</foxxycode_ide_context>" +
			"\n\n<foxxycode_terminal_context>\n# Active Terminal: zsh\n$ go build\n</foxxycode_terminal_context>" +
			"\n\n<foxxycode_session_assets>\n- /tmp/shot.png (shot.png)\n</foxxycode_session_assets>" +
			"\n\n<foxxycode_terminal_output name=\"zsh\">$ go test\nFAIL\n</foxxycode_terminal_output>"},
		{Role: llm.RoleAssistant, Content: "Because of a typo.", Reasoning: "Reading the build output."},
	})
}

func TestReadableExportDocumentDropsAmbientContext(t *testing.T) {
	readable := readableExportDocument(ambientDoc())

	if len(readable.Messages) != 2 {
		t.Fatalf("expected both turns to survive, got %d", len(readable.Messages))
	}
	user := readable.Messages[0].Content
	for _, gone := range []string{"Active File", "Open Tabs", "Active Terminal", "main.go", "util.go"} {
		if strings.Contains(user, gone) {
			t.Errorf("readable export still shows %q", gone)
		}
	}
	// What the user typed, uploaded, or explicitly pulled in with @terminal stays.
	for _, kept := range []string{"Why does it fail?", "shot.png", "FAIL"} {
		if !strings.Contains(user, kept) {
			t.Errorf("readable export dropped %q", kept)
		}
	}
	if readable.Messages[1].Content != "Because of a typo." {
		t.Errorf("assistant turn was altered: %q", readable.Messages[1].Content)
	}
}

// The source document must not be modified: the JSON renderer reads it after the
// readable copy has been taken.
func TestReadableExportDocumentLeavesTheOriginalIntact(t *testing.T) {
	doc := ambientDoc()
	_ = readableExportDocument(doc)

	if !strings.Contains(doc.Messages[0].Content, "Active File") {
		t.Fatal("readableExportDocument mutated the document it was given")
	}
}

// A turn that was nothing but ambient context would render as an empty heading.
func TestReadableExportDocumentDropsTurnsLeftEmpty(t *testing.T) {
	doc := buildExportDocument("s", "T", []llm.Message{
		{Role: llm.RoleUser, Content: "<foxxycode_ide_context>\n# Active File\nx.go\n</foxxycode_ide_context>"},
		{Role: llm.RoleAssistant, Content: "Answer."},
	})

	readable := readableExportDocument(doc)

	if len(readable.Messages) != 1 || readable.Messages[0].Role != "assistant" {
		t.Fatalf("expected only the assistant turn, got %+v", readable.Messages)
	}
}

// The JSON export is the machine-readable one: a re-import wants what the model
// actually saw, so it keeps every wrapper.
func TestRenderExportKeepsAmbientContextInJSONOnly(t *testing.T) {
	doc := ambientDoc()

	jsonBody, _, _, err := renderExport(doc, exportJSON)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if !bytes.Contains(jsonBody, []byte("foxxycode_ide_context")) {
		t.Error("the JSON export dropped the injected context")
	}

	for _, format := range []exportFormat{exportHTML, exportPDF, exportDOCX} {
		body, _, _, err := renderExport(doc, format)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if format == exportPDF {
			continue // PDF text is glyph-encoded; the feature suite reads it back
		}
		if bytes.Contains(body, []byte("Active File")) {
			t.Errorf("the %s export still shows the ambient IDE context", format)
		}
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

func TestExportBaseName(t *testing.T) {
	cases := map[string]string{
		"My Chat": "My_Chat",
		"a/b:c":   "abc",
		"":        "sess_x",
		`x"y|z`:   "xyz",
		"Привет":  "Привет",
	}
	for in, want := range cases {
		if got := exportBaseName(in, "sess_x"); got != want {
			t.Errorf("exportBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestExportContentDisposition pins the RFC 6266 shape: an ASCII-only plain
// filename any client can read, plus an RFC 8187 filename* carrying the real
// (possibly non-Latin) title. A title made only of non-ASCII characters has no
// meaningful ASCII form, so the fallback degrades to the session id.
func TestExportContentDisposition(t *testing.T) {
	cases := []struct {
		title      string
		wantPlain  string
		wantEncSub string
	}{
		{"My Chat", `filename="My_Chat.pdf"`, "filename*=UTF-8''My_Chat.pdf"},
		{"", `filename="sess_x.pdf"`, "filename*=UTF-8''sess_x.pdf"},
		{"Отчёт по задаче", `filename="sess_x.pdf"`, "filename*=UTF-8''%D0%9E%D1%82%D1%87%D1%91%D1%82_%D0%BF%D0%BE_%D0%B7%D0%B0%D0%B4%D0%B0%D1%87%D0%B5.pdf"},
		{"Sprint 3 — обзор", `filename="Sprint_3.pdf"`, ""},
	}
	for _, tc := range cases {
		got := exportContentDisposition(tc.title, "sess_x", "pdf")
		if !strings.HasPrefix(got, "attachment; ") {
			t.Errorf("exportContentDisposition(%q) = %q, missing attachment prefix", tc.title, got)
		}
		if !strings.Contains(got, tc.wantPlain) {
			t.Errorf("exportContentDisposition(%q) = %q, want plain %s", tc.title, got, tc.wantPlain)
		}
		if tc.wantEncSub != "" && !strings.Contains(got, tc.wantEncSub) {
			t.Errorf("exportContentDisposition(%q) = %q, want %s", tc.title, got, tc.wantEncSub)
		}
	}
}

// TestExportContentDispositionCannotBreakTheHeader guards the encoding: a title
// carrying header separators must not introduce new parameters, and control
// characters must never reach the wire.
func TestExportContentDispositionCannotBreakTheHeader(t *testing.T) {
	got := exportContentDisposition("evil; name=oops\r\nX-Injected: 1", "sess_x", "json")
	if strings.Contains(got, "\r") || strings.Contains(got, "\n") {
		t.Fatalf("header value carries a line break: %q", got)
	}
	if strings.Count(got, ";") != 2 {
		t.Fatalf("header gained extra parameters: %q", got)
	}
	marker := "filename*=UTF-8''"
	encoded := got[strings.Index(got, marker)+len(marker):]
	if strings.ContainsAny(encoded, `;,="' `) {
		t.Fatalf("filename* carries raw header separators: %q", encoded)
	}
}

// TestExportContentDispositionFoldsTheIDFallback covers the path where the title
// has no ASCII form and the session id supplies the plain filename: the id comes
// off the request path, so it must be folded rather than trusted.
func TestExportContentDispositionFoldsTheIDFallback(t *testing.T) {
	got := exportContentDisposition("Отчёт", `sess"; evil=1`, "json")
	m := regexp.MustCompile(`filename="([^"]*)"`).FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("no plain filename in %q", got)
	}
	if strings.ContainsAny(m[1], `";=`) {
		t.Fatalf("session id reached the header unchecked: %q", m[1])
	}
	if strings.Count(got, ";") != 2 {
		t.Fatalf("header gained extra parameters: %q", got)
	}
}

// TestSanitizeXMLText covers the characters that make a DOCX unopenable: the
// C0 control range (ANSI escapes, NUL) has to go, while the three whitespace
// controls XML does allow must survive.
func TestSanitizeXMLText(t *testing.T) {
	got := sanitizeXMLText("log \x1b[31mred\x1b[0m\ttab\nline\r\n\x00 end")
	for _, bad := range []string{"\x1b", "\x00"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitizeXMLText kept %q: %q", bad, got)
		}
	}
	for _, keep := range []string{"\t", "\n", "\r", "log ", "red", "end"} {
		if !strings.Contains(got, keep) {
			t.Errorf("sanitizeXMLText dropped %q: %q", keep, got)
		}
	}
}

// TestDocxEscapeSanitizes makes sure the escaping helper every run goes through
// is the place the sanitising happens, so no renderer can bypass it.
func TestDocxEscapeSanitizes(t *testing.T) {
	if got := docxEscape("a\x1bb<c"); got != "ab&lt;c" {
		t.Fatalf("docxEscape = %q, want %q", got, "ab&lt;c")
	}
}

// TestDocxHeadingStylesAreDefined walks every heading level markdown can
// produce and asserts the style it names exists in the style sheet.
func TestDocxHeadingStylesAreDefined(t *testing.T) {
	for level := 1; level <= 6; level++ {
		md := strings.Repeat("#", level) + " Title"
		blocks := markdownToBlocks(md)
		if len(blocks) != 1 {
			t.Fatalf("level %d: expected one block, got %d", level, len(blocks))
		}
		x := docxBlockXML(blocks[0], false)
		m := regexp.MustCompile(`<w:pStyle w:val="([^"]+)"/>`).FindStringSubmatch(x)
		if m == nil {
			t.Fatalf("level %d: heading emitted no paragraph style: %s", level, x)
		}
		if !strings.Contains(stylesXML, `w:styleId="`+m[1]+`"`) {
			t.Errorf("level %d uses undefined style %q", level, m[1])
		}
	}
}

// TestMarkdownToBlocksNumbersOrderedItems checks list items carry their ordinal
// so the PDF can print real numbers instead of a dash.
func TestMarkdownToBlocksNumbersOrderedItems(t *testing.T) {
	blocks := markdownToBlocks("3. three\n4. four")
	if len(blocks) != 2 {
		t.Fatalf("expected 2 list items, got %d", len(blocks))
	}
	for i, want := range []int{3, 4} {
		if !blocks[i].ordered {
			t.Errorf("item %d not marked ordered", i)
		}
		if blocks[i].number != want {
			t.Errorf("item %d numbered %d, want %d", i, blocks[i].number, want)
		}
	}
	bullets := markdownToBlocks("- a\n- b")
	for i := range bullets {
		if bullets[i].ordered {
			t.Errorf("bullet %d marked ordered", i)
		}
	}
}

// TestDocxListsUseDocumentNumbering asserts list text carries no marker glyph of
// its own (Word draws it from numbering.xml) and that ordered and bullet items
// point at different numbering definitions.
func TestDocxListsUseDocumentNumbering(t *testing.T) {
	numIDs := map[string]bool{}
	for _, b := range markdownToBlocks("- bullet\n\n1. step") {
		x := docxBlockXML(b, false)
		if strings.Contains(x, "•") || strings.Contains(x, "–") {
			t.Errorf("list item repeats its marker in the text: %s", x)
		}
		m := regexp.MustCompile(`<w:numId w:val="([0-9]+)"/>`).FindStringSubmatch(x)
		if m == nil {
			t.Fatalf("list item carries no numbering reference: %s", x)
		}
		numIDs[m[1]] = true
	}
	if len(numIDs) != 2 {
		t.Fatalf("bullet and ordered items share a numbering id: %v", numIDs)
	}
	if !strings.Contains(numberingXML, `w:numFmt w:val="decimal"`) {
		t.Error("numbering.xml defines no decimal format for ordered lists")
	}
}

// TestWriteRunsPDFKeepsOneParagraphOnOneLine is the unit-level counterpart of
// the PDF feature scenario: a short sentence must occupy a single line no matter
// how many formatted runs it is built from.
func TestWriteRunsPDFKeepsOneParagraphOnOneLine(t *testing.T) {
	plainHeight := pdfBlockHeight(t, "Some bold and code in one sentence.")
	richHeight := pdfBlockHeight(t, "Some **bold** and `code` in one sentence.")
	if richHeight > plainHeight {
		t.Fatalf("inline formatting grew the paragraph from %.2fmm to %.2fmm", plainHeight, richHeight)
	}
}

// TestWriteBlocksPDFSeparatesParagraphs asserts a block boundary pushes the
// text further down than a plain line wrap does. The wrapping paragraph supplies
// the line advance to compare against, so the check does not depend on the
// chosen font size.
func TestWriteBlocksPDFSeparatesParagraphs(t *testing.T) {
	pdf := newExportPDF("probe")
	pdf.AddPage()
	writeBlocksPDF(pdf, markdownToBlocks(strings.Repeat("alpha ", 40)+"\n\nBeta paragraph."))
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output: %v", err)
	}
	ops := pdfTextOps(buf.Bytes())
	if len(ops) < 3 {
		t.Fatalf("expected the first paragraph to wrap, got %d drawn lines", len(ops))
	}
	lineAdvance := ops[0].Y - ops[1].Y
	last := len(ops) - 1
	gap := ops[last-1].Y - ops[last].Y
	if gap <= lineAdvance {
		t.Fatalf("paragraph gap %.2f is not larger than the line advance %.2f", gap, lineAdvance)
	}
}

// pdfBlockHeight renders markdown through the PDF block writer and reports how
// far down the page the cursor moved, in millimetres.
func pdfBlockHeight(t *testing.T, md string) float64 {
	t.Helper()
	pdf := newExportPDF("probe")
	pdf.AddPage()
	before := pdf.GetY()
	writeBlocksPDF(pdf, markdownToBlocks(md))
	if err := pdf.Error(); err != nil {
		t.Fatalf("fpdf error: %v", err)
	}
	return pdf.GetY() - before
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
