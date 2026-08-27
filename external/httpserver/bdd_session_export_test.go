//go:build http

package httpserver

// Godog harness for features/session_export.feature: exports a chat with a
// user/assistant exchange into JSON/HTML/PDF/DOCX over the live HTTP surface
// and asserts the response carries the right content type, disposition, and
// payload markers. Mirrors the server setup in bdd_chat_load_test.go without
// the concurrent-turn machinery, since export only reads persisted messages.
//
// Several scenarios inspect the rendered document itself rather than just its
// envelope: the PDF content stream is inflated and its text-showing operators
// are read back so paragraph layout can be asserted, and the DOCX parts are
// unzipped so the XML is checked for well-formedness and style resolution.

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const (
	exportUserQuestion    = "How do I export a session?"
	exportAssistantAnswer = "Use the download button in the chat header."
	exportReasoning       = "Considering the available formats."

	// The inline-formatting scenario asserts that these two fragments of one
	// sentence land on the same PDF baseline, and that the paragraph starting
	// with exportSecondParaLead is pushed further down than a mere line break.
	exportSentenceHead = "Some"
	exportSentenceTail = "in one sentence."
	exportSecondPara   = "Alpha"

	// Ambient editor state the agent appends to a user turn. A readable export
	// must keep the typed question and drop everything else here.
	exportTypedQuestion     = "Why does the build fail?"
	exportAmbientActiveFile = "src/broken_main.go"
	exportAmbientOpenTab    = "src/untouched_util.go"
	exportAmbientTerminal   = "zsh-build-shell"
)

type sessionExportFeatureState struct {
	root      string
	sessRoot  string
	ts        *httptest.Server
	srv       *Server
	sessionID string

	respStatus  int
	respHeaders http.Header
	respBody    []byte

	// Editor-delivery scenarios: the events a stand-in plugin saw, and the temp
	// directory the rendered files landed in so the scenario can clean it up.
	ideCh      chan ideEvent
	exportPath string
	exportDir  string
}

func (s *sessionExportFeatureState) reset() error {
	s.close()
	root, err := os.MkdirTemp("", "foxxycode-bdd-export-*")
	if err != nil {
		return err
	}
	*s = sessionExportFeatureState{root: root}
	return nil
}

func (s *sessionExportFeatureState) close() {
	if s.ideCh != nil {
		ideEvents.unsubscribe(s.ideCh)
		s.ideCh = nil
	}
	// The file route writes into the real OS temp dir, outside the scenario root.
	if s.exportDir != "" {
		_ = os.RemoveAll(s.exportDir)
		s.exportDir = ""
	}
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.srv != nil {
		s.srv.Drain()
		s.srv = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

func (s *sessionExportFeatureState) startServer() error {
	home := filepath.Join(s.root, "home")
	s.sessRoot = filepath.Join(s.root, "sessions")
	for _, d := range []string{filepath.Join(home, "memory"), s.sessRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// The runner is never invoked for export (we add messages directly), but the
	// manager still needs a non-nil one to build a session.
	runner := func(ctx context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: s.root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, &session.FileStore{Root: s.sessRoot})
	s.srv = New(cfg, mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(s.srv.Handler())

	sn, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: s.root})
	if err != nil {
		return err
	}
	s.sessionID = sn.SessionID
	return nil
}

// liveState returns the in-memory session the scenario seeds messages onto.
func (s *sessionExportFeatureState) liveState() (*session.State, error) {
	st := s.srv.mgr.SessionByID(s.sessionID)
	if st == nil {
		return nil, fmt.Errorf("session %q is not live", s.sessionID)
	}
	return st, nil
}

// seedChat persists a user/assistant exchange (with reasoning) onto the live
// session state, mirroring what a real turn leaves behind.
func (s *sessionExportFeatureState) seedChat() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion})
	st.AddMessage(llm.Message{
		Role:      llm.RoleAssistant,
		Content:   exportAssistantAnswer,
		Reasoning: exportReasoning,
	})
	return nil
}

// seedUserOnly leaves the transcript without any assistant answer, which is the
// state in which the panel hides the export action.
func (s *sessionExportFeatureState) seedUserOnly() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion})
	return nil
}

