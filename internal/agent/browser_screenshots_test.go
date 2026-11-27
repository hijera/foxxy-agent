package agent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

func TestBrowserScreenshotEvictions(t *testing.T) {
	shot := llm.ImagePart{DataURL: "data:image/png;base64,AA=="}
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "user attachment", ImageParts: []llm.ImagePart{shot}},
		{Role: llm.RoleUser, Content: browserVisionNote, ImageParts: []llm.ImagePart{shot}, BrowserScreenshot: true},
		{Role: llm.RoleTool, Content: "clicked submit"},
		{Role: llm.RoleUser, Content: browserVisionNote, ImageParts: []llm.ImagePart{shot}, BrowserScreenshot: true},
	}
	for _, tc := range []struct {
		name string
		mode string
		max  int
		want []int
	}{
		{"keep existing behavior", "", 0, nil},
		{"keep only newest", config.BrowserScreenshotLatest, 0, []int{1}},
		{"below limit", config.BrowserScreenshotReplace, 3, nil},
		{"attachment occupies one slot", config.BrowserScreenshotReplace, 2, []int{1}},
		{"attachments fill budget", config.BrowserScreenshotLatest, 1, []int{1, 3}},
		{"child receives every image", config.BrowserScreenshotSubagent, 0, []int{1, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := browserImageEvictions(messages, config.BrowserScreenshotSettings{Mode: tc.mode, MaxScreenshots: tc.max})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("evictions = %v, want %v", got, tc.want)
			}
		})
	}
}

type screenshotDescriptionProvider struct {
	toolThenDoneProvider
	names []string
}

func (p *screenshotDescriptionProvider) Complete(_ context.Context, messages []llm.Message, defs []llm.ToolDefinition) (*llm.Response, error) {
	if len(messages) != 2 || len(messages[1].ImageParts) != 1 || len(defs) != 0 {
		return nil, fmt.Errorf("expected one screenshot and no tools")
	}
	name := messages[1].ImageParts[0].Name
	p.names = append(p.names, name)
	return &llm.Response{Content: "description of " + name}, nil
}

func TestBrowserScreenshotReplacementDescribesEachEvictedImageOnce(t *testing.T) {
	for _, tc := range []struct {
		mode string
		max  int
		want []string
	}{
		{config.BrowserScreenshotLatest, 0, []string{"first.png", "second.png"}},
		{config.BrowserScreenshotReplace, 2, []string{"first.png"}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := &config.Config{Browser: config.BrowserConfig{
				ScreenshotSettings: &config.BrowserScreenshotSettings{Mode: tc.mode, MaxScreenshots: tc.max},
			}}
			a := NewAgent(cfg, &session.State{ID: "sess_browser_replace", CWD: t.TempDir()}, resumePermissionSender{}, nil)
			provider := &screenshotDescriptionProvider{}
			var messages []llm.Message
			for _, name := range []string{"first.png", "second.png", "third.png"} {
				part := llm.ImagePart{DataURL: "data:image/png;base64,AA==", Name: name}
				messages = a.addBrowserScreenshots(context.Background(), provider, messages, []browserScreenshot{{part: part}})
			}
			a.fitBrowserScreenshots(context.Background(), provider, messages)
			if !reflect.DeepEqual(provider.names, tc.want) {
				t.Fatalf("described images = %v, want %v", provider.names, tc.want)
			}
			for i, name := range tc.want {
				if len(messages[i].ImageParts) != 0 || !strings.Contains(messages[i].Content, "description of "+name) {
					t.Fatalf("message for %s was not replaced by its description", name)
				}
			}
			if latest := messages[len(messages)-1].ImageParts; len(latest) != 1 || latest[0].Name != "third.png" {
				t.Fatalf("latest image = %v", latest)
			}
		})
	}
}

type screenshotChildStub struct {
	state   *session.State
	spec    session.SubagentSpec
	prompt  string
	images  []acp.ImagePartRef
	retired string
}

func (s *screenshotChildStub) CreateSubagentSession(_ context.Context, spec session.SubagentSpec) (*session.State, error) {
	s.spec = spec
	s.state = &session.State{ID: spec.ID}
	return s.state, nil
}

