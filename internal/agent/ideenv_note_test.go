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
	ideenv.Set([]string{"/ws/src/a.go", "/ws/src/b.go", "/other/c.go"}, "/ws/src/a.go", nil)

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
	ideenv.Set([]string{"/ws/a.go"}, "", nil)
	note := ideEnvNote("/ws")
	if !strings.Contains(note, "# Active File\n(none)") {
		t.Fatalf("expected (none) active file, got %q", note)
	}
}

func TestIdeEnvNoteIncludesSelection(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	ideenv.Set([]string{"/ws/src/a.go"}, "/ws/src/a.go", &ideenv.Selection{
		File: "/ws/src/a.go", StartLine: 21, EndLine: 31, Text: "x := 1\ny := 2",
	})
	note := ideEnvNote("/ws")
	if !strings.Contains(note, "# Selection\nsrc/a.go:21-31\nx := 1\ny := 2\n") {
		t.Fatalf("selection section missing or wrong: %q", note)
	}
}

func TestIdeEnvNoteOmitsSelectionWhenNone(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	ideenv.Set([]string{"/ws/a.go"}, "/ws/a.go", nil)
	if note := ideEnvNote("/ws"); strings.Contains(note, "# Selection") {
		t.Fatalf("unexpected selection section: %q", note)
	}
}

func TestIdeEnvNoteCapsSelectionText(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	long := strings.Repeat("z", ideEnvMaxSelectionBytes+500)
	ideenv.Set(nil, "", &ideenv.Selection{File: "/ws/a.go", StartLine: 1, EndLine: 400, Text: long})
	note := ideEnvNote("/ws")
	if !strings.Contains(note, "# Selection") {
		t.Fatalf("selection section missing: %q", note)
	}
	if strings.Count(note, "z") > ideEnvMaxSelectionBytes {
		t.Fatalf("selection text not capped: %d z's", strings.Count(note, "z"))
	}
}

func TestRunInjectsIdeContextIntoUserMessage(t *testing.T) {
	t.Cleanup(ideenv.Reset)
	cwd := t.TempDir()
	ideenv.Set([]string{cwd + "/main.go"}, cwd+"/main.go", nil)

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
	ideenv.Set(many, many[0], nil)
	note := ideEnvNote("/ws")
	// Count lines in the Open Tabs section.
	tabsSection := note[strings.Index(note, "# Open Tabs\n")+len("# Open Tabs\n"):]
	tabsSection = strings.TrimSuffix(tabsSection, "\n</foxxycode_ide_context>")
	lines := strings.Count(tabsSection, "\n") + 1
	if lines > ideEnvMaxTabs {
		t.Fatalf("open tabs not capped: %d lines", lines)
	}
}