// seedInlineFormatting produces one sentence carrying bold and inline code plus
// a following paragraph long enough to wrap, so the PDF steps can compare the
// gap between paragraphs against the advance between wrapped lines.
func (s *sessionExportFeatureState) seedInlineFormatting() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	second := exportSecondPara + " " + strings.Repeat("filler words to force wrapping ", 6)
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion})
	st.AddMessage(llm.Message{
		Role:    llm.RoleAssistant,
		Content: exportSentenceHead + " **bold** and `code` " + exportSentenceTail + "\n\n" + second,
	})
	return nil
}

// seedEscapeCodes plants the kind of terminal output a user pastes into the
// composer: ANSI colour escapes plus a stray NUL, neither of which is legal in
// XML 1.0 character data.
func (s *sessionExportFeatureState) seedEscapeCodes() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	st.AddMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: "log line \x1b[31mred\x1b[0m and a NUL:\x00 tail",
	})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: exportAssistantAnswer})
	return nil
}

func (s *sessionExportFeatureState) seedDeepHeadings() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	var md strings.Builder
	for level := 1; level <= 6; level++ {
		md.WriteString(strings.Repeat("#", level) + " Level " + fmt.Sprint(level) + "\n\n")
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: md.String()})
	return nil
}

func (s *sessionExportFeatureState) seedLists() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion})
	st.AddMessage(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "- first bullet\n- second bullet\n\n1. first step\n2. second step",
	})
	return nil
}

// exportTableMarkdown is a GFM pipe table with a header, two body rows and one
// explicitly aligned column — the shape that used to reach every readable format
// as a paragraph of "|" characters.
const exportTableMarkdown = "| Format | Editable | Notes |\n" +
	"|--------|:--------:|-------|\n" +
	"| pdf    | no       | fixed layout |\n" +
	"| docx   | yes      | edit in Word |"

func (s *sessionExportFeatureState) seedTable() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion})
	st.AddMessage(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "Here is the comparison:\n\n" + exportTableMarkdown,
	})
	return nil
}

// tableLaidOutAsGrid asserts the table reached the document as a real grid, in
// whatever way that format expresses one: markup for HTML and DOCX, drawn cell
// rectangles for PDF.
func (s *sessionExportFeatureState) tableLaidOutAsGrid(format string) error {
	switch format {
	case "html":
		body := string(s.respBody)
		if !strings.Contains(body, "<table>") || strings.Count(body, "<tr>") != 3 {
			return fmt.Errorf("HTML export has no three-row table")
		}
		// Counted on the closing tag: "<th" would also match "<thead>".
		if n := strings.Count(body, "</th>"); n != 3 {
			return fmt.Errorf("HTML table has %d header cells, want 3", n)
		}
		return nil
	case "docx":
		body, err := s.docxPart("word/document.xml")
		if err != nil {
			return err
		}
		if strings.Count(body, "<w:tr>") != 3 || strings.Count(body, "<w:tc>") != 9 {
			return fmt.Errorf("DOCX export has no 3x3 table, got %d rows and %d cells",
				strings.Count(body, "<w:tr>"), strings.Count(body, "<w:tc>"))
		}
		if !strings.Contains(body, "<w:tblHeader/>") {
			return fmt.Errorf("DOCX table header does not repeat across pages")
		}
		return nil
	case "pdf":
		// Every cell is drawn as its own rectangle; three rows of three columns.
		if n := pdfDrawnRectCount(s.respBody); n != 9 {
			return fmt.Errorf("PDF drew %d cell rectangles, want 9", n)
		}
		return nil
	}
	return fmt.Errorf("unsupported format %q", format)
}

// tableShowsNoPipes asserts the source pipes did not survive as prose.
func (s *sessionExportFeatureState) tableShowsNoPipes(format string) error {
	text, err := s.exportedText()
	if err != nil {
		return err
	}
	if strings.Contains(text, "|") {
		return fmt.Errorf("the %s export still shows raw pipe characters", format)
	}
	return nil
}

