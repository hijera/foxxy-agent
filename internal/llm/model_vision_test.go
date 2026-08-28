package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// neuralDeepCatalogBody is the shape api.neuraldeep.ru actually returns, trimmed to
// the fields that matter here. gpt-oss-20b really does refuse image input, so the
// catalog is the only place the agent can learn that before spending a request.
const neuralDeepCatalogBody = `{"data":[
 {"id":"qwen3.6-35b-a3b","type":"chat","capabilities":{"vision":true,"tools":true},"modalities":{"input":["text","image"],"output":["text"]}},
 {"id":"gpt-oss-20b","type":"chat","capabilities":{"vision":false,"tools":true},"modalities":{"input":["text"],"output":["text"]}},
 {"id":"legacy-modalities-only","type":"chat","modalities":{"input":["text","image"],"output":["text"]}},
 {"id":"plain-openai-model"}
]}`

// TestListModelsReadsVisionCapability makes the advertised capability survive the
// trip to the caller. Without it every discovered model looks text-only, which is
// how a vision-capable model ends up saved with multimodal: false.
func TestListModelsReadsVisionCapability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(neuralDeepCatalogBody))
	}))
	defer srv.Close()

	got, err := ListModels(context.Background(), ProviderInput{Type: "openai", APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	vision := map[string]bool{}
	for _, m := range got {
		vision[m.ID] = m.Vision
	}
	for id, want := range map[string]bool{
		"qwen3.6-35b-a3b": true,
		// modalities.input alone is enough: not every gateway fills capabilities.
		"legacy-modalities-only": true,
		"gpt-oss-20b":            false,
		// A plain OpenAI catalog advertises nothing; absence is not a claim of
		// support, so it stays false and the user can still tick the box.
		"plain-openai-model": false,
	} {
		if vision[id] != want {
			t.Errorf("%s: vision = %v, want %v", id, vision[id], want)
		}
	}
}
