//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestLocalOllamaBindingPullsMissingExactModelThenConnects(t *testing.T) {
	pulled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/models":
			models := []map[string]string{}
			if pulled {
				models = append(models, map[string]string{"id": "reviewed:1"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": models})
		case "/api/pull":
			pulled = true
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	executor := NewConfigModelExecutor(nil, nil)
	binding := ModelBinding{
		Provider: ProviderIdentity{Scope: "local", Adapter: "ollama"},
		LocalBootstrap: &LocalBootstrap{
			Connect: true, EnsureModel: "pull_if_missing", StorageScope: "app_cache",
		},
	}
	resolved := &config.ResolvedLLM{BaseURL: server.URL + "/v1", Model: "reviewed:1"}
	if err := executor.ensureLocalModel(context.Background(), binding, resolved); err != nil {
		t.Fatal(err)
	}
	if !pulled {
		t.Fatal("expected missing local model to be pulled")
	}
}

func TestNeuralDeepBindingUsesPinnedProviderURL(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{
			Name: "neuraldeep", Type: "neuraldeep", APIKey: "test-key",
		}},
		Models: []config.ModelEntry{{Model: "neuraldeep/gpt-oss-120b"}},
		Agent:  config.Agent{Model: "neuraldeep/gpt-oss-120b"},
	}

	binding, err := BindingForConfiguredModel(cfg, "", "primary")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := binding.Provider.BaseURL, "https://api.neuraldeep.ru/v1"; got != want {
		t.Fatalf("binding base URL = %q, want %q", got, want)
	}
	if _, err := NewConfigModelExecutor(cfg, nil).resolve(*binding); err != nil {
		t.Fatalf("resolve pinned NeuralDeep binding: %v", err)
	}
}