// pdfDrawnRectCount counts the rectangles drawn in a PDF's content streams.
// Table cells and code boxes are the only things the export draws with them.
func pdfDrawnRectCount(pdfBytes []byte) int {
	count := 0
	for _, m := range pdfStreamRe.FindAllSubmatch(pdfBytes, -1) {
		zr, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		content, err := io.ReadAll(zr)
		_ = zr.Close()
		if err != nil {
			continue
		}
		count += len(pdfRectRe.FindAll(content, -1))
	}
	return count
}

var pdfRectRe = regexp.MustCompile(`[0-9.]+ [0-9.]+ [0-9.]+ -?[0-9.]+ re`)

// seedInjectedContext reproduces what the agent appends to a user turn each
// time: the editor's ambient IDE state and a summary of the open terminals.
// Shapes mirror internal/agent/react.go.
func (s *sessionExportFeatureState) seedInjectedContext() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	content := exportTypedQuestion +
		"\n\n<foxxycode_ide_context>\n# Active File\n" + exportAmbientActiveFile +
		"\n\n# Open Tabs\n" + exportAmbientActiveFile + "\n" + exportAmbientOpenTab +
		"\n</foxxycode_ide_context>" +
		"\n\n<foxxycode_terminal_context>\n# Active Terminal: " + exportAmbientTerminal +
		"\n$ go build ./...\nok\n</foxxycode_terminal_context>"
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: content})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: exportAssistantAnswer})
	return nil
}

// exportedText renders the response back into the text a reader would see, so
// one step can assert on any format.
func (s *sessionExportFeatureState) exportedText() (string, error) {
	ct := s.respHeaders.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "application/pdf"):
		var b strings.Builder
		for _, op := range pdfTextOps(s.respBody) {
			b.WriteString(op.Text)
			b.WriteByte('\n')
		}
		return b.String(), nil
	case strings.HasPrefix(ct, "application/vnd.openxmlformats"):
		return s.docxPart("word/document.xml")
	default:
		return string(s.respBody), nil
	}
}

func (s *sessionExportFeatureState) documentKeepsTypedQuestion() error {
	text, err := s.exportedText()
	if err != nil {
		return err
	}
	if !strings.Contains(text, exportTypedQuestion) {
		return fmt.Errorf("the exported document lost the question the user typed")
	}
	return nil
}

func (s *sessionExportFeatureState) documentHidesAmbientContext() error {
	text, err := s.exportedText()
	if err != nil {
		return err
	}
	for _, leak := range []string{
		"Active File", "Open Tabs", "Active Terminal",
		exportAmbientActiveFile, exportAmbientOpenTab, exportAmbientTerminal,
		"foxxycode_ide_context", "foxxycode_terminal_context",
	} {
		if strings.Contains(text, leak) {
			return fmt.Errorf("the exported document still shows the ambient context %q", leak)
		}
	}
	return nil
}

func (s *sessionExportFeatureState) jsonKeepsInjectedContext() error {
	body := string(s.respBody)
	for _, want := range []string{"foxxycode_ide_context", "foxxycode_terminal_context", exportAmbientActiveFile} {
		if !strings.Contains(body, want) {
			return fmt.Errorf("the JSON export dropped %q, which re-import needs", want)
		}
	}
	return nil
}

func (s *sessionExportFeatureState) seedTitled(title string) error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	st.SetTitlePinned(title)
	return s.seedChat()
}

// requestExport issues GET .../export?format=<f> and records the response.
func (s *sessionExportFeatureState) requestExport(format, id string) error {
	u := s.ts.URL + "/foxxycode/sessions/" + url.PathEscape(id) + "/export?format=" + url.QueryEscape(format)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	s.respStatus = res.StatusCode
	s.respHeaders = res.Header
	s.respBody = body
	return nil
}

func (s *sessionExportFeatureState) exportChat(format string) error {
	return s.requestExport(format, s.sessionID)
}

func (s *sessionExportFeatureState) exportMissingChat(format string) error {
	return s.requestExport(format, "sess_does_not_exist")
}

