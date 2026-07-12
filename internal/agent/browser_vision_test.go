package agent

import (
	"context"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// toolThenDoneProvider returns one tool call on the first turn, then ends the turn.
// It records the messages seen on its final call so tests can assert what the model saw.
type toolThenDoneProvider struct {
	calls    int
	toolName string
	lastSeen []llm.Message
}

func (p *toolThenDoneProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *toolThenDoneProvider) Stream(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	p.lastSeen = append([]llm.Message(nil), messages...)
	if p.calls == 1 {
		return &llm.Response{
			ToolCalls:  []llm.ToolCall{{ID: "c1", Name: p.toolName, InputJSON: "{}"}},
			StopReason: "tool_use",
		}, nil
	}
	onChunk(llm.StreamChunk{TextDelta: "done"})
	return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
}

// TestBrowserToolScreenshotInjectedAsUserVision verifies that when a tool calls
// env.AddToolImage (as the browser tools do after a screenshot), the ReAct loop
// injects the image into history as a user-role vision message the model can see.
func TestBrowserToolScreenshotInjectedAsUserVision(t *testing.T) {
	st := &session.State{
		ID:         "sess_browser_vision",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	provider := &toolThenDoneProvider{toolName: "fake_browser_shot"}
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}

	// A stand-in for a browser tool: hands a screenshot to the agent via AddToolImage.
	const dataURL = "data:image/png;base64,iVBORw0KGgo="
	ag.registry.Register(&tools.Tool{
		Definition: llm.ToolDefinition{
			Name:        "fake_browser_shot",
			Description: "test",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		RequiresPermission: false,
		Execute: func(_ context.Context, _ string, env *tools.Env) (string, error) {
			if env.AddToolImage != nil {
				env.AddToolImage(dataURL, "/tmp/shot.png", "shot.png")
			}
			return "navigated", nil
		},
	})

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "open a page"}}); err != nil {
		t.Fatal(err)
	}

	// The screenshot must reach the model (final provider call), injected as a
	// user-role vision block after the tool round.
	var vision *llm.Message
	for i := range provider.lastSeen {
		m := provider.lastSeen[i]
		if m.Role == llm.RoleUser && len(m.ImageParts) > 0 {
			vision = &provider.lastSeen[i]
		}
	}
	if vision == nil {
		t.Fatal("no user-role vision message with image parts was sent to the model after the browser tool call")
	}
	if vision.ImageParts[0].DataURL != dataURL {
		t.Errorf("injected image DataURL = %q, want %q", vision.ImageParts[0].DataURL, dataURL)
	}
	if vision.Content != browserVisionNote {
		t.Errorf("vision message content = %q, want browserVisionNote", vision.Content)
	}

	// It must NOT be persisted to the transcript (no spurious user bubble, no re-send).
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleUser && len(m.ImageParts) > 0 {
			t.Error("browser vision message must not be persisted to session history")
		}
	}
}
