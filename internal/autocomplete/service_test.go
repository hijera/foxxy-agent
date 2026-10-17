package autocomplete_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/autocomplete"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type provider struct{ prompt *string }

func (p provider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	panic("autocomplete must stream")
}

func (p provider) Stream(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition, emit func(llm.StreamChunk)) (*llm.Response, error) {
	*p.prompt = messages[len(messages)-1].Content
	emit(llm.StreamChunk{TextDelta: "answer"})
	return &llm.Response{Content: "answer"}, nil
}

func TestServiceRunsWithInjectedDependencies(t *testing.T) {
	on := true
	cfg := &config.Config{
		Providers:    []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:       []config.ModelEntry{{Model: "fake/model"}},
		Autocomplete: config.AutocompleteConfig{Enabled: &on, Mode: "chat"},
	}
	var prompt string
	var service autocomplete.Service
	result, err := service.Complete(context.Background(), cfg,
		autocomplete.Request{Prefix: "return ", Path: "main.go"}, autocomplete.Options{
			ProviderFactory: func(in llm.ProviderInput) (llm.Provider, error) {
				if !in.RetryDisabled || !in.NoThinking {
					t.Fatalf("completion lost its generation policy: %+v", in)
				}
				return provider{&prompt}, nil
			},
			RelatedFiles: func(string, int) []llm.FIMFile {
				return []llm.FIMFile{{Path: "helper.go", Content: "func helper() {}"}}
			},
		})
	if err != nil || result.Completion != "answer" || !strings.Contains(prompt, "helper.go") {
		t.Fatalf("standalone completion: %+v, %v, prompt %q", result, err, prompt)
	}
	if service.Snapshot()["served"] != int64(1) {
		t.Fatal("standalone service lost completion statistics")
	}
}

type cancelledProvider struct{ provider }

func (p cancelledProvider) Stream(ctx context.Context, _ []llm.Message, _ []llm.ToolDefinition, _ func(llm.StreamChunk)) (*llm.Response, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCancellationDoesNotBecomeTimeoutOrProviderFailure(t *testing.T) {
	on := true
	cfg := &config.Config{
		Providers:    []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:       []config.ModelEntry{{Model: "fake/model"}},
		Autocomplete: config.AutocompleteConfig{Enabled: &on, Mode: "chat"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var service autocomplete.Service
	_, err := service.Complete(ctx, cfg, autocomplete.Request{Prefix: "return "}, autocomplete.Options{
		ProviderFactory: func(llm.ProviderInput) (llm.Provider, error) { return cancelledProvider{}, nil },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	stats := service.Snapshot()
	if stats["cancelled"] != int64(1) || stats["timeouts"] != int64(0) || stats["errors"] != int64(0) {
		t.Fatalf("wrong cancellation accounting: %v", stats)
	}
}

func TestDisabledServiceDoesNotConsultHostOrModel(t *testing.T) {
	off := false
	var service autocomplete.Service
	result, err := service.Complete(context.Background(), &config.Config{
		Autocomplete: config.AutocompleteConfig{Enabled: &off},
	}, autocomplete.Request{Prefix: "return "}, autocomplete.Options{
		ProviderFactory: func(llm.ProviderInput) (llm.Provider, error) { t.Fatal("model consulted"); return nil, nil },
		RelatedFiles:    func(string, int) []llm.FIMFile { t.Fatal("host consulted"); return nil },
	})
	if err != nil || result.Enabled {
		t.Fatalf("disabled service: %+v, %v", result, err)
	}
}

type fimProvider struct {
	provider
	rawCalls *int
}

func (p fimProvider) CompleteRaw(context.Context, string) (*llm.Response, error) {
	*p.rawCalls = *p.rawCalls + 1
	return nil, errors.New("raw completion unsupported")
}

func TestFIMFallbackMemoryBelongsToEachService(t *testing.T) {
	on := true
	cfg := &config.Config{
		Providers:    []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:       []config.ModelEntry{{Model: "fake/qwen2.5-coder"}},
		Autocomplete: config.AutocompleteConfig{Enabled: &on, Mode: "auto"},
	}
	var rawCalls int
	var prompt string
	opts := autocomplete.Options{ProviderFactory: func(llm.ProviderInput) (llm.Provider, error) {
		return fimProvider{provider{&prompt}, &rawCalls}, nil
	}}
	request := autocomplete.Request{Prefix: "return "}
	var first, second autocomplete.Service
	for _, service := range []*autocomplete.Service{&first, &first, &second} {
		result, err := service.Complete(context.Background(), cfg, request, opts)
		if err != nil || result.Mode != "chat" || result.Completion != "answer" {
			t.Fatalf("fallback result: %+v, %v", result, err)
		}
	}
	if rawCalls != 2 || first.Snapshot()["fim_fallback"] != int64(1) || second.Snapshot()["fim_fallback"] != int64(1) {
		t.Fatalf("fallback leaked between services: raw=%d, first=%v, second=%v", rawCalls, first.Snapshot(), second.Snapshot())
	}
}