func (s *sessionExportFeatureState) jsonAttachment() error {
	if s.respStatus != http.StatusOK {
		return fmt.Errorf("expected 200, got %d", s.respStatus)
	}
	if ct := s.respHeaders.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		return fmt.Errorf("expected application/json content type, got %q", ct)
	}
	cd := s.respHeaders.Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, ".json") {
		return fmt.Errorf("unexpected content-disposition: %q", cd)
	}
	return nil
}

func (s *sessionExportFeatureState) attachmentOfType(expected string) error {
	if s.respStatus != http.StatusOK {
		return fmt.Errorf("expected 200, got %d", s.respStatus)
	}
	if ct := s.respHeaders.Get("Content-Type"); !strings.HasPrefix(ct, expected) {
		return fmt.Errorf("expected %q content type, got %q", expected, ct)
	}
	if cd := s.respHeaders.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		return fmt.Errorf("missing attachment disposition: %q", cd)
	}
	return nil
}

func (s *sessionExportFeatureState) jsonContainsQA() error {
	var doc exportDocument
	if err := json.Unmarshal(s.respBody, &doc); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var hasUser, hasAssistant bool
	for _, m := range doc.Messages {
		if m.Role == "user" && strings.Contains(m.Content, exportUserQuestion) {
			hasUser = true
		}
		if m.Role == "assistant" && strings.Contains(m.Content, exportAssistantAnswer) {
			hasAssistant = true
		}
	}
	if !hasUser || !hasAssistant {
		return fmt.Errorf("JSON missing Q/A: %+v", doc.Messages)
	}
	return nil
}

func (s *sessionExportFeatureState) htmlContainsAnswer() error {
	body := string(s.respBody)
	if !strings.Contains(body, exportAssistantAnswer) {
		return fmt.Errorf("HTML body missing the assistant answer")
	}
	return nil
}

func (s *sessionExportFeatureState) pdfHeader() error {
	if !bytes.HasPrefix(s.respBody, []byte("%PDF")) {
		return fmt.Errorf("PDF payload does not start with %%PDF")
	}
	return nil
}

func (s *sessionExportFeatureState) validDocx() error {
	zr, err := zip.NewReader(bytes.NewReader(s.respBody), int64(len(s.respBody)))
	if err != nil {
		return fmt.Errorf("DOCX is not a valid zip: %w", err)
	}
	found := false
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("DOCX package missing word/document.xml")
	}
	return nil
}

func (s *sessionExportFeatureState) rejectedWithStatus(code int) error {
	if s.respStatus != code {
		return fmt.Errorf("expected status %d, got %d", code, s.respStatus)
	}
	return nil
}

// --- PDF layout steps -----------------------------------------------------

// pdfTextOp is one text-showing operation lifted out of the PDF content stream,
// carrying the absolute baseline position the text was drawn at.
type pdfTextOp struct {
	X, Y float64
	Text string
}

