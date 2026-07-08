package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type endTurnProvider struct{}

func (endTurnProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: "ok", StopReason: "end_turn"}, nil
}

func (endTurnProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	onChunk(llm.StreamChunk{TextDelta: "ok"})
	return &llm.Response{Content: "ok", StopReason: "end_turn"}, nil
}

func TestIdeEnvNoteEmptyWhenNoState(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	ideenv.Reset()
	if note := ideEnvNote("/ws"); note != "" {
		t.Fatalf("expected empty note, got %q", note)
	}
}

func TestIdeEnvNoteRelativizesToCwd(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	ideenv.Set([]string{"/ws/src/a.go", "/ws/src/b.go", "/other/c.go"}, "/ws/src/a.go")

	note := ideEnvNote("/ws")
	if !strings.HasPrefix(note, "<foxxycode_ide_context>") || !strings.HasSuffix(note, "</foxxycode_ide_context>") {
		t.Fatalf("note not wrapped in tag: %q", note)
	}
	if !strings.Contains(note, "# Active File\nsrc/a.go\n") {
		t.Fatalf("active file not relativized: %q", note)
	}
	if !strings.Contains(note, "src/a.go") || !strings.Contains(note, "src/b.go") {
		t.Fatalf("open tabs missing relative paths: %q", note)
	}
	// A path outside cwd stays absolute (forward slashes).
	if !strings.Contains(note, "/other/c.go") {
		t.Fatalf("out-of-workspace path not preserved: %q", note)
	}
}

func TestIdeEnvNoteNoneWhenOnlyActiveMissing(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	ideenv.Set([]string{"/ws/a.go"}, "")
	note := ideEnvNote("/ws")
	if !strings.Contains(note, "# Active File\n(none)") {
		t.Fatalf("expected (none) active file, got %q", note)
	}
}

func TestRunInjectsIdeContextIntoUserMessage(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	cwd := t.TempDir()
	ideenv.Set([]string{cwd + "/main.go"}, cwd+"/main.go")

	st := &session.State{
		ID:         "sess_ide_ctx",
		CWD:        cwd,
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return endTurnProvider{}, nil }

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "hi"}}); err != nil {
		t.Fatal(err)
	}

	var userMsg string
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleUser {
			userMsg = m.Content
			break
		}
	}
	if !strings.Contains(userMsg, "<foxxycode_ide_context>") || !strings.Contains(userMsg, "main.go") {
		t.Fatalf("user message missing IDE context block: %q", userMsg)
	}
}

func TestIdeEnvNoteCapsTabs(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	many := make([]string, ideEnvMaxTabs+10)
	for i := range many {
		many[i] = "/ws/f" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + ".go"
	}
	ideenv.Set(many, many[0])
	note := ideEnvNote("/ws")
	// Count lines in the Open Tabs section.
	tabsSection := note[strings.Index(note, "# Open Tabs\n")+len("# Open Tabs\n"):]
	tabsSection = strings.TrimSuffix(tabsSection, "\n</foxxycode_ide_context>")
	lines := strings.Count(tabsSection, "\n") + 1
	if lines > ideEnvMaxTabs {
		t.Fatalf("open tabs not capped: %d lines", lines)
	}
}
