//go:build http

package httpserver

// Godog harness for features/session_export.feature: exports a chat with a
// user/assistant exchange into JSON/HTML/PDF/DOCX over the live HTTP surface
// and asserts the response carries the right content type, disposition, and
// payload markers. Mirrors the server setup in bdd_chat_load_test.go without
// the concurrent-turn machinery, since export only reads persisted messages.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const (
	exportUserQuestion = "How do I export a session?"
	exportAssistantAnswer = "Use the download button in the chat header."
	exportReasoning       = "Considering the available formats."
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

// seedChat persists a user/assistant exchange (with reasoning) onto the live
// session state, mirroring what a real turn leaves behind.
func (s *sessionExportFeatureState) seedChat() error {
	st := s.srv.mgr.SessionByID(s.sessionID)
	if st == nil {
		return fmt.Errorf("session %q is not live", s.sessionID)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion})
	st.AddMessage(llm.Message{
		Role:      llm.RoleAssistant,
		Content:   exportAssistantAnswer,
		Reasoning: exportReasoning,
	})
	return nil
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
	if cd := s.respHeaders.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") || !strings.HasSuffix(cd, ".json\"") {
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

	sc.Step(`^the panel exports the chat as (\w+)$`, s.exportChat)
	sc.Step(`^the panel exports a non-existent chat as (\w+)$`, s.exportMissingChat)

	sc.Step(`^the response is a downloadable JSON attachment$`, s.jsonAttachment)
	sc.Step(`^the response is a downloadable attachment of type (text/html|application/pdf|application/vnd\.openxmlformats-officedocument\.wordprocessingml\.document)$`, s.attachmentOfType)
	sc.Step(`^the JSON contains the user question and the assistant answer$`, s.jsonContainsQA)
	sc.Step(`^the HTML body contains the assistant answer$`, s.htmlContainsAnswer)
	sc.Step(`^the PDF payload begins with the PDF header$`, s.pdfHeader)
	sc.Step(`^the DOCX payload is a valid Office Open XML package$`, s.validDocx)
	sc.Step(`^the export request is rejected with status (\d+)$`, s.rejectedWithStatus)
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