var (
	pdfStreamRe = regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`)
	pdfTextRe   = regexp.MustCompile(`(?s)BT ([0-9.]+) ([0-9.]+) Td \(((?:\\.|[^\\)])*)\)Tj ET`)
)

// pdfTextOps inflates every FlateDecode content stream in the document and
// returns the drawn text runs in document order. fpdf writes UTF-8 font text as
// UTF-16BE inside a literal PDF string, so the payload is unescaped and decoded
// back into a Go string.
func pdfTextOps(pdfBytes []byte) []pdfTextOp {
	var ops []pdfTextOp
	for _, m := range pdfStreamRe.FindAllSubmatch(pdfBytes, -1) {
		zr, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		content, err := io.ReadAll(zr)
		_ = zr.Close()
		if err != nil {
			continue
		}
		for _, t := range pdfTextRe.FindAllSubmatch(content, -1) {
			var x, y float64
			if _, err := fmt.Sscanf(string(t[1])+" "+string(t[2]), "%f %f", &x, &y); err != nil {
				continue
			}
			ops = append(ops, pdfTextOp{X: x, Y: y, Text: decodePDFString(t[3])})
		}
	}
	return ops
}

// decodePDFString undoes PDF literal-string escaping and decodes the UTF-16BE
// payload fpdf emits for embedded UTF-8 fonts.
func decodePDFString(raw []byte) string {
	var b []byte
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			b = append(b, raw[i])
			continue
		}
		i++
		switch c := raw[i]; c {
		case 'n':
			b = append(b, '\n')
		case 'r':
			b = append(b, '\r')
		case 't':
			b = append(b, '\t')
		case '(', ')', '\\':
			b = append(b, c)
		default:
			if c >= '0' && c <= '7' {
				v, n := 0, 0
				for i < len(raw) && n < 3 && raw[i] >= '0' && raw[i] <= '7' {
					v = v*8 + int(raw[i]-'0')
					i++
					n++
				}
				i--
				b = append(b, byte(v))
				continue
			}
			b = append(b, c)
		}
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(units))
}

// findPDFOp returns the index of the first drawn run containing needle.
func findPDFOp(ops []pdfTextOp, needle string) int {
	for i, op := range ops {
		if strings.Contains(op.Text, needle) {
			return i
		}
	}
	return -1
}

// pdfSentenceSingleLine asserts the head and tail of one sentence share a
// baseline, i.e. the inline bold/code runs did not each start a new line.
func (s *sessionExportFeatureState) pdfSentenceSingleLine() error {
	ops := pdfTextOps(s.respBody)
	head := findPDFOp(ops, exportSentenceHead)
	tail := findPDFOp(ops, exportSentenceTail)
	if head < 0 || tail < 0 {
		return fmt.Errorf("sentence fragments not found in PDF text (head=%d tail=%d, ops=%d)", head, tail, len(ops))
	}
	if ops[head].Y != ops[tail].Y {
		return fmt.Errorf("sentence split across lines: %q at y=%.2f, %q at y=%.2f",
			exportSentenceHead, ops[head].Y, exportSentenceTail, ops[tail].Y)
	}
	if ops[tail].X <= ops[head].X {
		return fmt.Errorf("sentence tail does not continue to the right of the head (x %.2f -> %.2f)",
			ops[head].X, ops[tail].X)
	}
	return nil
}

// pdfParagraphsSpaced asserts the step down to the next paragraph is larger
// than the step between two wrapped lines inside a paragraph. The wrapped-line
// advance is measured from the document itself so the check stays independent
// of the chosen font size.
func (s *sessionExportFeatureState) pdfParagraphsSpaced() error {
	ops := pdfTextOps(s.respBody)
	sentence := findPDFOp(ops, exportSentenceTail)
	second := findPDFOp(ops, exportSecondPara)
	if sentence < 0 || second < 0 {
		return fmt.Errorf("paragraphs not found in PDF text (sentence=%d second=%d)", sentence, second)
	}
	if second+1 >= len(ops) {
		return fmt.Errorf("second paragraph did not wrap, cannot measure the line advance")
	}
	lineAdvance := ops[second].Y - ops[second+1].Y
	if lineAdvance <= 0 {
		return fmt.Errorf("unexpected line advance %.2f inside the wrapped paragraph", lineAdvance)
	}
	gap := ops[sentence].Y - ops[second].Y
	if gap <= lineAdvance {
		return fmt.Errorf("no space between paragraphs: gap %.2f is not larger than the line advance %.2f", gap, lineAdvance)
	}
	return nil
}

// --- DOCX steps -----------------------------------------------------------

func (s *sessionExportFeatureState) docxPart(name string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(s.respBody), int64(len(s.respBody)))
	if err != nil {
		return "", fmt.Errorf("DOCX is not a valid zip: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		buf, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
		return string(buf), nil
	}
	return "", fmt.Errorf("DOCX part %q not found", name)
}

func (s *sessionExportFeatureState) docxWellFormed() error {
	body, err := s.docxPart("word/document.xml")
	if err != nil {
		return err
	}
	dec := xml.NewDecoder(strings.NewReader(body))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("word/document.xml is not well-formed XML: %w", err)
		}
	}
}

func (s *sessionExportFeatureState) docxSnippetTextSurvives() error {
	body, err := s.docxPart("word/document.xml")
	if err != nil {
		return err
	}
	for _, want := range []string{"log line", "red", "tail"} {
		if !strings.Contains(body, want) {
			return fmt.Errorf("word/document.xml lost the text %q around the escape codes", want)
		}
	}
	return nil
}

var (
	docxUsedStyleRe    = regexp.MustCompile(`<w:pStyle w:val="([^"]+)"/>`)
	docxDefinedStyleRe = regexp.MustCompile(`w:styleId="([^"]+)"`)
)

