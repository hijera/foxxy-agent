//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// TestEnhancePromptWireRequest drives the real provider stack (no
// makeLLMFromYAML stub) against a fake OpenAI endpoint, pinning what actually
// reaches the wire: the model the session selected, and that model's own
// max_tokens rather than the 96-token cap defaultProviderFromAgentModel applies
// for describe-style titles.
func TestEnhancePromptWireRequest(t *testing.T) {
	var gotBody map[string]interface{}
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","object":"chat.completion","choices":[{"index":0,` +
			`"message":{"role":"assistant","content":"Refactor the memory endpoint and add tests."},` +
			`"finish_reason":"stop"}]}`))
	}))
	defer llmSrv.Close()

	root := t.TempDir()
	cfg := &config.Config{
		Paths: config.Paths{Home: root, CWD: root},
		Providers: []config.ProviderConfig{
			{Name: "openai", Type: "openai", APIBase: llmSrv.URL, APIKey: "sk-test"},
		},
		Models: []config.ModelEntry{
			{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2},
			{Model: enhanceAltModel, MaxTokens: 4096, Temperature: 0.2},
		},
		Agent: config.Agent{Model: "openai/gpt-4o"},
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, nil)
	srv := New(cfg, mgr, slog.Default(), root)
	t.Cleanup(srv.Drain)

	sn, err := mgr.HandleSessionNew(context.Background(), acp.SessionNewParams{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	st := mgr.SessionByID(sn.SessionID)
	if st == nil {
		t.Fatal("session not found")
	}
	if err := applySessionYAMLModel(cfg, st, enhanceAltModel); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, b := enhancePost(t, ts.URL, sn.SessionID, `{"text":"fix memory thing"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", res.StatusCode, string(b))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "Refactor the memory endpoint") {
		t.Fatalf("unexpected enhanced text %q", out.Text)
	}
	if gotBody == nil {
		t.Fatal("provider was never called")
	}
	// The session picked gpt-4o-mini; agent.model is gpt-4o. Both facts are
	// independent, so report them separately.
	if got := gotBody["model"]; got != "gpt-4o-mini" {
		t.Errorf("wire model: want gpt-4o-mini (session pick) got %v", got)
	}
	// 96 here would mean the rewrite is being truncated to a title-sized reply.
	if got := gotBody["max_tokens"]; got != float64(4096) {
		t.Errorf("wire max_tokens: want 4096 (model's own) got %v", got)
	}
}
