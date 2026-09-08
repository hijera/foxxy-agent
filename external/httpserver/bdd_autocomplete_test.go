//go:build http

package httpserver

// Godog harness for features/code_autocomplete.feature: an editor plugin asks
// POST /foxxycode/completion for the text to insert at the caret, and reads
// GET /foxxycode/completion/config to learn when to ask.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type autocompleteFeatureState struct {
	llmSrv *httptest.Server
	ts     *httptest.Server
	root   string

	// promptSeen is the user message the fake model received.
	promptSeen string

	completion string
	clientCfg  map[string]interface{}
}

func (s *autocompleteFeatureState) reset() error {
	s.close()
	s.promptSeen = ""
	s.completion = ""
	s.clientCfg = nil
	root, err := os.MkdirTemp("", "foxxycode-autocomplete-bdd")
	if err != nil {
		return err
	}
	s.root = root
	return nil
}

func (s *autocompleteFeatureState) close() {
	if s.ts != nil {
		s.ts.Close()
		s.ts = nil
	}
	if s.llmSrv != nil {
		s.llmSrv.Close()
		s.llmSrv = nil
	}
	if s.root != "" {
		_ = os.RemoveAll(s.root)
		s.root = ""
	}
}

func (s *autocompleteFeatureState) startServerAutocompleteEnabled() error {
	// A realistically messy reply: fenced, and re-typing the caret's own line. The fake answers
	// streamed chat requests too, which is how the server asks so it can cut a reply short.
	fake := &fakeLLM{chatReply: "```go\nreturn a + b\n```"}
	s.llmSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(buf.Bytes(), &body)
		for _, m := range body.Messages {
			if m.Role == "user" {
				s.promptSeen = m.Content
			}
		}
		r.Body = io.NopCloser(&buf)
		fake.serve(w, r)
	}))

	enabled := true
	cfg := &config.Config{
		Paths: config.Paths{Home: s.root, CWD: s.root},
		Providers: []config.ProviderConfig{
			{Name: "openai", Type: "openai", APIBase: s.llmSrv.URL + "/v1", APIKey: "sk-test"},
		},
		Models: []config.ModelEntry{
			{Model: "openai/gpt-4o", MaxTokens: 4096, Temperature: 0.2},
		},
		Agent:        config.Agent{Model: "openai/gpt-4o"},
		Autocomplete: config.AutocompleteConfig{Enabled: &enabled},
	}
	cfg.Autocomplete.ApplyDefaults()

	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), s.root, nil)
	srv := New(cfg, mgr, slog.Default(), s.root)
	s.ts = httptest.NewServer(srv.Handler())
	return nil
}

func (s *autocompleteFeatureState) requestCompletionInsideFunction() error {
	body := `{"prefix":"func add(a, b int) int {\n\treturn ","suffix":"\n}",` +
		`"path":"main.go","language":"go"}`
	res, err := http.Post(s.ts.URL+"/foxxycode/completion", "application/json", strings.NewReader(body))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /foxxycode/completion status %d: %s", res.StatusCode, string(raw))
	}
	var parsed struct {
		Completion string `json:"completion"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("decode completion body: %w", err)
	}
	s.completion = parsed.Completion
	return nil
}

func (s *autocompleteFeatureState) suggestionIsInsertable() error {
	// "return " is already typed, so re-inserting it would produce "return return a + b";
	// the fence would be inserted as literal backticks.
	if strings.Contains(s.completion, "```") {
		return fmt.Errorf("suggestion still carries a markdown fence: %q", s.completion)
	}
	if strings.HasPrefix(s.completion, "return ") {
		return fmt.Errorf("suggestion repeats the caret line: %q", s.completion)
	}
	if strings.TrimSpace(s.completion) != "a + b" {
		return fmt.Errorf("suggestion = %q, want %q", s.completion, "a + b")
	}
	return nil
}

func (s *autocompleteFeatureState) modelSawBothSidesOfCaret() error {
	if !strings.Contains(s.promptSeen, autocompleteCursor) {
		return fmt.Errorf("prompt has no caret marker: %q", s.promptSeen)
	}
	before, after, ok := strings.Cut(s.promptSeen, autocompleteCursor)
	if !ok {
		return fmt.Errorf("prompt has no caret marker: %q", s.promptSeen)
	}
	if !strings.Contains(before, "func add(a, b int) int {") {
		return fmt.Errorf("prompt is missing the code before the caret: %q", before)
	}
	if !strings.Contains(after, "}") {
		return fmt.Errorf("prompt is missing the code after the caret: %q", after)
	}
	return nil
}

func (s *autocompleteFeatureState) readClientConfig() error {
	res, err := http.Get(s.ts.URL + "/foxxycode/completion/config")
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /foxxycode/completion/config status %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(&s.clientCfg)
}

func (s *autocompleteFeatureState) clientConfigDescribesBehaviour() error {
	if enabled, _ := s.clientCfg["enabled"].(bool); !enabled {
		return fmt.Errorf("client config reports disabled: %v", s.clientCfg)
	}
	trigger, _ := s.clientCfg["trigger"].(string)
	if trigger != config.AutocompleteTriggerAuto && trigger != config.AutocompleteTriggerManual {
		return fmt.Errorf("client config trigger = %q", trigger)
	}
	if debounce, _ := s.clientCfg["debounce_ms"].(float64); debounce <= 0 {
		return fmt.Errorf("client config debounce_ms = %v", s.clientCfg["debounce_ms"])
	}
	return nil
}

func initializeAutocompleteScenario(sc *godog.ScenarioContext) {
	s := &autocompleteFeatureState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.close()
		return ctx, nil
	})

	sc.Step(`^a running foxxycode HTTP server with autocomplete enabled$`, s.startServerAutocompleteEnabled)
	sc.Step(`^an editor requests a completion for a caret inside a function$`, s.requestCompletionInsideFunction)
	sc.Step(`^the suggestion comes back as text that can be inserted verbatim$`, s.suggestionIsInsertable)
	sc.Step(`^the model was given the code on both sides of the caret$`, s.modelSawBothSidesOfCaret)
	sc.Step(`^an editor reads the autocomplete client config$`, s.readClientConfig)
	sc.Step(`^the client config reports autocomplete enabled with a trigger and a debounce$`, s.clientConfigDescribesBehaviour)
}

func TestCodeAutocompleteFeature(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "code-autocomplete",
		ScenarioInitializer: initializeAutocompleteScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/code_autocomplete.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("code autocomplete feature suite failed")
	}
}