func (s *sessionExportFeatureState) docxStylesResolved() error {
	body, err := s.docxPart("word/document.xml")
	if err != nil {
		return err
	}
	styles, err := s.docxPart("word/styles.xml")
	if err != nil {
		return err
	}
	defined := map[string]bool{}
	for _, m := range docxDefinedStyleRe.FindAllStringSubmatch(styles, -1) {
		defined[m[1]] = true
	}
	for _, m := range docxUsedStyleRe.FindAllStringSubmatch(body, -1) {
		if !defined[m[1]] {
			return fmt.Errorf("document uses paragraph style %q which styles.xml does not define", m[1])
		}
	}
	return nil
}

func (s *sessionExportFeatureState) docxNoLiteralMarker() error {
	body, err := s.docxPart("word/document.xml")
	if err != nil {
		return err
	}
	for _, glyph := range []string{"•", "–  "} {
		if strings.Contains(body, glyph) {
			return fmt.Errorf("document.xml carries the literal list marker %q on top of the numbering definition", glyph)
		}
	}
	return nil
}

func (s *sessionExportFeatureState) docxOrderedNumbering() error {
	body, err := s.docxPart("word/document.xml")
	if err != nil {
		return err
	}
	numbering, err := s.docxPart("word/numbering.xml")
	if err != nil {
		return err
	}
	used := regexp.MustCompile(`<w:numId w:val="([0-9]+)"/>`).FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	for _, m := range used {
		seen[m[1]] = true
	}
	if len(seen) < 2 {
		return fmt.Errorf("bullet and numbered lists share the same numbering id %v", seen)
	}
	if !strings.Contains(numbering, `w:numFmt w:val="decimal"`) {
		return fmt.Errorf("numbering.xml defines no decimal list format")
	}
	return nil
}

// --- Editor delivery steps -------------------------------------------------

// listenAsPlugin stands in for a connected IntelliJ / VS Code plugin by
// subscribing to the same process-global hub the SSE route serves from.
func (s *sessionExportFeatureState) listenAsPlugin() error {
	s.ideCh = ideEvents.subscribe()
	return nil
}

// exportToFile drives the editor delivery route, which writes the document to
// disk instead of answering with an attachment.
func (s *sessionExportFeatureState) exportToFile(format string) error {
	u := s.ts.URL + "/foxxycode/sessions/" + url.PathEscape(s.sessionID) + "/export/file?format=" + url.QueryEscape(format)
	req, err := http.NewRequest(http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-FoxxyCode-Session-ID", s.sessionID)
	res, err := s.ts.Client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	s.respStatus = res.StatusCode
	s.respHeaders = res.Header
	s.respBody = body
	if res.StatusCode == http.StatusOK {
		var payload struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("invalid JSON from the file route: %w", err)
		}
		s.exportPath = payload.Path
		s.exportDir = filepath.Dir(payload.Path)
	}
	return nil
}

// blockPreviousExport stands in for a document the user still has open: a
// directory where the file goes cannot be written either, which is the same
// condition the writer sees when Windows locks an open .docx.
func (s *sessionExportFeatureState) blockPreviousExport() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	title := strings.TrimSpace(st.GetTitlePinned())
	if title == "" {
		title = s.sessionID
	}
	dir := filepath.Join(os.TempDir(), exportTempSubdir, "exports", exportBaseName(s.sessionID, "session"))
	s.exportDir = dir
	return os.MkdirAll(filepath.Join(dir, exportBaseName(title, s.sessionID)+".docx"), 0o700)
}

func (s *sessionExportFeatureState) fileNameHasNumericSuffix() error {
	stem := strings.TrimSuffix(filepath.Base(s.exportPath), filepath.Ext(s.exportPath))
	if !regexp.MustCompile(`_\d+$`).MatchString(stem) {
		return fmt.Errorf("file %q carries no numeric suffix; the blocked name was reused", stem)
	}
	return nil
}

