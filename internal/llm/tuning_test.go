package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureServer answers any chat or raw completion with a canned body and hands
// back the decoded request so a test can pin what reached the wire.
func captureServer(t *testing.T, path string, reply string) (*httptest.Server, *map[string]interface{}) {
	t.Helper()
	got := map[string]interface{}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

const chatReply = `{"id":"1","object":"chat.completion","choices":[{"index":0,` +
	`"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`

func TestChatTuningReachesTheWire(t *testing.T) {
	srv, got := captureServer(t, "/v1/chat/completions", chatReply)
	p, err := NewProvider(ProviderInput{
		Type: "openai", Model: "qwen3.6-35b", APIKey: "k", BaseURL: srv.URL + "/v1",
		MaxTokens: 64, Stop: []string{"\n}", "\n\n\n"}, Deterministic: true, NoThinking: true,
		RetryDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	body := *got
	if temp, ok := body["temperature"].(float64); !ok || temp != 0 {
		t.Fatalf("temperature = %v, want an explicit 0", body["temperature"])
	}
	stops, _ := body["stop"].([]interface{})
	if len(stops) != 2 || stops[0] != "\n}" {
		t.Fatalf("stop = %v", body["stop"])
	}
	kw, _ := body["chat_template_kwargs"].(map[string]interface{})
	if think, ok := kw["enable_thinking"].(bool); !ok || think {
		t.Fatalf("chat_template_kwargs = %v, want enable_thinking:false", body["chat_template_kwargs"])
	}
}

// Without the tuning flags nothing new is sent: existing agent traffic must be byte-identical.
func TestChatWithoutTuningSendsNothingExtra(t *testing.T) {
	srv, got := captureServer(t, "/v1/chat/completions", chatReply)
	p, err := NewProvider(ProviderInput{Type: "openai", Model: "qwen3.6-35b", APIKey: "k", BaseURL: srv.URL + "/v1", RetryDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	body := *got
	for _, k := range []string{"temperature", "stop", "chat_template_kwargs"} {
		if _, present := body[k]; present {
			t.Fatalf("%s was sent without being requested: %v", k, body[k])
		}
	}
}

func TestCompleteRawSendsThePromptVerbatim(t *testing.T) {
	srv, got := captureServer(t, "/v1/completions",
		`{"id":"1","object":"text_completion","choices":[{"index":0,"text":"a + b","finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":12,"completion_tokens":3}}`)
	p, err := NewProvider(ProviderInput{
		Type: "openai", Model: "qwen2.5-coder", APIKey: "k", BaseURL: srv.URL + "/v1",
		MaxTokens: 48, Deterministic: true, Stop: []string{"<|endoftext|>"}, RetryDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rc, ok := AsRawCompleter(p)
	if !ok {
		t.Fatal("openai provider must expose RawCompleter through the resilience wrapper")
	}
	prompt := "<|fim_prefix|>return <|fim_suffix|>\n}<|fim_middle|>"
	resp, err := rc.CompleteRaw(context.Background(), prompt)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "a + b" || resp.InputTokens != 12 || resp.OutputTokens != 3 || resp.StopReason != "end_turn" {
		t.Fatalf("response = %+v", resp)
	}
	body := *got
	if body["prompt"] != prompt {
		t.Fatalf("prompt on the wire = %q", body["prompt"])
	}
	if _, chat := body["messages"]; chat {
		t.Fatal("raw completion must not carry chat messages")
	}
	if mt, _ := body["max_tokens"].(float64); int(mt) != 48 {
		t.Fatalf("max_tokens = %v", body["max_tokens"])
	}
	if temp, ok := body["temperature"].(float64); !ok || temp != 0 {
		t.Fatalf("temperature = %v", body["temperature"])
	}
}

func TestAsRawCompleterLooksThroughBlockingWrapper(t *testing.T) {
	p, err := NewProvider(ProviderInput{Type: "openai", Model: "m", APIKey: "k", DisableStream: true, RetryDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := AsRawCompleter(p); !ok {
		t.Fatal("RawCompleter not reachable through blocking + resilient wrappers")
	}
}

func TestAnthropicHasNoRawCompleter(t *testing.T) {
	p, err := NewProvider(ProviderInput{Type: "anthropic", Model: "claude", APIKey: "k", RetryDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := AsRawCompleter(p); ok {
		t.Fatal("anthropic must not claim raw completion support")
	}
}
