//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

type miniAppAssistantTestProvider struct {
	content  string
	messages []llm.Message
}

func (p *miniAppAssistantTestProvider) Complete(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	p.messages = append([]llm.Message(nil), messages...)
	return &llm.Response{Content: p.content}, nil
}

func (*miniAppAssistantTestProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, _ func(llm.StreamChunk)) (*llm.Response, error) {
	return nil, nil
}

func TestAssistDraftUsesConversationAndReturnsValidatedDraft(t *testing.T) {
	app := verificationTestApp()
	app.Revision = "rev-1"
	app.Inputs = append(app.Inputs, Input{ID: "project", Type: "string", Title: "Project", Required: true, UI: InputUI{Control: "text"}})
	rawDraft, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	provider := &miniAppAssistantTestProvider{
		content: `{"reply":"Добавил обязательное поле проекта.","changes":["Добавлено поле project"],"draft":` + string(rawDraft) + `}`,
	}
	executor := NewProviderModelExecutor(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model"}},
		Agent:     config.Agent{Model: "fake/model"},
	})
	executor.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return provider, nil })

	result, err := executor.AssistDraft(context.Background(), DraftAssistantRequest{
		Draft:   app,
		History: []DraftAssistantMessage{{Role: "user", Content: "Сначала добавь описание"}, {Role: "assistant", Content: "Готово"}},
		Prompt:  "Добавь обязательное поле проекта",
	})
	if err != nil {
		t.Fatalf("AssistDraft() error = %v", err)
	}
	if result.Reply == "" || len(result.Changes) != 1 || len(result.Draft.Inputs) != 2 {
		t.Fatalf("assistant result = %+v", result)
	}
	if len(provider.messages) != 4 || provider.messages[1].Content != "Сначала добавь описание" || provider.messages[2].Role != llm.RoleAssistant {
		t.Fatalf("provider messages = %+v", provider.messages)
	}
	if !strings.Contains(provider.messages[0].Content, "project") || !strings.Contains(provider.messages[3].Content, "обязательное поле") {
		t.Fatalf("provider prompt did not contain draft or request: %+v", provider.messages)
	}
}

func assistantTestExecutor(provider llm.Provider) *ProviderModelExecutor {
	executor := NewProviderModelExecutor(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model"}},
		Agent:     config.Agent{Model: "fake/model"},
	})
	executor.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return provider, nil })
	return executor
}

// History is capped by length, and a Go string slices by byte, so a cut inside
// a multi-byte rune used to hand the provider broken UTF-8. Cyrillic is two
// bytes per rune, which puts the boundary mid-rune for an odd-length prefix.
func TestAssistDraftTruncatesHistoryOnRuneBoundaries(t *testing.T) {
	app := verificationTestApp()
	app.Revision = "rev-1"
	rawDraft, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	provider := &miniAppAssistantTestProvider{
		content: `{"reply":"ok","draft":` + string(rawDraft) + `}`,
	}
	long := strings.Repeat("я", maxAssistantMessageLength)
	if _, err := assistantTestExecutor(provider).AssistDraft(context.Background(), DraftAssistantRequest{
		Draft:   app,
		History: []DraftAssistantMessage{{Role: "user", Content: long}},
		Prompt:  "оставь как есть",
	}); err != nil {
		t.Fatalf("AssistDraft() error = %v", err)
	}
	if len(provider.messages) != 3 {
		t.Fatalf("provider messages = %d, want 3", len(provider.messages))
	}
	sent := provider.messages[1].Content
	if !utf8.ValidString(sent) {
		t.Fatalf("history entry was cut mid-rune: %q", sent[len(sent)-8:])
	}
	if utf8.RuneCountInString(sent) > maxAssistantMessageLength {
		t.Fatalf("history entry kept %d runes, want at most %d", utf8.RuneCountInString(sent), maxAssistantMessageLength)
	}
}

// The prompt is operator input on the same request; leaving it uncapped let a
// single editor message push an arbitrary payload at the provider.
func TestAssistDraftRejectsAnOverlongPrompt(t *testing.T) {
	app := verificationTestApp()
	app.Revision = "rev-1"
	provider := &miniAppAssistantTestProvider{content: `{"reply":"ok"}`}
	_, err := assistantTestExecutor(provider).AssistDraft(context.Background(), DraftAssistantRequest{
		Draft:  app,
		Prompt: strings.Repeat("a", maxAssistantPromptLength+1),
	})
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("overlong prompt error = %v", err)
	}
	if len(provider.messages) != 0 {
		t.Fatal("an overlong prompt still reached the provider")
	}
}

// A draft whose inputs map happens to carry an input identified as "type" must
// not be mistaken for a single inline input object.
func TestUnmarshalAssistantResponseKeepsInputsNamedType(t *testing.T) {
	raw := `{"reply":"ok","draft":{"inputs":{
		"type":{"type":"string","title":"Kind","ui":{"control":"text"}},
		"path":{"type":"string","title":"Path","ui":{"control":"text"}}
	}}}`
	var result DraftAssistantResponse
	if err := unmarshalAssistantResponse(raw, &result); err != nil {
		t.Fatalf("unmarshalAssistantResponse() error = %v", err)
	}
	if len(result.Draft.Inputs) != 2 {
		t.Fatalf("inputs = %+v, want the two keyed entries", result.Draft.Inputs)
	}
	ids := []string{result.Draft.Inputs[0].ID, result.Draft.Inputs[1].ID}
	if ids[0] != "path" || ids[1] != "type" {
		t.Fatalf("input ids = %v, want [path type]", ids)
	}
}

func TestAssistDraftRejectsIdentityChanges(t *testing.T) {
	app := verificationTestApp()
	changed := app
	changed.ID = "other-app"
	rawDraft, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	provider := &miniAppAssistantTestProvider{content: `{"reply":"changed","draft":` + string(rawDraft) + `}`}
	executor := NewProviderModelExecutor(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model"}},
		Agent:     config.Agent{Model: "fake/model"},
	})
	executor.SetProviderFactory(func(llm.ProviderInput) (llm.Provider, error) { return provider, nil })

	if _, err := executor.AssistDraft(context.Background(), DraftAssistantRequest{Draft: app, Prompt: "Переименуй приложение"}); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("identity change error = %v, want id rejection", err)
	}
}

func TestUnmarshalAssistantResponseNormalizesObjectInputs(t *testing.T) {
	app := verificationTestApp()
	rawDraft, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	var draftObject map[string]json.RawMessage
	if err := json.Unmarshal(rawDraft, &draftObject); err != nil {
		t.Fatal(err)
	}
	draftObject["inputs"] = json.RawMessage(`{"file_name":{"type":"string","title":"Имя файла","required":true,"ui":{"control":"text"}}}`)
	normalizedDraft, err := json.Marshal(draftObject)
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(map[string]any{
		"reply": "Добавил поле имени файла.",
		"draft": json.RawMessage(normalizedDraft),
	})
	if err != nil {
		t.Fatal(err)
	}

	var result DraftAssistantResponse
	if err := unmarshalAssistantResponse(string(response), &result); err != nil {
		t.Fatalf("unmarshalAssistantResponse() error = %v", err)
	}
	if len(result.Draft.Inputs) != 1 {
		t.Fatalf("normalized inputs = %+v, want one input", result.Draft.Inputs)
	}
	if result.Draft.Inputs[0].ID != "file_name" || result.Draft.Inputs[0].Title != "Имя файла" {
		t.Fatalf("normalized input = %+v", result.Draft.Inputs[0])
	}
}