func (s *sessionExportFeatureState) exportedFileReadable(format string) error {
	if s.respStatus != http.StatusOK {
		return fmt.Errorf("expected 200, got %d (%s)", s.respStatus, s.respBody)
	}
	if !filepath.IsAbs(s.exportPath) {
		return fmt.Errorf("path %q is not absolute; the plugin cannot open it", s.exportPath)
	}
	if ext := strings.TrimPrefix(filepath.Ext(s.exportPath), "."); ext != format {
		return fmt.Errorf("file %q does not carry the %s extension", s.exportPath, format)
	}
	body, err := os.ReadFile(s.exportPath)
	if err != nil {
		return fmt.Errorf("the exported file is not readable: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("the exported file is empty")
	}
	if format == "pdf" && !bytes.HasPrefix(body, []byte("%PDF")) {
		return fmt.Errorf("the exported file is not a PDF")
	}
	return nil
}

func (s *sessionExportFeatureState) fileNamedAfterTitle() error {
	name := filepath.Base(s.exportPath)
	if !strings.HasPrefix(name, "Отчёт_по_задаче.") {
		return fmt.Errorf("file %q is not named after the chat title", name)
	}
	return nil
}

func (s *sessionExportFeatureState) ideAskedToReveal() error {
	if s.ideCh == nil {
		return fmt.Errorf("no plugin is listening in this scenario")
	}
	for {
		select {
		case ev := <-s.ideCh:
			if ev.Type != "reveal_file" {
				continue // ignore unrelated edit traffic
			}
			if ev.Path != s.exportPath {
				return fmt.Errorf("IDE asked to reveal %q, want %q", ev.Path, s.exportPath)
			}
			if ev.SessionID != s.sessionID {
				return fmt.Errorf("reveal event carries session %q, want %q", ev.SessionID, s.sessionID)
			}
			return nil
		default:
			return fmt.Errorf("no reveal_file event reached the plugin")
		}
	}
}

func (s *sessionExportFeatureState) dirHoldsOneFile(format string) error {
	entries, err := os.ReadDir(s.exportDir)
	if err != nil {
		return err
	}
	var matched []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "."+format) {
			matched = append(matched, e.Name())
		}
	}
	if len(matched) != 1 {
		return fmt.Errorf("expected exactly one %s file, found %v", format, matched)
	}
	return nil
}

// --- Content-Disposition steps --------------------------------------------

func (s *sessionExportFeatureState) dispositionUTF8Name(want string) error {
	cd := s.respHeaders.Get("Content-Disposition")
	marker := "filename*=UTF-8''"
	idx := strings.Index(cd, marker)
	if idx < 0 {
		return fmt.Errorf("Content-Disposition carries no RFC 5987 filename*: %q", cd)
	}
	got, err := url.PathUnescape(strings.TrimSpace(cd[idx+len(marker):]))
	if err != nil {
		return fmt.Errorf("filename* is not valid percent-encoding: %w", err)
	}
	if got != want {
		return fmt.Errorf("filename* decodes to %q, want %q", got, want)
	}
	return nil
}

func (s *sessionExportFeatureState) dispositionASCIIFallback() error {
	cd := s.respHeaders.Get("Content-Disposition")
	m := regexp.MustCompile(`filename="([^"]*)"`).FindStringSubmatch(cd)
	if m == nil {
		return fmt.Errorf("Content-Disposition carries no plain filename fallback: %q", cd)
	}
	name := m[1]
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("plain filename fallback is empty: %q", cd)
	}
	for _, r := range name {
		if r > 127 {
			return fmt.Errorf("plain filename fallback %q is not ASCII (clients cannot read it)", name)
		}
	}
	if !strings.HasSuffix(name, ".pdf") {
		return fmt.Errorf("plain filename fallback %q lost its extension", name)
	}
	return nil
}