func (*screenshotChildStub) RunSubagentTurn(context.Context, string, []acp.ContentBlock, acp.UpdateSender) (*acp.SessionPromptResult, error) {
	return nil, fmt.Errorf("vision turn required")
}

func (s *screenshotChildStub) RunSubagentVisionTurn(_ context.Context, _ string, prompt []acp.ContentBlock, images []acp.ImagePartRef, _ acp.UpdateSender) (*acp.SessionPromptResult, error) {
	s.prompt = prompt[0].Text
	s.images = images
	s.state.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "The submit button is visible."})
	return &acp.SessionPromptResult{StopReason: acp.StopReasonEndTurn}, nil
}

func (s *screenshotChildStub) RetireSubagentSession(id string) { s.retired = id }

func TestBrowserScreenshotSeparateSessionReturnsOnlyReport(t *testing.T) {
	cfg := &config.Config{Browser: config.BrowserConfig{
		ScreenshotSettings: &config.BrowserScreenshotSettings{Mode: config.BrowserScreenshotSubagent},
	}}
	parent := &session.State{ID: "sess_parent", CWD: t.TempDir(), Mode: session.ModeAgent}
	a := NewAgent(cfg, parent, resumePermissionSender{}, nil)
	child := &screenshotChildStub{}
	a.SetSubagentRuntime(child)
	part := llm.ImagePart{DataURL: "data:image/png;base64,AA==", Name: "page.png"}
	got := a.addBrowserScreenshots(context.Background(), nil, nil, []browserScreenshot{{part: part, prompt: "Is submit visible?"}})
	if len(got) != 1 || len(got[0].ImageParts) != 0 || !strings.Contains(got[0].Content, "The submit button is visible.") {
		t.Fatalf("parent received %#v", got)
	}
	if child.spec.ParentSessionID != parent.ID || child.spec.ID == "" || child.retired != child.spec.ID || len(child.spec.Tools) != 0 || child.spec.ConnectMCP {
		t.Fatalf("unexpected child lifecycle or scope: %#v, retired %q", child.spec, child.retired)
	}
	if len(child.images) != 1 || child.images[0].DataURL != part.DataURL || !strings.Contains(child.prompt, "Is submit visible?") {
		t.Fatalf("child received %d images and prompt %q", len(child.images), child.prompt)
	}
}

type imageLimitProvider struct {
	toolThenDoneProvider
	rejected bool
}

func (p *imageLimitProvider) Stream(ctx context.Context, messages []llm.Message, defs []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	for _, msg := range messages {
		if msg.BrowserScreenshot && len(msg.ImageParts) > 0 {
			p.rejected = true
			return nil, fmt.Errorf("Слишком много вложений одного типа для этой модели")
		}
	}
	return p.toolThenDoneProvider.Stream(ctx, messages, defs, onChunk)
}

func TestBrowserScreenshotLimitReplaysRequestWithoutRerunningTool(t *testing.T) {
	st := &session.State{ID: "sess_shot_limit", CWD: t.TempDir(), Mode: session.ModeAgent}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}
	a := NewAgent(cfg, st, resumePermissionSender{}, nil)
	provider := &imageLimitProvider{toolThenDoneProvider: toolThenDoneProvider{toolName: "fake_browser_shot"}}
	a.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	executions := 0
	a.registry.Register(&tools.Tool{
		Definition: llm.ToolDefinition{Name: "fake_browser_shot", InputSchema: map[string]interface{}{"type": "object"}},
		Execute: func(_ context.Context, _ string, env *tools.Env) (string, error) {
			executions++
			env.AddBrowserScreenshot("data:image/png;base64,AA==", "", "page.png", "")
			return "clicked", nil
		},
	})
	if result, err := a.Run(context.Background(), []acp.ContentBlock{{Type: acp.ContentTypeText, Text: "click the button"}}); err != nil || result != string(acp.StopReasonEndTurn) {
		t.Fatalf("turn result = %q, %v", result, err)
	}
	if !provider.rejected || executions != 1 || provider.calls != 2 {
		t.Fatalf("image rejected=%v, tool executions=%d, accepted model requests=%d", provider.rejected, executions, provider.calls)
	}
}
