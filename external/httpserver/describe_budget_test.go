//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

// POST /foxxycode/describe asks for a phrase and a tag line, but a reasoning
// model thinks first. At the old cap of 96 tokens gpt-oss on NeuralDeep spent
// the whole budget on reasoning (finish_reason "length", content null), so the
// route answered with the first words of the text and no tags - two times in
// three once the tag line made the reasoning longer. The budget now leaves the
// model room to answer; a model configured with less keeps its own limit.
func TestDescribeProviderLeavesRoomForReasoning(t *testing.T) {
	var mu sync.Mutex
	var sent map[string]interface{}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = nil
		_ = json.Unmarshal(body, &sent)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","model":"m","choices":[{"index":0,"finish_reason":"stop",`+
			`"message":{"role":"assistant","content":"Nightly Postgres backup\ntags: postgres, backup"}}]}`)
	}))
	defer stub.Close()

	for _, tc := range []struct {
		name     string
		modelMax int
		want     int
	}{
		{"large model limit", 8192, describeMaxTokens},
		{"no model limit", 0, describeMaxTokens},
		{"smaller model limit", 200, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Providers: []config.ProviderConfig{{Name: "stub", Type: "openai", APIBase: stub.URL + "/v1", APIKey: "k"}},
				Models:    []config.ModelEntry{{Model: "stub/m", MaxTokens: tc.modelMax}},
				Agent:     config.Agent{Model: "stub/m"},
			}
			cfg.Agent.ApplyDefaults()
			p, err := defaultProviderFromAgentModel(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Complete(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "backup"}}, nil); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			got, _ := sent["max_tokens"].(float64)
			mu.Unlock()
			if int(got) != tc.want {
				t.Fatalf("max_tokens = %v, want %d", sent["max_tokens"], tc.want)
			}
		})
	}
	if describeMaxTokens < 512 {
		t.Fatalf("describeMaxTokens = %d: a reasoning model needs a few hundred tokens before it answers", describeMaxTokens)
	}
}