func initializeSessionExportScenario(sc *godog.ScenarioContext) {
	s := &sessionExportFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server$`, s.startServer)
	sc.Step(`^a chat with a user question and an assistant answer$`, s.seedChat)
	sc.Step(`^a chat with only a user question$`, s.seedUserOnly)
	sc.Step(`^a chat whose answer mixes bold and inline code inside one sentence$`, s.seedInlineFormatting)
	sc.Step(`^a chat whose question carries terminal escape codes$`, s.seedEscapeCodes)
	sc.Step(`^a chat whose answer uses headings from level one to level six$`, s.seedDeepHeadings)
	sc.Step(`^a chat whose answer contains a bullet list and a numbered list$`, s.seedLists)
	sc.Step(`^a chat whose answer contains a markdown table$`, s.seedTable)
	sc.Step(`^a chat titled "([^"]*)" with an assistant answer$`, s.seedTitled)
	sc.Step(`^a chat whose question carries injected IDE and terminal context$`, s.seedInjectedContext)

	sc.Step(`^an editor plugin listening for IDE events$`, s.listenAsPlugin)
	sc.Step(`^the previously exported document cannot be replaced$`, s.blockPreviousExport)

	sc.Step(`^the panel exports the chat as (\w+)$`, s.exportChat)
	sc.Step(`^the panel exports a non-existent chat as (\w+)$`, s.exportMissingChat)
	sc.Step(`^the editor panel exports the chat to a file as (\w+)$`, s.exportToFile)

	sc.Step(`^the response is a downloadable JSON attachment$`, s.jsonAttachment)
	sc.Step(`^the response is a downloadable attachment of type (text/html|application/pdf|application/vnd\.openxmlformats-officedocument\.wordprocessingml\.document)$`, s.attachmentOfType)
	sc.Step(`^the JSON contains the user question and the assistant answer$`, s.jsonContainsQA)
	sc.Step(`^the HTML body contains the assistant answer$`, s.htmlContainsAnswer)
	sc.Step(`^the PDF payload begins with the PDF header$`, s.pdfHeader)
	sc.Step(`^the DOCX payload is a valid Office Open XML package$`, s.validDocx)
	sc.Step(`^the export request is rejected with status (\d+)$`, s.rejectedWithStatus)

	sc.Step(`^the sentence is laid out on a single line of the PDF$`, s.pdfSentenceSingleLine)
	sc.Step(`^consecutive paragraphs are separated by vertical space$`, s.pdfParagraphsSpaced)
	sc.Step(`^the DOCX document part is well-formed XML$`, s.docxWellFormed)
	sc.Step(`^the surrounding text of the snippet survives$`, s.docxSnippetTextSurvives)
	sc.Step(`^every paragraph style used by the document is defined in the style sheet$`, s.docxStylesResolved)
	sc.Step(`^no list item repeats the marker glyph in its own text$`, s.docxNoLiteralMarker)
	sc.Step(`^the numbered list is numbered by the document rather than bulleted$`, s.docxOrderedNumbering)
	sc.Step(`^the (\w+) export lays the table out as a grid$`, s.tableLaidOutAsGrid)
	sc.Step(`^the (\w+) export shows no raw pipe characters$`, s.tableShowsNoPipes)
	sc.Step(`^the attachment offers the UTF-8 filename "([^"]*)"$`, s.dispositionUTF8Name)
	sc.Step(`^the attachment keeps an ASCII filename fallback$`, s.dispositionASCIIFallback)

	sc.Step(`^the document still carries the question the user typed$`, s.documentKeepsTypedQuestion)
	sc.Step(`^the document shows no active file, open tabs or terminal section$`, s.documentHidesAmbientContext)
	sc.Step(`^the JSON still carries the injected context blocks$`, s.jsonKeepsInjectedContext)

	sc.Step(`^the response carries the absolute path of a readable (\w+) file$`, s.exportedFileReadable)
	sc.Step(`^the file is named after the chat title$`, s.fileNamedAfterTitle)
	sc.Step(`^the file name carries a numeric suffix$`, s.fileNameHasNumericSuffix)
	sc.Step(`^the IDE is asked to reveal the exported file$`, s.ideAskedToReveal)
	sc.Step(`^the export directory holds exactly one (\w+) file$`, s.dirHoldsOneFile)
}

func TestSessionExportFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "session-export",
		ScenarioInitializer: initializeSessionExportScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/session_export.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("session export feature suite failed")
	}
}
