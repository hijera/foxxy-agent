//go:build miniapps

package miniapps

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
