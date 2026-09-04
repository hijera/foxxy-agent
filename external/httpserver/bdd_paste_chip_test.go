//go:build http

package httpserver

// Godog harness for features/ide_paste_chip.feature: drives the live HTTP
// surface (copy-buffer ingest, paste classification) to prove that a fragment
// copied in the IDE and pasted 1:1 into the composer classifies as a mention.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/idecopy"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type pasteChipState struct {
	root   string
	ts     *httptest.Server
	srv    *Server
	result map[string]interface{}
}

func (s *pasteChipState) reset() error {
	s.close()
	idecopy.Reset()
	ideenv.Reset()
	ideterm.Reset()
	root, err := os.MkdirTemp("", "foxxycode-bdd-pastechip-*")
	if err != nil {
		return err
	}
	s.root = root
	s.result = nil
	return nil
}

func (s *pasteChipState) close() {
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
	idecopy.Reset()
	ideenv.Reset()
	ideterm.Reset()
}

func (s *pasteChipState) startServer() error {
	home := filepath.Join(s.root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:  config.Paths{Home: home, CWD: s.root},
		Models: []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:  config.Agent{Model: "openai/gpt-4o"},
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, nil)
	s.srv = New(cfg, mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(s.srv.Handler())
	return nil
}

func (s *pasteChipState) post(path string, payload interface{}) (int, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	res, err := http.Post(s.ts.URL+path, "application/json", bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	s.result = nil
	_ = json.NewDecoder(res.Body).Decode(&s.result)
	return res.StatusCode, nil
}

func (s *pasteChipState) ideReportedFileCopy(start, end int, rel string, text *godog.DocString) error {
	status, err := s.post("/foxxycode/ide/copy-buffer", map[string]interface{}{
		"kind":      "file",
		"path":      filepath.Join(s.root, filepath.FromSlash(rel)),
		"startLine": start,
		"endLine":   end,
		"text":      text.Content,
	})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("copy-buffer returned %d", status)
	}
	return nil
}

func (s *pasteChipState) ideReportedTerminal(name string, output *godog.DocString) error {
	status, err := s.post("/foxxycode/ide/terminal-state", map[string]interface{}{
		"terminals": []map[string]interface{}{
			{"id": "1", "name": name, "output": output.Content, "active": true},
		},
	})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("terminal-state returned %d", status)
	}
	return nil
}

func (s *pasteChipState) composerClassifies(text *godog.DocString) error {
	status, err := s.post("/foxxycode/ide/paste-classify", map[string]interface{}{"text": text.Content})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("paste-classify returned %d: %v", status, s.result)
	}
	return nil
}

func (s *pasteChipState) classificationIsFileMention(rel string, start, end int) error {
	if s.result["kind"] != "file" {
		return fmt.Errorf("kind = %v, want file (%v)", s.result["kind"], s.result)
	}
	if s.result["pathRel"] != rel {
		return fmt.Errorf("pathRel = %v, want %q", s.result["pathRel"], rel)
	}
	if int(s.result["startLine"].(float64)) != start || int(s.result["endLine"].(float64)) != end {
		return fmt.Errorf("lines = %v-%v, want %d-%d", s.result["startLine"], s.result["endLine"], start, end)
	}
	return nil
}

func (s *pasteChipState) classificationIsTerminalMention(name string) error {
	if s.result["kind"] != "terminal" {
		return fmt.Errorf("kind = %v, want terminal (%v)", s.result["kind"], s.result)
	}
	if s.result["terminalName"] != name {
		return fmt.Errorf("terminalName = %v, want %q", s.result["terminalName"], name)
	}
	return nil
}

func (s *pasteChipState) classificationIsNone() error {
	if s.result["kind"] != "none" {
		return fmt.Errorf("kind = %v, want none (%v)", s.result["kind"], s.result)
	}
	return nil
}

func initializePasteChipScenario(sc *godog.ScenarioContext) {
	s := &pasteChipState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server$`, s.startServer)
	sc.Step(`^the IDE reported copying lines (\d+)-(\d+) of workspace file "([^"]+)" as:$`, s.ideReportedFileCopy)
	sc.Step(`^the IDE reported terminal "([^"]+)" with output:$`, s.ideReportedTerminal)
	sc.Step(`^the composer classifies the pasted text:$`, s.composerClassifies)
	sc.Step(`^the classification is a file mention of "([^"]+)" lines (\d+)-(\d+)$`, s.classificationIsFileMention)
	sc.Step(`^the classification is a terminal mention of "([^"]+)"$`, s.classificationIsTerminalMention)
	sc.Step(`^the classification is none$`, s.classificationIsNone)
}

func TestIdePasteChipFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "ide-paste-chip",
		ScenarioInitializer: initializePasteChipScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/ide_paste_chip.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("ide paste chip feature suite failed")
	}
}
