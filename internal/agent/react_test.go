package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/hooks"
	"github.com/hijera/foxxycode-agent/internal/hooks/hooktest"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mcp"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/skills"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	toolweb "github.com/hijera/foxxycode-agent/internal/tools/web"
	"github.com/hijera/foxxycode-agent/internal/tools"
	"github.com/hijera/foxxycode-agent/internal/tools/todo"
)

// --- Shared test doubles ---------------------------------------------------

type resumePermissionSender struct{}

func (resumePermissionSender) SendSessionUpdate(string, interface{}) error { return nil }

func (resumePermissionSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow", OptionID: "allow"}, nil
}

func (resumePermissionSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

// The silent-start guard is configuration, not a constant: it bounds how long a
// streamed call may produce nothing. The default has to clear a real prefill on a
// large context - 30s used to cut healthy reasoning turns once a big tool result
// entered the prompt - while an explicit 0 turns the guard off entirely.
func TestFirstTokenTimeoutBoundsSilentProviderStartup(t *testing.T) {
	var agentCfg config.Agent
	if got := agentCfg.EffectiveLLMFirstTokenTimeout(); got != 90*time.Second {
		t.Fatalf("default first-token timeout = %v, want 90s", got)
	}

	off := 0
	agentCfg.LLMFirstTokenTimeoutMS = &off
	if got := agentCfg.EffectiveLLMFirstTokenTimeout(); got != 0 {
		t.Fatalf("explicit 0 = %v, want the guard disabled", got)
	}

	custom := 180000
	agentCfg.LLMFirstTokenTimeoutMS = &custom
	if got := agentCfg.EffectiveLLMFirstTokenTimeout(); got != 180*time.Second {
		t.Fatalf("explicit 180000 = %v, want 180s", got)
	}
}

type resumePermissionProvider struct {
	t    *testing.T
	seen []llm.Message
}

func (p *resumePermissionProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	p.t.Fatal("Complete must not be used by ResumeAfterPermission")
	return nil, nil
}

func (p *resumePermissionProvider) Stream(_ context.Context, messages []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.seen = append([]llm.Message(nil), messages...)
	onChunk(llm.StreamChunk{TextDelta: "continued"})
	return &llm.Response{Content: "continued", StopReason: "end_turn"}, nil
}

// emptyThenAnswerProvider mimics a gpt-oss / harmony endpoint that ends its first turn
// with only internal reasoning (a tool call that leaked into the reasoning channel) and
// an empty content / empty tool_calls response, then answers normally when re-prompted.
// Without a continuation nudge the ReAct loop would dead-end the turn on a lone
// "thinking" bubble, leaving the user with no visible answer.
type emptyThenAnswerProvider struct {
	calls int
}

func (p *emptyThenAnswerProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *emptyThenAnswerProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	if p.calls == 1 {
		// Reasoning only, with the tool call leaked into the analysis channel as text;
		// no final content and no structured tool_calls.
		onChunk(llm.StreamChunk{ReasoningDelta: `Let's search config for moonshot.{"path":"/cfg","pattern":"moonshot","max_results":20}`})
		return &llm.Response{Content: "", StopReason: "end_turn"}, nil
	}
	onChunk(llm.StreamChunk{TextDelta: "Here is the real answer."})
	return &llm.Response{Content: "Here is the real answer.", StopReason: "end_turn"}, nil
}

// --- react.go: content blocks, context files, tool kind, command, memory ---

func TestContentBlocksToText_textAndResource(t *testing.T) {
	blocks := []acp.ContentBlock{
		{Type: "text", Text: "hello"},
		{Type: "resource", Resource: &acp.Resource{URI: "file:///a/b.go", Text: "pkg main"}},
	}
	got := contentBlocksToText(blocks)
	if !strings.Contains(got, `<foxxycode_attachment path="`) ||
		!strings.Contains(got, `name="b.go"`) ||
		!strings.Contains(got, "<![CDATA[") ||
		!strings.Contains(got, "pkg main") ||
		!strings.Contains(got, "]]>") {
		t.Fatalf("unexpected XML bundle: %s", got)
	}
}

func TestContentBlocksToText_lineRangeFragment(t *testing.T) {
	blocks := []acp.ContentBlock{
		{Type: "resource", Resource: &acp.Resource{URI: "Dockerfile#L21-31", Text: "FROM x"}},
	}
	got := contentBlocksToText(blocks)
	if !strings.Contains(got, `path="Dockerfile"`) ||
		!strings.Contains(got, `name="Dockerfile"`) ||
		!strings.Contains(got, `lines="21-31"`) ||
		!strings.Contains(got, "FROM x") {
		t.Fatalf("unexpected XML bundle: %s", got)
	}
	if strings.Contains(got, "#L21-31") {
		t.Fatalf("fragment leaked into attributes: %s", got)
	}
}

func TestExtractContextFiles_fileURI(t *testing.T) {
	blocks := []acp.ContentBlock{
		{Type: "resource", Resource: &acp.Resource{URI: "file:///tmp/x.txt", Text: "x"}},
		{Type: "resource", Resource: &acp.Resource{URI: "https://example.com/z", Text: ""}},
	}
	got := extractContextFiles(blocks)
	if len(got) != 1 || got[0] != "/tmp/x.txt" {
		t.Fatalf("got %#v", got)
	}
}

func TestToolKind(t *testing.T) {
	cases := []struct {
		name, want string
	}{
		{"read", "read"},
		{"glob", "read"},
		{"grep", "read"},
		{"write", "write"},
		{"apply_patch", "write"},
		{"run_command", "run_command"},
		{"mkdir", "write"},
		{"mcp_server__tool", "other"},
	}
	for _, tc := range cases {
		if g := toolKind(tc.name); g != tc.want {
			t.Errorf("toolKind(%q) = %q, want %q", tc.name, g, tc.want)
		}
	}
}

func TestMCPToolDefinitionsAppliesBothFilters(t *testing.T) {
	// The configured enable/disable filter and the fork's per-mode annotation
	// filter both have to apply. Ask only gets read-only MCP tools, so a helper
	// that dropped the mode gate would silently hand Ask a writing tool.
	newAgent := func() *Agent {
		st := &session.State{
			ID:   "sess_mcp_defs",
			CWD:  t.TempDir(),
			Mode: session.ModeAgent,
			MCPClients: []*mcp.Client{
				mcp.NewStaticClient("srv", []mcp.ToolInfo{
					{Name: "echo", ReadOnly: true},
					{Name: "write"},
					{Name: "secret", ReadOnly: true},
				}),
				mcp.NewStaticClient("other", []mcp.ToolInfo{{Name: "echo", ReadOnly: true}}),
			},
			MCPFilterFactory: func() func(server, tool string) bool {
				return func(server, tool string) bool {
					return server != "srv" || tool != "secret"
				}
			},
		}
		return NewAgent(&config.Config{}, st, resumePermissionSender{}, nil)
	}

	names := func(defs []llm.ToolDefinition) []string {
		out := make([]string, 0, len(defs))
		for _, d := range defs {
			out = append(out, d.Name)
		}
		return out
	}

	got := names(newAgent().mcpToolDefinitions())
	want := []string{"srv__echo", "srv__write", "other__echo"}
	if !slices.Equal(got, want) {
		t.Fatalf("agent mode defs = %v, want %v", got, want)
	}

	// Ask never receives MCP definitions at all: the mode gate sits in front
	// of mcpToolDefinitions, so the whole family disappears from the prompt.
	if defs := newAgent().currentToolDefinitions("ask"); slices.ContainsFunc(defs, func(d llm.ToolDefinition) bool {
		return strings.Contains(d.Name, "__")
	}) {
		t.Fatalf("ask mode must not offer MCP tools, got %v", names(defs))
	}
}

func TestCallMCPToolDisabledGuard(t *testing.T) {
	st := &session.State{
		ID:         "sess_mcp_guard",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		MCPClients: []*mcp.Client{mcp.NewStaticClient("srv", []mcp.ToolInfo{{Name: "echo"}})},
		MCPFilterFactory: func() func(server, tool string) bool {
			return func(server, tool string) bool { return false }
		},
	}
	ag := NewAgent(&config.Config{}, st, resumePermissionSender{}, nil)
	if _, err := ag.callMCPTool(context.Background(), "srv", "echo", "{}"); err == nil {
		t.Fatal("disabled MCP tool must be rejected at dispatch")
	}
}

func TestExtractCommand(t *testing.T) {
	if g := extractCommand(`{"command":"ls -la"}`); g != "ls -la" {
		t.Fatalf("got %q", g)
	}
	if g := extractCommand(`{`); g != "" {
		t.Fatalf("invalid json: got %q", g)
	}
}

func TestFormatMergedMemory(t *testing.T) {
	if g := formatMergedMemory("", "facts"); g != "facts" {
		t.Fatalf("got %q", g)
	}
	if g := formatMergedMemory("note", ""); g != "Session notes:\nnote" {
		t.Fatalf("got %q", g)
	}
	want := "facts\n\nSession notes:\nnote"
	if g := formatMergedMemory("note", "facts"); g != want {
		t.Fatalf("got %q want %q", g, want)
	}
}

// --- react.go: ReAct loop empty-turn recovery ------------------------------

func TestRunReActLoopRecoversFromEmptyAssistantTurn(t *testing.T) {
	st := &session.State{
		ID:         "sess_empty_turn",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	provider := &emptyThenAnswerProvider{}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "do the thing"}})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop reason = %q, want end_turn", stop)
	}
	if provider.calls < 2 {
		t.Fatalf("provider called %d times; expected the loop to re-prompt after an empty (reasoning-only) turn", provider.calls)
	}
	msgs := st.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || last.Content != "Here is the real answer." {
		t.Fatalf("conversation dead-ended on a thinking-only turn: last message = %+v", last)
	}
}

// --- resume_permission.go --------------------------------------------------

func TestResumeAfterPermissionRejectContinuesWithoutExecutingTool(t *testing.T) {
	sessionDir := t.TempDir()
	st := &session.State{
		ID:         "sess_resume_reject",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: sessionDir,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "run blocked command then continue"},
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID:        "call_blocked",
					Name:      "run_command",
					InputJSON: `{"command":"printf SHOULD_NOT_RUN"}`,
				}},
			},
		},
	}
	provider := &resumePermissionProvider{t: t}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}

	stop, err := ag.ResumeAfterPermission(context.Background(), "call_blocked", &acp.PermissionResult{
		Outcome:  "cancelled",
		OptionID: "reject",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop reason %q", stop)
	}
	var toolMsg *llm.Message
	for i := range st.GetMessages() {
		m := st.GetMessages()[i]
		if m.Role == llm.RoleTool && m.ToolCallID == "call_blocked" {
			toolMsg = &m
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("missing resumed tool result message")
		return
	}
	if strings.Contains(toolMsg.Content, "SHOULD_NOT_RUN") {
		t.Fatalf("rejected permission executed the tool: %q", toolMsg.Content)
	}
	if toolMsg.Content != "permission denied by user" {
		t.Fatalf("tool result %q", toolMsg.Content)
	}
	if len(provider.seen) == 0 {
		t.Fatal("provider was not called to continue after rejected permission")
	}
	last := lastHistoryMessage(provider.seen)
	if last.Role != llm.RoleTool || last.ToolCallID != "call_blocked" || last.Content != "permission denied by user" {
		t.Fatalf("provider did not receive denied tool result as latest message: %+v", last)
	}
	if got := st.GetMessages()[len(st.GetMessages())-1]; got.Role != llm.RoleAssistant || got.Content != "continued" {
		t.Fatalf("missing continuation assistant message: %+v", got)
	}
}

// lastHistoryMessage is the newest message of a request that is part of the
// replayed conversation: the turn context block trails it and belongs to no
// transcript (turn_context.go).
func lastHistoryMessage(msgs []llm.Message) llm.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.Contains(msgs[i].Content, turnContextOpenTag) {
			continue
		}
		return msgs[i]
	}
	return llm.Message{}
}

// --- system_prompt.go: context breakdown -----------------------------------

func TestComputeContextBreakdownSystemPromptNonZero(t *testing.T) {
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	st := &session.State{ID: "t", CWD: t.TempDir(), Mode: session.ModeAgent}
	a := NewAgent(cfg, st, nil, nil)
	toolsMD := "## Tools\n\ntool_a: does things"
	_ = toolsMD
	_ = a.buildSystemPrompt("agent", nil, []llm.ToolDefinition{{Name: "tool_a", Description: "does things"}}, "", nil)
	b := st.GetLastContextBreakdown()
	if b == nil {
		t.Fatal("expected breakdown")
		return
	}
	if b.SystemPrompt <= 0 {
		t.Fatalf("expected system prompt tokens > 0, got %+v", b)
	}
	if b.ToolDefinitions <= 0 {
		t.Fatalf("expected tool definition tokens > 0, got %+v", b)
	}
	// Sanity: system includes agent.md body text.
	if b.SystemPrompt < 100 {
		t.Fatalf("system prompt estimate too small: %d", b.SystemPrompt)
	}
}

func TestBuildSystemPromptIncludesRuntimeEnvironment(t *testing.T) {
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	st := &session.State{ID: "t", CWD: t.TempDir(), Mode: session.ModeAgent}
	a := NewAgent(cfg, st, nil, nil)
	a.environment = platform.Environment{
		OS:    "windows",
		Arch:  "amd64",
		Shell: platform.Shell{Kind: platform.ShellPwsh, Path: "pwsh"},
	}

	prompt := a.buildSystemPrompt("agent", nil, nil, "", nil)
	for _, want := range []string{"<os>windows</os>", "<arch>amd64</arch>", "<shell>pwsh</shell>"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt does not contain %q", want)
		}
	}
}

func TestComputeContextBreakdownSubtractsParts(t *testing.T) {
	full := strings.Repeat("x", 400) + "\n\n" + strings.Repeat("y", 200)
	skillsText := strings.Repeat("s", 100)
	toolsText := strings.Repeat("t", 80)
	rules := strings.Repeat("r", 40)
	b := computeContextBreakdown(full, skillsText, toolsText, rules, nil, nil)
	if b.SystemPrompt <= 0 {
		t.Fatalf("system tokens: %d", b.SystemPrompt)
	}
	if b.Skills != session.EstimateTokens(skillsText) {
		t.Fatalf("skills: got %d", b.Skills)
	}
}

// --- system_prompt.go: rules block -----------------------------------------

func TestBuildSystemPromptIncludesRulesBlock(t *testing.T) {
	tmp := t.TempDir()
	rulePath := filepath.Join(tmp, ".foxxycode", "rules")
	if err := os.MkdirAll(rulePath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nalwaysApply: true\nglobs: ['**/*.go']\n---\nRULE_GLOB_TOKEN:xyz\n"
	if err := os.WriteFile(filepath.Join(rulePath, "go.mdc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.ReplaceRulesCatalog(session.DiscoverRules(&config.Config{}, tmp))
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	a := NewAgent(cfg, st, nil, nil)
	prompt := a.buildSystemPrompt("agent", nil, nil, "", []string{filepath.Join(tmp, "main.go")})
	if !strings.Contains(prompt, "RULE_GLOB_TOKEN") {
		t.Fatal("expected rule token in prompt")
	}
	if strings.Contains(prompt, "## Active Skills") {
		t.Fatal("rule token should be under Rules not Skills heading")
	}
}

func TestBuildSystemPromptMentionOnlyRule(t *testing.T) {
	tmp := t.TempDir()
	rulePath := filepath.Join(tmp, ".foxxycode", "rules")
	if err := os.MkdirAll(rulePath, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nalwaysApply: false\ndescription: mention only\n---\nRULE_MENTION_ONLY:secret\n"
	if err := os.WriteFile(filepath.Join(rulePath, "mention_demo.mdc"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.ReplaceRulesCatalog(session.DiscoverRules(&config.Config{}, tmp))
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	a := NewAgent(cfg, st, nil, nil)
	without := a.buildSystemPrompt("agent", nil, nil, "hello", nil)
	if strings.Contains(without, "RULE_MENTION_ONLY") {
		t.Fatal("mention-only rule must not appear without @mention")
	}
	with := a.buildSystemPrompt("agent", nil, nil, "please @mention_demo now", nil)
	if !strings.Contains(with, "RULE_MENTION_ONLY") {
		t.Fatal("expected mention-only rule body with @mention_demo")
	}
}

func TestBuildSystemPromptProjectDocsInRules(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("AGENTS_DOC_TOKEN"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "DESIGN.md"), []byte("DESIGN_DOC_TOKEN"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.ReplaceRulesCatalog(session.DiscoverRules(&config.Config{}, tmp))
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	a := NewAgent(cfg, st, nil, nil)
	prompt := a.buildSystemPrompt("agent", nil, nil, "", nil)
	if !strings.Contains(prompt, "AGENTS_DOC_TOKEN") || !strings.Contains(prompt, "DESIGN_DOC_TOKEN") {
		t.Fatal("expected project docs in rules block")
	}
	agentsIdx := strings.Index(prompt, "AGENTS_DOC_TOKEN")
	designIdx := strings.Index(prompt, "DESIGN_DOC_TOKEN")
	if agentsIdx < 0 || designIdx < 0 || agentsIdx > designIdx {
		t.Fatal("expected AGENTS.md before DESIGN.md in prompt")
	}
}

// The root AGENTS.md is both a project doc of the rules block and the default
// instructions.files entry. Whatever that list says, it and DESIGN.md reach the
// model once: a second copy costs the file's full size on every request.
func TestBuildSystemPromptRootAgentsMDOnce(t *testing.T) {
	tmp := t.TempDir()
	for name, body := range map[string]string{
		"AGENTS.md":       "ROOT_AGENTS_ONCE_TOKEN",
		"DESIGN.md":       "DESIGN_ONCE_TOKEN",
		"CONTRIBUTING.md": "EXTRA_INSTRUCTION_TOKEN",
	} {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name      string
		files     []string
		wantExtra int
	}{
		{name: "default instruction files", files: nil},
		{name: "explicit empty list", files: []string{}},
		{name: "AGENTS.md listed with another file", files: []string{"AGENTS.md", "CONTRIBUTING.md"}, wantExtra: 1},
		{name: "only another file", files: []string{"CONTRIBUTING.md"}, wantExtra: 1},
		{name: "DESIGN.md listed", files: []string{"DESIGN.md"}},
		{name: "AGENTS.md spelled with a dot segment", files: []string{"./AGENTS.md"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Agent.ApplyDefaults()
			cfg.Prompts.ApplyDefaults()
			cfg.Instructions.Files = tc.files
			// What loading a config does: an empty list becomes ["AGENTS.md"].
			cfg.Instructions.ApplyDefaults()
			st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
			st.ReplaceRulesCatalog(session.DiscoverRules(cfg, tmp))
			a := NewAgent(cfg, st, nil, nil)
			for _, mode := range []string{"agent", "plan", "docs", "ask", "debug"} {
				prompt := a.buildSystemPrompt(mode, nil, nil, "", nil)
				for token, want := range map[string]int{
					"ROOT_AGENTS_ONCE_TOKEN":  1,
					"DESIGN_ONCE_TOKEN":       1,
					"EXTRA_INSTRUCTION_TOKEN": tc.wantExtra,
				} {
					if got := strings.Count(prompt, token); got != want {
						t.Errorf("%s mode: %s appears %d times, want %d", mode, token, got, want)
					}
				}
			}
		})
	}
}

// The context popover reads this breakdown. The root AGENTS.md belongs to its
// rules segment, once: the instruction files must not add a second copy to the
// system prompt segment.
func TestContextBreakdownCountsRootAgentsMDOnceUnderRules(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Instructions.ApplyDefaults()
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	a := NewAgent(cfg, st, nil, nil)
	breakdown := func() session.ContextBreakdown {
		t.Helper()
		_ = a.buildSystemPrompt("agent", nil, nil, "", nil)
		b := st.GetLastContextBreakdown()
		if b == nil {
			t.Fatal("expected a context breakdown after building the system prompt")
			return session.ContextBreakdown{}
		}
		return *b
	}

	before := breakdown()
	body := strings.TrimSpace(strings.Repeat("root agents guidance line\n", 1600))
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	after := breakdown()

	bodyTok := session.EstimateTokens(body)
	if grew := after.Rules - before.Rules; grew < bodyTok {
		t.Fatalf("rules segment grew by %d tokens, want at least the %d-token AGENTS.md", grew, bodyTok)
	}
	if grew := after.SystemPrompt - before.SystemPrompt; grew > bodyTok/10 {
		t.Fatalf("system prompt segment grew by %d tokens for a %d-token AGENTS.md: the file is counted twice", grew, bodyTok)
	}
}

// --- system_prompt.go: per-provider prompt selection -----------------------

func TestBuildSystemPromptPerProviderSelectsFamily(t *testing.T) {
	newAgentFor := func(perProviderEnabled bool) *Agent {
		st := &session.State{ID: "t", CWD: t.TempDir(), Mode: session.ModeAgent}
		cfg := &config.Config{
			Providers: []config.ProviderConfig{{Name: "anthropic", Type: "anthropic", APIKey: "test"}},
			Models:    []config.ModelEntry{{Model: "anthropic/claude-x", MaxTokens: 100}},
			Agent:     config.Agent{Model: "anthropic/claude-x"},
		}
		cfg.Agent.ApplyDefaults()
		cfg.Prompts.ApplyDefaults()
		cfg.Prompts.PerProvider.Enabled = &perProviderEnabled
		return NewAgent(cfg, st, nil, nil)
	}

	// Enabled: the anthropic family variant (agent.anthropic.md) carries a
	// "Model-family notes" section that the shared agent.md does not.
	on := newAgentFor(true).buildSystemPrompt("agent", nil, nil, "", nil)
	if !strings.Contains(on, "Model-family notes") {
		t.Fatal("expected anthropic family prompt when per-provider prompts are enabled")
	}

	// Disabled: falls back to the shared base prompt without family notes.
	off := newAgentFor(false).buildSystemPrompt("agent", nil, nil, "", nil)
	if strings.Contains(off, "Model-family notes") {
		t.Fatal("expected shared base prompt when per-provider prompts are disabled")
	}
}

func TestBuildSystemPromptPerModelFileFromDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.md"), []byte("SHARED {{.CWD}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Model ref anthropic/claude-x slugifies to anthropic-claude-x.
	if err := os.WriteFile(filepath.Join(dir, "agent.anthropic-claude-x.md"), []byte("PERMODEL {{.CWD}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &session.State{ID: "t", CWD: t.TempDir(), Mode: session.ModeAgent}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "anthropic", Type: "anthropic", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "anthropic/claude-x", MaxTokens: 100}},
		Agent:     config.Agent{Model: "anthropic/claude-x"},
	}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Prompts.Dir = dir
	a := NewAgent(cfg, st, nil, nil)
	got := a.buildSystemPrompt("agent", nil, nil, "", nil)
	if !strings.Contains(got, "PERMODEL") {
		t.Fatalf("expected per-model prompt file to be selected, got: %.80s", got)
	}
}

func TestPromptVariantsIncludeResolvedAPIModel(t *testing.T) {
	st := &session.State{ID: "t", CWD: t.TempDir(), Mode: session.ModeAgent}
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "local", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "local/gpt-oss-20b", MaxTokens: 100}},
		Agent:     config.Agent{Model: "local/gpt-oss-20b"},
	}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()

	a := NewAgent(cfg, st, nil, nil)
	want := []string{"local-gpt-oss-20b", "gpt-oss-20b", "gpt-oss"}
	if got := a.promptVariants(); !slices.Equal(got, want) {
		t.Fatalf("promptVariants() = %v, want %v", got, want)
	}
}

// --- system_prompt.go: skills injection ------------------------------------

// TestAugmentUserMessageWithInvokedSkills_bodyInjected verifies that when a user
// explicitly invokes a slash command (/find-skills), its body is prepended to the
// user message sent to the LLM. The chat display (stored Content) remains unchanged.
//
// Regression: previously the body was never emitted because filteredInvoke compared
// against activeGlobCanon which always included no-glob skills, preventing injection
// in both the system-prompt ephemeral section and (by extension) the user message.
func TestAugmentUserMessageWithInvokedSkills_bodyInjected(t *testing.T) {
	const body = "UNIQUE_FIND_SKILLS_BODY_TOKEN"
	sk := &skills.Skill{
		Name:        "SKILL",
		FilePath:    filepath.Join("skills", "find-skills", "SKILL.md"),
		Description: "find skills",
		Content:     body,
	}

	userText := "/find-skills search pdf"
	result := augmentUserMessageWithInvokedSkills(userText, []*skills.Skill{sk})

	if !strings.Contains(result, body) {
		t.Fatalf("expected skill body %q to be prepended to user message; got:\n%s", body, result)
	}
	if !strings.Contains(result, userText) {
		t.Fatalf("expected original user text %q to be preserved in result; got:\n%s", userText, result)
	}
	// Skill body must come BEFORE the original user text.
	if strings.Index(result, body) > strings.Index(result, userText) {
		t.Fatalf("skill body should appear before user text in augmented message")
	}
}

// TestAugmentUserMessageWithInvokedSkills_noSkillMatch returns userText unchanged when
// the invoked name does not match any loaded skill.
func TestAugmentUserMessageWithInvokedSkills_noSkillMatch(t *testing.T) {
	sk := &skills.Skill{
		Name:     "SKILL",
		FilePath: filepath.Join("skills", "other", "SKILL.md"),
		Content:  "other body",
	}
	userText := "/find-skills pdf"
	result := augmentUserMessageWithInvokedSkills(userText, []*skills.Skill{sk})
	if result != userText {
		t.Fatalf("expected unchanged userText when no skill matches; got:\n%s", result)
	}
}

// TestAugmentUserMessageWithInvokedSkills_noSlashCommand returns userText unchanged when
// the message contains no slash command.
func TestAugmentUserMessageWithInvokedSkills_noSlashCommand(t *testing.T) {
	sk := &skills.Skill{
		Name:     "SKILL",
		FilePath: filepath.Join("skills", "find-skills", "SKILL.md"),
		Content:  "body",
	}
	userText := "поищи что-нибудь"
	result := augmentUserMessageWithInvokedSkills(userText, []*skills.Skill{sk})
	if result != userText {
		t.Fatalf("expected unchanged userText when no slash command; got:\n%s", result)
	}
}

// TestBuildSkillsPromptMarkdown_catalogSkillBodyNotInSystemPrompt verifies that slash
// command skill bodies are NOT injected into the system prompt; only the catalog listing
// appears there. Bodies travel via user message augmentation instead.
func TestBuildSkillsPromptMarkdown_catalogSkillBodyNotInSystemPrompt(t *testing.T) {
	const body = "UNIQUE_FIND_SKILLS_BODY_TOKEN"
	sk := &skills.Skill{
		Name:        "SKILL",
		FilePath:    filepath.Join("skills", "find-skills", "SKILL.md"),
		Description: "find skills",
		Content:     body,
	}

	allLoaded := []*skills.Skill{sk}
	active := skills.FilterForContext(allLoaded, nil)

	result := buildSkillsPromptMarkdown(allLoaded, active)

	if strings.Contains(result, body) {
		t.Fatalf("slash command skill body should NOT be in system prompt; got:\n%s", result)
	}
	if !strings.Contains(result, "find-skills") {
		t.Fatalf("skill name should appear in the slash catalog; got:\n%s", result)
	}
}

// TestBuildSkillsPromptMarkdown_noGlobNonCatalogBodyInSystemPrompt verifies that a
// skill with no globs that is NOT in the catalog has its body in the system prompt.
func TestBuildSkillsPromptMarkdown_noGlobNonCatalogBodyInSystemPrompt(t *testing.T) {
	const body = "NO_GLOB_NON_CATALOG_BODY"
	// This skill uses a path that doesn't become a slash command name in the catalog.
	sk := &skills.Skill{
		Name:     "my-always-rule",
		FilePath: filepath.Join("rules", "my-always-rule.md"),
		Content:  body,
	}

	allLoaded := []*skills.Skill{sk}
	active := skills.FilterForContext(allLoaded, nil)

	result := buildSkillsPromptMarkdown(allLoaded, active)

	if !strings.Contains(result, body) {
		t.Fatalf("always-apply non-catalog skill body should be in system prompt; got:\n%s", result)
	}
}

// --- toolsets.go -----------------------------------------------------------

func TestDocsToolSetFiltersToReadAndDocsWrite(t *testing.T) {
	r := tools.NewRegistry()
	set := ToolSetForMode("docs", false)
	filtered := FilterToolDefinitions(r.AllToolDefinitions(), set)
	got := make(map[string]bool)
	for _, d := range filtered {
		got[d.Name] = true
	}
	for _, want := range []string{"read", "glob", "grep", "websearch", "webfetch", "question", "docs_write", "docs_edit"} {
		if !got[want] {
			t.Errorf("docs toolset should include %q", want)
		}
	}
	for _, forbid := range []string{"write", "edit", "run_command", "plan_write", "foxxycode_todo_plan_read"} {
		if got[forbid] {
			t.Errorf("docs toolset should not include %q", forbid)
		}
	}
}

func TestModeAllowsMCPTools(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want bool
	}{
		{mode: "agent", want: true},
		{mode: "plan", want: true},
		{mode: "debug", want: true},
		{mode: "docs", want: false},
		{mode: "ask", want: false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			if got := ModeAllowsMCPTools(tc.mode); got != tc.want {
				t.Fatalf("ModeAllowsMCPTools(%q) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestAskToolSetFiltersToReadAndWeb(t *testing.T) {
	r := tools.NewRegistry()
	set := ToolSetForMode("ask", false)
	filtered := FilterToolDefinitions(r.AllToolDefinitions(), set)
	got := make(map[string]bool)
	for _, d := range filtered {
		got[d.Name] = true
	}
	for _, want := range []string{"read", "keep_result", "glob", "grep", "print_tree", "websearch", "webfetch", "question"} {
		if !got[want] {
			t.Errorf("ask toolset should include %q", want)
		}
	}
	for _, forbid := range []string{
		"write", "edit", "apply_patch", "mkdir", "rm", "run_command", "background_list",
		"docs_write", "docs_edit", "plan_write", "plan_list", "plan_read", "plan_exit",
		"config_get", "config_set", "ssh_run_command", "foxxycode_todo_plan_read",
		"foxxycode_scheduler_jobs_list", "foxxycode_scheduler_job_create", "svn_status",
	} {
		if got[forbid] {
			t.Errorf("ask toolset should not include %q", forbid)
		}
	}
}

func TestToolCallRefusedByModeEnforcesAskOnly(t *testing.T) {
	if msg, refused := toolCallRefusedByMode("ask", "write", false); !refused || !strings.Contains(msg, "Ask mode") {
		t.Errorf("ask mode must refuse write at execution time, got refused=%v msg=%q", refused, msg)
	}
	if _, refused := toolCallRefusedByMode("ask", "mcp_server__lookup", false); !refused {
		t.Error("ask mode must refuse MCP tool calls at execution time")
	}
	if _, refused := toolCallRefusedByMode("ask", "run_command", false); !refused {
		t.Error("ask mode must refuse run_command at execution time")
	}
	if _, refused := toolCallRefusedByMode("ask", "read", false); refused {
		t.Error("ask mode must allow read")
	}
	for _, mode := range []string{"agent", "plan", "debug", "docs"} {
		if _, refused := toolCallRefusedByMode(mode, "write", false); refused {
			t.Errorf("%s mode must not enforce the ask refusal", mode)
		}
	}
}

// A shell call replayed into an ask session is refused before it runs, whatever
// the command: ask mode offers no shell at all.
func TestAskModeRefusesShellBeforeExecution(t *testing.T) {
	cwd := t.TempDir()
	st := &session.State{
		ID:   "sess_ask_shell_guard",
		CWD:  cwd,
		Mode: session.ModeAsk,
	}
	ag := NewAgent(&config.Config{}, st, resumePermissionSender{}, nil)
	env := &tools.Env{CWD: cwd, PermissionMode: config.PermModeBypass}

	result, err := ag.executeToolCall(context.Background(), llm.ToolCall{
		ID:        "call_write",
		Name:      "run_command",
		InputJSON: `{"command":"echo changed > created-by-ask.txt"}`,
	}, env, string(session.ModeAsk), st.ID, false, 0)
	if err != nil {
		t.Fatalf("shell call should be returned as a policy result, got error: %v", err)
	}
	if !strings.Contains(result, "not available in Ask mode") {
		t.Fatalf("unexpected refusal result: %q", result)
	}
	if _, err := os.Stat(filepath.Join(cwd, "created-by-ask.txt")); !os.IsNotExist(err) {
		t.Fatalf("Ask shell command changed the workspace; stat err=%v", err)
	}
}

func TestPlanToolSetFiltersToReadWebAndShell(t *testing.T) {
	r := tools.NewRegistry()
	set := ToolSetForMode("plan", false)
	filtered := FilterToolDefinitions(r.AllToolDefinitions(), set)
	got := make(map[string]bool)
	for _, d := range filtered {
		got[d.Name] = true
	}
	for _, want := range []string{"read", "keep_result", "glob", "grep", "websearch", "webfetch", "run_command", "question", "plan_write", "plan_list", "plan_read"} {
		if !got[want] {
			t.Errorf("plan toolset should include %q", want)
		}
	}
	for _, forbid := range []string{"write", "foxxycode_todo_plan_read"} {
		if got[forbid] {
			t.Errorf("plan toolset should not include %q", forbid)
		}
	}
	// Default: the model may finish planning and start the implementation itself.
	if !got["plan_exit"] {
		t.Error("plan toolset should include plan_exit by default")
	}
}

func TestPlanToolSetDropsPlanExitWhenSelfRunForbidden(t *testing.T) {
	r := tools.NewRegistry()
	set := ToolSetForMode("plan", true)
	filtered := FilterToolDefinitions(r.AllToolDefinitions(), set)
	for _, d := range filtered {
		if d.Name == "plan_exit" {
			t.Fatal("plan_exit must not be offered when plan_no_self_run is on")
		}
	}
	if !set.Allows("plan_write") {
		t.Error("the guard must not touch the rest of the plan toolset")
	}
	if set.Allows("plan_exit") {
		t.Error("Allows must refuse plan_exit under the guard")
	}
}

func TestToolSetForAgentIsUnrestricted(t *testing.T) {
	set := ToolSetForMode("agent", false)
	if !set.Unrestricted() {
		t.Fatal("agent mode should use unrestricted tool set")
	}
}

func TestKeepResultIsAvailableInReadCapableModes(t *testing.T) {
	for _, mode := range []string{"plan", "docs", "ask"} {
		if !ToolSetForMode(mode, false).Allows("keep_result") {
			t.Errorf("%s mode must offer keep_result for read/grep pinning", mode)
		}
	}
}

type progressThenAnswerProvider struct{ calls int }

func (p *progressThenAnswerProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *progressThenAnswerProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	switch p.calls {
	case 2:
		tc := llm.ToolCall{ID: "tc1", Name: "glob", InputJSON: `{"pattern":"*"}`}
		onChunk(llm.StreamChunk{ToolCall: &tc})
		return &llm.Response{ToolCalls: []llm.ToolCall{tc}, StopReason: "tool_use"}, nil
	case 5:
		onChunk(llm.StreamChunk{TextDelta: "done"})
		return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
	default: // 1, 3, 4: empty, reasoning-only turns
		onChunk(llm.StreamChunk{ReasoningDelta: "still thinking"})
		return &llm.Response{Content: "", StopReason: "end_turn"}, nil
	}
}

func TestRunReActLoopResetsEmptyCounterOnToolProgress(t *testing.T) {
	st := &session.State{
		ID:         "sess_progress",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	provider := &progressThenAnswerProvider{}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "convert the file"}})
	if err != nil {
		t.Fatalf("loop gave up while the model was still making progress: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q, want end_turn (the model eventually answered)", stop)
	}
	if provider.calls < 5 {
		t.Fatalf("provider called %d times; the loop gave up before the model answered", provider.calls)
	}
	msgs := st.GetMessages()
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, "done") {
		t.Fatalf("final answer not reached: %+v", last)
	}
}

// --- loop guard escalation and false-positive safety -----------------------

// alwaysDegeneratingProvider never recovers: every turn degenerates into the same
// repeated passage. The loop guard must give up after the nudge budget instead of
// nudging forever.
type alwaysDegeneratingProvider struct{ calls int }

func (p *alwaysDegeneratingProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *alwaysDegeneratingProvider) Stream(ctx context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	var produced strings.Builder
	for i := 0; i < 200 && ctx.Err() == nil; i++ {
		produced.WriteString(bddLoopedSentence)
		onChunk(llm.StreamChunk{TextDelta: bddLoopedSentence})
	}
	return &llm.Response{Content: produced.String(), StopReason: "tool_use"}, context.Canceled
}

func TestLoopGuardStopsTurnAfterNudgeBudget(t *testing.T) {
	st := &session.State{
		ID:         "sess_loop_budget",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	provider := &alwaysDegeneratingProvider{}
	nudges := 2
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 20, LoopNudgeMax: &nudges},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "do the thing"}})
	if err == nil {
		t.Fatal("expected the turn to stop with a notice once the nudge budget ran out")
	}
	if !strings.Contains(err.Error(), "repeating") {
		t.Fatalf("error should explain the loop: %v", err)
	}
	if stop != string(acp.StopReasonRefused) {
		t.Fatalf("stop = %q, want agent_refused", stop)
	}
	// One initial attempt plus one per nudge, and nowhere near max_turns.
	if provider.calls != nudges+1 {
		t.Fatalf("provider called %d times, want %d (initial attempt + %d nudges)", provider.calls, nudges+1, nudges)
	}
}

func TestLoopGuardDisabledLetsTheStreamRun(t *testing.T) {
	st := &session.State{
		ID:         "sess_loop_off",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	provider := &bddLoopProvider{
		channel:      loopAbortText,
		recoverAfter: 0,
		realAnswer:   "answered without interference",
		maxDeltas:    30,
	}
	off := false
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", LoopGuard: &off},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "go"}}); err != nil {
		t.Fatalf("turn failed with the guard disabled: %v", err)
	}
	if provider.cancelled != 0 {
		t.Fatal("the guard cancelled a stream even though loop_guard is false")
	}
}

// varyingToolProvider calls the same tool with different arguments every turn.
// That is ordinary progress, not a loop, and must never be blocked.
type varyingToolProvider struct{ calls int }

func (p *varyingToolProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *varyingToolProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	if p.calls > 6 {
		onChunk(llm.StreamChunk{TextDelta: "done"})
		return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
	}
	tc := llm.ToolCall{
		ID:        fmt.Sprintf("call_%d", p.calls),
		Name:      "glob",
		InputJSON: fmt.Sprintf(`{"pattern":"**/*%d.go"}`, p.calls),
	}
	onChunk(llm.StreamChunk{ToolCall: &tc})
	return &llm.Response{ToolCalls: []llm.ToolCall{tc}, StopReason: "tool_use"}, nil
}

func TestLoopGuardIgnoresVaryingToolArguments(t *testing.T) {
	st := &session.State{
		ID:         "sess_loop_varying",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
	}
	provider := &varyingToolProvider{}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: 20},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "search for things"}})
	if err != nil {
		t.Fatalf("the guard interfered with legitimate varying tool calls: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q, want end_turn", stop)
	}
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleTool && (m.Content == toolLoopNudge || m.Content == toolLoopSkippedResult) {
			t.Fatal("the loop guard blocked a call with different arguments")
		}
	}
}

type recordingPermissionSender struct {
	requests []acp.PermissionRequestParams
}

func (s *recordingPermissionSender) SendSessionUpdate(string, interface{}) error { return nil }

func (s *recordingPermissionSender) RequestPermission(_ context.Context, p acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	s.requests = append(s.requests, p)
	return &acp.PermissionResult{Outcome: "allow", OptionID: "allow"}, nil
}

func (s *recordingPermissionSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

type configReloadProvider struct {
	calls int
	tools [][]llm.ToolDefinition
}

func (p *configReloadProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *configReloadProvider) Stream(_ context.Context, _ []llm.Message, defs []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.calls++
	p.tools = append(p.tools, append([]llm.ToolDefinition(nil), defs...))
	switch p.calls {
	case 1:
		call := llm.ToolCall{ID: "cfg-1", Name: "config_set", InputJSON: `{"commands":["set skills.auto_discovery=false"]}`}
		onChunk(llm.StreamChunk{ToolCall: &call})
		return &llm.Response{ToolCalls: []llm.ToolCall{call}, StopReason: "tool_use"}, nil
	case 2:
		call := llm.ToolCall{ID: "cfg-2", Name: "config_commit", InputJSON: `{}`}
		onChunk(llm.StreamChunk{ToolCall: &call})
		return &llm.Response{ToolCalls: []llm.ToolCall{call}, StopReason: "tool_use"}, nil
	}
	onChunk(llm.StreamChunk{TextDelta: "Configuration reloaded."})
	return &llm.Response{Content: "Configuration reloaded.", StopReason: "end_turn"}, nil
}

func TestConfigSetRefreshesToolDefinitionsWithinSameTurn(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("skills:\n  auto_discovery: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}}
	cfg.Models = []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}}
	cfg.Agent.Model = "fake/model"
	st := &session.State{ID: "sess_config_reload", CWD: dir, Mode: session.ModeAgent}
	provider := &configReloadProvider{}
	ag := NewAgent(cfg, st, resumePermissionSender{}, nil)
	ag.SetConfigReloader(func(context.Context) ([]string, error) { return nil, nil })
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }

	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "disable skill discovery"}}); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want stage, commit, and final answer", provider.calls)
	}
	contains := func(defs []llm.ToolDefinition, name string) bool {
		for _, def := range defs {
			if def.Name == name {
				return true
			}
		}
		return false
	}
	if !contains(provider.tools[0], "load_skill") {
		t.Fatal("load_skill should be present before config_set")
	}
	if !contains(provider.tools[1], "load_skill") {
		t.Fatal("staging alone must not reload the runtime")
	}
	if contains(provider.tools[2], "load_skill") {
		t.Fatal("load_skill should be removed after same-turn config_commit reload")
	}
}

// Committing the agent's own config can start MCP processes and change the
// permission policy itself, so accept_edits must still prompt for
// config_commit (unlike project file writes), the prompt must show the staged
// commands, and only the explicit bypass mode may skip the dialog.
func TestConfigCommitPermissionPerMode(t *testing.T) {
	for _, tc := range []struct {
		mode        string
		wantPrompts int
	}{
		{mode: config.PermModeAcceptEdits, wantPrompts: 1},
		{mode: config.PermModeBypass, wantPrompts: 0},
	} {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(configPath, []byte("skills:\n  auto_discovery: true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Providers = []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}}
		cfg.Models = []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}}
		cfg.Agent.Model = "fake/model"
		cfg.Tools.PermissionMode = tc.mode
		st := &session.State{ID: "sess_cfg_perm_" + tc.mode, CWD: dir, Mode: session.ModeAgent}
		sender := &recordingPermissionSender{}
		provider := &configReloadProvider{}
		ag := NewAgent(cfg, st, sender, nil)
		ag.SetConfigReloader(func(context.Context) ([]string, error) { return nil, nil })
		ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }

		if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "disable skill discovery"}}); err != nil {
			t.Fatalf("mode %s: %v", tc.mode, err)
		}
		var commitPrompts []acp.PermissionRequestParams
		for _, req := range sender.requests {
			if strings.Contains(req.ToolCall.Title, "config_commit") {
				commitPrompts = append(commitPrompts, req)
			}
		}
		if len(commitPrompts) != tc.wantPrompts {
			t.Fatalf("mode %s: config_commit permission prompts = %d, want %d", tc.mode, len(commitPrompts), tc.wantPrompts)
		}
		if tc.wantPrompts > 0 {
			body := ""
			for _, item := range commitPrompts[0].ToolCall.Content {
				body += item.Content.Text
			}
			if !strings.Contains(body, "set skills.auto_discovery=false") {
				t.Fatalf("mode %s: permission prompt does not show the staged commands: %q", tc.mode, body)
			}
		}
	}
}

// A pending agent-mode call approved after the session switched to ask must be
// refused, and an "allow always" answer must not leave a grant behind for the
// call that never ran.
func TestResumeAfterPermissionInAskModeRefusesAndRecordsNoGrant(t *testing.T) {
	st := &session.State{
		ID:         "sess_resume_ask",
		CWD:        t.TempDir(),
		Mode:       session.ModeAsk,
		SessionDir: t.TempDir(),
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "run it"},
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID:        "call_ask_hidden",
					Name:      "run_command",
					InputJSON: `{"command":"printf SHOULD_NOT_RUN"}`,
				}},
			},
		},
	}
	provider := &resumePermissionProvider{t: t}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) {
		return provider, nil
	}

	// Every persisted bundle carries the arguments the prompt showed; a
	// resume without them fails closed before the mode check.
	if err := session.WriteToolCallArgs(st.SessionDir, "call_ask_hidden", `{"command":"printf SHOULD_NOT_RUN"}`); err != nil {
		t.Fatal(err)
	}
	stop, err := ag.ResumeAfterPermission(context.Background(), "call_ask_hidden", &acp.PermissionResult{
		Outcome:  "allow",
		OptionID: "allow_always",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop reason %q", stop)
	}
	var toolMsg *llm.Message
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleTool && m.ToolCallID == "call_ask_hidden" {
			mm := m
			toolMsg = &mm
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("missing tool result for the refused call")
	}
	if strings.Contains(toolMsg.Content, "SHOULD_NOT_RUN") {
		t.Fatalf("the approved call executed in ask mode: %q", toolMsg.Content)
	}
	if !strings.Contains(toolMsg.Content, "not available in Ask mode") {
		t.Fatalf("tool result is not the ask-mode refusal: %q", toolMsg.Content)
	}
	if grants := st.GetPermissionCommandGrants(); len(grants) != 0 {
		t.Fatalf("refused call still recorded an allow-always grant: %v", grants)
	}
}

type todoSnapshotSender struct {
	updates []interface{}
}

func (s *todoSnapshotSender) SendSessionUpdate(_ string, update interface{}) error {
	s.updates = append(s.updates, update)
	return nil
}

func (*todoSnapshotSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	return &acp.PermissionResult{Outcome: "allow", OptionID: "allow"}, nil
}

func (*todoSnapshotSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

func TestTodoItemUpdateSavesAndPublishesFinalPlanSnapshot(t *testing.T) {
	dir := t.TempDir()
	st := &session.State{
		ID:         "sess_todo_snapshot",
		CWD:        dir,
		Mode:       session.ModeAgent,
		SessionDir: dir,
	}
	st.SetPlan([]acp.PlanEntry{
		{Content: "Inspect existing cards", Status: "completed"},
		{Content: "Render structured preview", Status: "pending"},
	})
	sender := &todoSnapshotSender{}
	ag := NewAgent(&config.Config{}, st, sender, nil)

	_, err := ag.executeToolCall(
		context.Background(),
		llm.ToolCall{
			ID:        "todo-update-1",
			Name:      todo.ToolNameItemUpdate,
			InputJSON: `{"index":1,"status":"completed"}`,
		},
		ag.buildToolEnv(string(session.ModeAgent), dir),
		string(session.ModeAgent),
		st.ID,
		false,
		0,
	)
	if err != nil {
		t.Fatalf("executeToolCall: %v", err)
	}

	meta, err := session.ReadToolCallMeta(dir, "todo-update-1")
	if err != nil {
		t.Fatalf("ReadToolCallMeta: %v", err)
	}
	// One meta.json carries both the outcome and the snapshot: a transcript
	// reload must never see a completed todo call without its plan rows.
	if meta.Status != "completed" {
		t.Fatalf("persisted Status = %q, want completed", meta.Status)
	}
	if len(meta.PlanSnapshot) != 2 || meta.PlanSnapshot[1].Status != "completed" {
		t.Fatalf("persisted PlanSnapshot = %+v", meta.PlanSnapshot)
	}

	st.SetPlan([]acp.PlanEntry{{Content: "Later plan", Status: "pending"}})
	persisted, err := session.ReadToolCallMeta(dir, "todo-update-1")
	if err != nil || persisted.PlanSnapshot[1].Content != "Render structured preview" {
		t.Fatalf("historical plan snapshot changed: meta=%+v err=%v", persisted, err)
	}

	var completed acp.ToolCallStatusUpdate
	for _, update := range sender.updates {
		candidate, ok := update.(acp.ToolCallStatusUpdate)
		if ok && candidate.Status == "completed" {
			completed = candidate
		}
	}
	foxxycode, _ := completed.Meta["foxxycode"].(map[string]interface{})
	sent, _ := foxxycode["todoPlan"].([]acp.PlanEntry)
	if len(sent) != 2 || sent[1].Status != "completed" {
		t.Fatalf("SSE todoPlan = %+v", sent)
	}
}

// rotatingToolProvider cycles through a fixed set of globs, the shape a model
// falls into when it keeps re-fetching context it believes it lost. No two
// consecutive calls are identical, so toolRepeatDetector never sees it. Once
// answerWhenBlocked turns true it stops calling tools and answers instead, which
// is what a model does after the guard has taken the loop away.
type rotatingToolProvider struct {
	calls    int
	patterns []string
	// answerWhenBlocked makes the model give up on tools after this many calls.
	answerAfter int
	// toollessCalls counts requests that arrived with no tool definitions.
	toollessCalls int
	answer        string
}

func (p *rotatingToolProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return nil, nil
}

func (p *rotatingToolProvider) Stream(_ context.Context, _ []llm.Message, defs []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	if len(defs) == 0 {
		p.toollessCalls++
		answer := p.answer
		if answer == "" {
			answer = "here is what I found"
		}
		onChunk(llm.StreamChunk{TextDelta: answer})
		return &llm.Response{Content: answer, StopReason: "end_turn"}, nil
	}
	if p.answerAfter > 0 && p.calls >= p.answerAfter {
		answer := p.answer
		if answer == "" {
			answer = "here is what I found"
		}
		onChunk(llm.StreamChunk{TextDelta: answer})
		return &llm.Response{Content: answer, StopReason: "end_turn"}, nil
	}
	tc := llm.ToolCall{
		ID:        fmt.Sprintf("call_%d", p.calls),
		Name:      "glob",
		InputJSON: fmt.Sprintf(`{"pattern":%q}`, p.patterns[p.calls%len(p.patterns)]),
	}
	p.calls++
	onChunk(llm.StreamChunk{ToolCall: &tc})
	return &llm.Response{ToolCalls: []llm.ToolCall{tc}, StopReason: "tool_use"}, nil
}

func loopGuardAgent(t *testing.T, id string, provider llm.Provider, stuckAction string, maxTurns int) (*Agent, *session.State) {
	t.Helper()
	st := &session.State{ID: id, CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	ag := NewAgent(&config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model", MaxTurns: maxTurns, LoopStuckAction: stuckAction},
	}, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	return ag, st
}

// everyCallAnswered asserts the invariant OpenAI-compatible endpoints depend on:
// a tool_call_id that was announced must carry a result.
func everyCallAnswered(t *testing.T, st *session.State) {
	t.Helper()
	announced, results := 0, map[string]bool{}
	for _, m := range st.GetMessages() {
		announced += len(m.ToolCalls)
		if m.Role == llm.RoleTool {
			results[m.ToolCallID] = true
		}
	}
	if len(results) != announced {
		t.Fatalf("%d tool calls announced but %d results recorded", announced, len(results))
	}
}

func TestLoopGuardQuarantineLetsTheTurnFinish(t *testing.T) {
	// The model rotates until the guard takes the loop away, then answers - the
	// point of quarantining rather than ending the turn is that this answer still
	// reaches the user.
	provider := &rotatingToolProvider{
		patterns:    []string{"**/*a.go", "**/*b.go", "**/*c.go"},
		answerAfter: 12,
		answer:      "three packages match",
	}
	ag, st := loopGuardAgent(t, "sess_quarantine", provider, "", 20)

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "look around"}})
	if err != nil {
		t.Fatalf("a quarantined loop must not fail the turn: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q, want end_turn", stop)
	}
	last := st.GetMessages()[len(st.GetMessages())-1]
	if !strings.Contains(last.Content, "three packages match") {
		t.Fatalf("the turn lost the model's answer: %q", last.Content)
	}
	var blocked int
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleTool && m.Content == toolQuarantinedResult {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("the guard never took the loop away")
	}
	everyCallAnswered(t, st)
}

func TestLoopGuardForcesAnAnswerWhenOnlyBlockedCallsRemain(t *testing.T) {
	// This model knows nothing but the loop. Quarantine alone would let it ask for
	// blocked calls until max_turns and end with nothing, so the guard withholds
	// the tools for one request and takes the answer.
	provider := &rotatingToolProvider{patterns: []string{"**/*a.go", "**/*b.go", "**/*c.go"}}
	maxTurns := 20
	ag, st := loopGuardAgent(t, "sess_forced_answer", provider, "", maxTurns)

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "look around"}})
	if err != nil {
		t.Fatalf("the turn must end with an answer, not an error: %v", err)
	}
	if stop != string(acp.StopReasonEndTurn) {
		t.Fatalf("stop = %q, want end_turn", stop)
	}
	if provider.toollessCalls == 0 {
		t.Fatal("the guard never withheld the tools, so the model was never made to answer")
	}
	if provider.calls >= maxTurns {
		t.Fatalf("ran %d tool-calling turns of %d; the guard should cut in well before max_turns", provider.calls, maxTurns)
	}
	everyCallAnswered(t, st)
}

func TestLoopGuardStopActionStillEndsTheTurn(t *testing.T) {
	// The ported behaviour stays available behind agent.loop_stuck_action: stop.
	provider := &rotatingToolProvider{patterns: []string{"**/*a.go", "**/*b.go", "**/*c.go"}}
	maxTurns := 20
	ag, st := loopGuardAgent(t, "sess_stop_cycle", provider, config.AgentLoopStuckActionStop, maxTurns)

	stop, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "look around"}})
	if stop != string(acp.StopReasonRefused) {
		t.Fatalf("stop = %q (err %v), want agent_refused", stop, err)
	}
	if err == nil || err.Error() != toolCycleStopNotice {
		t.Fatalf("err = %v, want the cycle notice the SPA localizes", err)
	}
	if provider.calls >= maxTurns {
		t.Fatalf("the cycle ran for %d turns, want it cut well before max_turns %d", provider.calls, maxTurns)
	}
	everyCallAnswered(t, st)
}

func TestLoopGuardStopActionKeepsIdenticalRunsOnTheRepeatCheck(t *testing.T) {
	// One pattern: every call is identical, which is the repeat detector's shape,
	// and its notice must not be replaced by the cycle one.
	provider := &rotatingToolProvider{patterns: []string{"**/*a.go"}}
	ag, st := loopGuardAgent(t, "sess_stop_identical", provider, config.AgentLoopStuckActionStop, 20)

	_, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "look around"}})
	if err == nil || !strings.Contains(err.Error(), "with identical arguments") {
		t.Fatalf("err = %v, want the identical-arguments notice, not the cycle one", err)
	}
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleTool && (m.Content == toolCycleNudge || m.Content == toolCycleSkippedResult) {
			t.Fatal("the cycle detector claimed a run the repeat detector owns")
		}
	}
}

// resumeRewriteFixture prepares a session whose pending run_command call was
// rewritten by a PreToolUse hook that then asked for permission: the history
// holds the model's original arguments, the bundle holds the arguments the
// prompt showed, exactly the state a persisted approval resumes from.
func resumeRewriteFixture(t *testing.T, hookCommand string) (*Agent, *session.State, string) {
	t.Helper()
	home := t.TempDir()
	if err := hooktest.Write(filepath.Join(home, "hooks.json"), hooktest.Entry{
		Event:    hooks.EventPreToolUse,
		Matcher:  "run_command",
		Handlers: []hooks.Handler{hooktest.Handler("rewrite-ask", hookCommand)},
	}); err != nil {
		t.Fatal(err)
	}
	st := &session.State{
		ID:         "sess_resume_rewrite",
		CWD:        t.TempDir(),
		Mode:       session.ModeAgent,
		SessionDir: t.TempDir(),
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "run the command"},
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID:        "call_rewrite",
					Name:      "run_command",
					InputJSON: `{"command":"echo original-arguments"}`,
				}},
			},
		},
	}
	// What the permission prompt showed: the arguments after the first
	// PreToolUse run, persisted by executeToolCall before the prompt.
	if err := session.WriteToolCallArgs(st.SessionDir, "call_rewrite", `{"command":"echo shown-and-approved"}`); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Paths:     config.Paths{Home: home, CWD: st.CWD},
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100}},
		Agent:     config.Agent{Model: "fake/model"},
	}
	cfg.Hooks.ApplyDefaults(cfg.Paths)
	provider := &resumePermissionProvider{t: t}
	ag := NewAgent(cfg, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	return ag, st, home
}

func resumedToolResult(st *session.State) string {
	for _, m := range st.GetMessages() {
		if m.Role == llm.RoleTool && m.ToolCallID == "call_rewrite" {
			return m.Content
		}
	}
	return ""
}

// The approval binds to the arguments the prompt showed: they run even when
// the hook that produced them is gone by the time the approval arrives.
func TestResumeAfterPermissionRunsTheApprovedArguments(t *testing.T) {
	ag, st, home := resumeRewriteFixture(t, "echo shown-and-approved")
	if err := os.Remove(filepath.Join(home, "hooks.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ag.ResumeAfterPermission(context.Background(), "call_rewrite", &acp.PermissionResult{Outcome: "selected", OptionID: "allow"}); err != nil {
		t.Fatal(err)
	}
	result := resumedToolResult(st)
	if !strings.Contains(result, "shown-and-approved") || strings.Contains(result, "original-arguments") {
		t.Fatalf("the resumed call must run the approved arguments, got %q", result)
	}
	if grants := st.GetPermissionCommandGrants(); len(grants) != 0 {
		t.Fatalf("a plain allow records no grant, got %v", grants)
	}
}

// A hook that changes the approved arguments again on the resume is not
// covered by the answer the user gave: the call is cancelled instead.
func TestResumeAfterPermissionRefusesArgumentsChangedAfterTheApproval(t *testing.T) {
	ag, st, _ := resumeRewriteFixture(t, "echo changed-after-approval")
	if _, err := ag.ResumeAfterPermission(context.Background(), "call_rewrite", &acp.PermissionResult{Outcome: "selected", OptionID: "allow"}); err != nil {
		t.Fatal(err)
	}
	result := resumedToolResult(st)
	if strings.Contains(result, "changed-after-approval") || strings.Contains(result, "shown-and-approved") || !strings.Contains(result, "cancelled") {
		t.Fatalf("a call rewritten after the approval must not run, got %q", result)
	}
}

// promptRefusingSender fails the test if a permission prompt is issued.
type promptRefusingSender struct {
	t        *testing.T
	prompted bool
}

func (*promptRefusingSender) SendSessionUpdate(string, interface{}) error { return nil }

func (s *promptRefusingSender) RequestPermission(context.Context, acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	s.prompted = true
	s.t.Error("no permission prompt must be issued")
	return &acp.PermissionResult{Outcome: "cancelled", OptionID: "reject"}, nil
}

func (*promptRefusingSender) RequestQuestion(context.Context, acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	return &acp.QuestionResult{}, nil
}

// toolCallArgsPath finds the persisted args.json of the fixture's tool call.
func toolCallArgsPath(t *testing.T, sessionDir string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(sessionDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "args.json" {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatalf("no persisted arguments under %s", sessionDir)
	}
	return found
}

// The same hook answering again on the resume is not a change: the bundle
// stores the arguments pretty-printed and the hook answers them compact, and
// the approval must survive that formatting difference.
func TestResumeAfterPermissionRunsWhenTheSameHookAnswersAgain(t *testing.T) {
	ag, st, _ := resumeRewriteFixture(t, "echo shown-and-approved")
	if _, err := ag.ResumeAfterPermission(context.Background(), "call_rewrite", &acp.PermissionResult{Outcome: "selected", OptionID: "allow"}); err != nil {
		t.Fatal(err)
	}
	result := resumedToolResult(st)
	if !strings.Contains(result, "shown-and-approved") || strings.Contains(result, "cancelled") {
		t.Fatalf("the hook that produced the approved arguments must not cancel the resume, got %q", result)
	}
}

// A bundle without persisted arguments has nothing the approval can bind to:
// the resume fails instead of running the history's arguments.
func TestResumeAfterPermissionFailsClosedWithoutPersistedArguments(t *testing.T) {
	ag, st, home := resumeRewriteFixture(t, "echo shown-and-approved")
	if err := os.Remove(filepath.Join(home, "hooks.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(toolCallArgsPath(t, st.SessionDir)); err != nil {
		t.Fatal(err)
	}
	_, err := ag.ResumeAfterPermission(context.Background(), "call_rewrite", &acp.PermissionResult{Outcome: "selected", OptionID: "allow"})
	if err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("a resume without persisted arguments must fail, got %v", err)
	}
	if result := resumedToolResult(st); result != "" {
		t.Fatalf("nothing must run without persisted arguments, got %q", result)
	}
}

// A refusal needs nothing from the bundle: it is recorded and the gate is
// cleared even when the persisted arguments cannot be read.
func TestResumeAfterPermissionRejectsWithoutReadingTheArguments(t *testing.T) {
	ag, st, _ := resumeRewriteFixture(t, "echo shown-and-approved")
	if err := session.WritePendingPermission(st.SessionDir, acp.PermissionRequestParams{
		SessionID: st.ID,
		ToolCall:  acp.PermissionToolCall{ToolCallID: "call_rewrite", Status: "pending"},
	}, "run_command", ""); err != nil {
		t.Fatal(err)
	}
	p := toolCallArgsPath(t, st.SessionDir)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ag.ResumeAfterPermission(context.Background(), "call_rewrite", &acp.PermissionResult{Outcome: "selected", OptionID: "reject"}); err != nil {
		t.Fatalf("a refusal must not depend on the arguments file: %v", err)
	}
	if result := resumedToolResult(st); result != "permission denied by user" {
		t.Fatalf("the refusal must be recorded, got %q", result)
	}
	if session.PendingPermissionHeld(st.SessionDir) {
		t.Fatal("the refused gate must be cleared")
	}
}

// Persisted arguments that cannot be read fail closed: nothing runs, and the
// error leaves the pending gate in place for another attempt.
func TestResumeAfterPermissionFailsClosedWhenTheApprovedArgumentsCannotBeRead(t *testing.T) {
	ag, st, _ := resumeRewriteFixture(t, "echo shown-and-approved")
	p := toolCallArgsPath(t, st.SessionDir)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	// A directory in place of the file: the read fails, and not with not-exist.
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ag.ResumeAfterPermission(context.Background(), "call_rewrite", &acp.PermissionResult{Outcome: "selected", OptionID: "allow"})
	if err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("an unreadable arguments file must fail the resume, got %v", err)
	}
	if result := resumedToolResult(st); result != "" {
		t.Fatalf("nothing must run when the approved arguments cannot be read, got %q", result)
	}
}

// Rewritten arguments that cannot be persisted cancel the call before the
// prompt: a resume would otherwise fall back to arguments the user never saw.
func TestRewrittenArgumentsThatCannotBePersistedCancelBeforeThePrompt(t *testing.T) {
	ag, st, _ := resumeRewriteFixture(t, "echo shown-and-approved")
	toolCalls := filepath.Dir(filepath.Dir(toolCallArgsPath(t, st.SessionDir)))
	if err := os.RemoveAll(toolCalls); err != nil {
		t.Fatal(err)
	}
	// A file where the tool call directories live: every write fails.
	if err := os.WriteFile(toolCalls, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	sender := &promptRefusingSender{t: t}
	ag.server = sender
	tc := st.GetMessages()[1].ToolCalls[0]
	env := ag.buildToolEnv(st.GetMode(), st.SessionDir)
	result, err := ag.executeToolCall(context.Background(), tc, env, st.GetMode(), st.GetID(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "cancelled") || !strings.Contains(result, "persisted") {
		t.Fatalf("a rewrite that cannot be persisted must cancel the call, got %q", result)
	}
	if sender.prompted {
		t.Fatal("the prompt must not be issued for arguments that were not persisted")
	}
}

// The comparison behind the resume check keeps number literals verbatim: a
// float64 decode would read two integers past 2^53 as the same arguments.
func TestSameToolArgsKeepsLargeIntegersApart(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		same bool
	}{
		{"formatting", `{"command":"echo x","n":1}`, "{\n  \"n\": 1,\n  \"command\": \"echo x\"\n}\n", true},
		{"large integers", `{"n":9007199254740992}`, `{"n":9007199254740993}`, false},
		{"float literal", `{"n":1.0}`, `{"n":1}`, false},
		{"different values", `{"command":"echo a"}`, `{"command":"echo b"}`, false},
		{"invalid", `{not json`, `{not json`, true},
		{"one invalid", `{"n":1}`, `{n:1}`, false},
	}
	for _, c := range cases {
		if got := sameToolArgs(c.a, c.b); got != c.same {
			t.Errorf("%s: sameToolArgs(%q, %q) = %v, want %v", c.name, c.a, c.b, got, c.same)
		}
	}
}

// TestBuildSystemPromptCustomTemplateWithoutRulesKeepsInstructions is the
// regression Codex found in review: the project AGENTS.md is dropped from
// {{.Instructions}} because the rules block carries it, so a template under
// prompts.dir that renders {{.Instructions}} and not {{.Rules}} would end up
// with neither copy.
func TestBuildSystemPromptCustomTemplateWithoutRulesKeepsInstructions(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("PROJECT_DOC_TOKEN"), 0o644); err != nil {
		t.Fatal(err)
	}
	promptsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptsDir, "agent.md"), []byte("You are FoxxyCode.\n\n{{.Instructions}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.ReplaceRulesCatalog(session.DiscoverRules(&config.Config{}, tmp))
	cfg := &config.Config{Paths: config.Paths{CWD: tmp}}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Instructions.ApplyDefaults()
	cfg.Prompts.Dir = promptsDir
	a := NewAgent(cfg, st, nil, nil)

	prompt := a.buildSystemPrompt("agent", nil, nil, "", nil)
	if n := strings.Count(prompt, "PROJECT_DOC_TOKEN"); n != 1 {
		t.Fatalf("a template without {{.Rules}} carries the project AGENTS.md %d time(s), want 1:\n%s", n, prompt)
	}

	// With the built-in template the rules block carries it, exactly once.
	cfg.Prompts.Dir = ""
	if n := strings.Count(a.buildSystemPrompt("agent", nil, nil, "", nil), "PROJECT_DOC_TOKEN"); n != 1 {
		t.Fatalf("the built-in template carries the project AGENTS.md %d time(s), want 1", n)
	}
}

// --- turn_context.go: what travels after the history ------------------------

// The projection the loop hands to the provider is built from the working
// message slice; appending the block must never reach back into it, or the next
// tool result would land on top of a request FoxxyCode already sent.
func TestWithTurnContextDoesNotWriteIntoTheCallersSlice(t *testing.T) {
	base := make([]llm.Message, 2, 8) // spare capacity: a naive append would stomp it
	base[0] = llm.Message{Role: llm.RoleSystem, Content: "system"}
	base[1] = llm.Message{Role: llm.RoleUser, Content: "hello"}

	sent := withTurnContext(base, "<turn_context>\nclock\n</turn_context>")
	if len(sent) != 3 || len(base) != 2 {
		t.Fatalf("lengths: sent=%d base=%d", len(sent), len(base))
	}
	grown := append(base, llm.Message{Role: llm.RoleTool, Content: "result"}) //nolint:gocritic // the point of the test
	if sent[2].Content != "<turn_context>\nclock\n</turn_context>" {
		t.Fatalf("appending to the caller's slice overwrote the sent block: %q", sent[2].Content)
	}
	if grown[2].Content != "result" {
		t.Fatalf("the caller's own append was disturbed: %q", grown[2].Content)
	}
}

func TestWithTurnContextSendsHistoryAloneWhenTheBlockIsEmpty(t *testing.T) {
	base := []llm.Message{{Role: llm.RoleSystem, Content: "system"}}
	if got := withTurnContext(base, "   "); len(got) != 1 {
		t.Fatalf("an empty block must add no message, got %d", len(got))
	}
}

func TestBuildTurnContextCarriesClockTodoAndNewlyActivatedRules(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, ".foxxycode", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ndescription: Go files\nglobs: **/*.go\nalwaysApply: false\n---\n\nTURN_CTX_RULE_TOKEN\n"
	if err := os.WriteFile(filepath.Join(rulesDir, "gofiles.mdc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.ReplaceRulesCatalog(session.DiscoverRules(&config.Config{}, tmp))
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	a := NewAgent(cfg, st, nil, nil)
	a.clock = func() time.Time { return time.Date(2038, 1, 19, 3, 14, 7, 0, time.UTC) }

	sys := a.buildSystemPromptParts("agent", nil, nil, "", nil)
	if strings.Contains(sys.Content, "TURN_CTX_RULE_TOKEN") {
		t.Fatal("a glob rule reached the system prompt before any tool touched a matching file")
	}

	block := a.buildTurnContext(sys)
	if !strings.Contains(block, "2038-01-19T03:14:07Z") {
		t.Fatalf("turn context lost the clock: %q", block)
	}
	if strings.Contains(block, "TURN_CTX_RULE_TOKEN") {
		t.Fatalf("turn context carries a rule nothing activated: %q", block)
	}

	st.SetPlan([]acp.PlanEntry{{Content: "TURN_CTX_TODO_TOKEN", Status: "pending"}})
	a.activateScopedRulesForToolCall("read", `{"path":"main.go"}`, tmp)

	block = a.buildTurnContext(sys)
	if !strings.Contains(block, "TURN_CTX_TODO_TOKEN") {
		t.Fatalf("turn context lost the checklist: %q", block)
	}
	if !strings.Contains(block, "TURN_CTX_RULE_TOKEN") {
		t.Fatalf("turn context lost the rule the read activated: %q", block)
	}
	// The frozen prompt is what the provider already has cached: the rule must
	// not be folded back into it mid-turn.
	if frozen := sys.Content; strings.Contains(frozen, "TURN_CTX_RULE_TOKEN") {
		t.Fatal("the activated rule rewrote the frozen system prompt")
	}
}

// A turn renders its system prompt more than once - a rebuild after compaction,
// the continuation a permission answer starts - so reading the plan hand-off
// must not consume it. That destructive read is what used to drop the plan
// halfway through the turn that was carrying it out.
func TestSystemPromptRebuildKeepsThePlanContext(t *testing.T) {
	tmp := t.TempDir()
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.SetPendingPlanContext("PLAN_HANDOFF_TOKEN")
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	a := NewAgent(cfg, st, nil, nil)

	for i := 1; i <= 3; i++ {
		if got := a.buildSystemPromptParts("agent", nil, nil, "", nil); !strings.Contains(got.Content, "PLAN_HANDOFF_TOKEN") {
			t.Fatalf("build %d lost the plan hand-off", i)
		}
	}

	// And it is let go when the turn ends, so the next one starts clean.
	a.releasePlanContext()
	if got := a.buildSystemPromptParts("agent", nil, nil, "", nil); strings.Contains(got.Content, "PLAN_HANDOFF_TOKEN") {
		t.Fatal("the plan hand-off outlived the turn that ran the plan")
	}
}

// The hand-off is released by the turn that ran the plan, but not while a
// permission gate is still held in the bundle: what answers that gate renders
// this turn's system prompt again, possibly in another process.
func TestPlanContextSurvivesWhileAPermissionGateIsHeld(t *testing.T) {
	tmp := t.TempDir()
	store := &session.FileStore{Root: t.TempDir()}
	sd, err := store.EnsureLayout("sess_gate_hold")
	if err != nil {
		t.Fatal(err)
	}
	st := &session.State{ID: "sess_gate_hold", CWD: tmp, Mode: session.ModeAgent, SessionDir: sd}
	st.SetPendingPlanContext("PLAN_HANDOFF_TOKEN")
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	a := NewAgent(cfg, st, nil, nil)

	if err := session.WritePendingPermission(sd, acp.PermissionRequestParams{
		SessionID: "sess_gate_hold",
		ToolCall:  acp.PermissionToolCall{ToolCallID: "call_held", Title: "Run: run_command", Status: "pending"},
	}, "run_command", "{}"); err != nil {
		t.Fatal(err)
	}
	a.releasePlanContext()
	if st.PendingPlanContext() != "PLAN_HANDOFF_TOKEN" {
		t.Fatal("the hand-off was released while a permission gate was still held")
	}

	if err := session.ClearPendingPermission(sd); err != nil {
		t.Fatal(err)
	}
	a.releasePlanContext()
	if st.PendingPlanContext() != "" {
		t.Fatal("the hand-off outlived the gate that was holding it")
	}
	if session.ReadPendingPlanContext(sd) != "" {
		t.Fatal("the hand-off is still in the bundle after the turn ended")
	}
}

// The built-in plan and ask templates never showed the session checklist, and
// neither mode offers the todo tools. Moving the block into the turn context
// must not start showing it there.
func TestTurnContextCarriesTheChecklistInAgentModeOnly(t *testing.T) {
	tmp := t.TempDir()
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.SetPlan([]acp.PlanEntry{{Content: "MODE_TODO_TOKEN", Status: "pending"}})
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	a := NewAgent(cfg, st, nil, nil)

	for mode, want := range map[string]bool{"agent": true, "plan": false, "ask": false} {
		sys := a.buildSystemPromptParts(mode, nil, nil, "", nil)
		block := a.buildTurnContext(sys)
		if got := strings.Contains(block, "MODE_TODO_TOKEN"); got != want {
			t.Errorf("%s mode: checklist in the turn context = %v, want %v", mode, got, want)
		}
		if strings.Contains(sys.Content, "MODE_TODO_TOKEN") {
			t.Errorf("%s mode: the checklist reached the frozen system prompt", mode)
		}
	}
}

// A template under prompts.dir that prints {{.UTCNow}} or {{.TodoList}} keeps
// the pre-cache behaviour: re-rendered before every call, and no turn context
// block, so its own conditionals around those fields stay true and the model is
// not handed two clocks.
func TestVolatileCustomTemplateKeepsThePerStepRefresh(t *testing.T) {
	tmp := t.TempDir()
	promptsDir := t.TempDir()
	body := "You are FoxxyCode.\n\n{{if .TodoList}}## Checklist\n\n{{.TodoList}}\n{{end}}\nNow: {{.UTCNow}}\n"
	if err := os.WriteFile(filepath.Join(promptsDir, "agent.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.Paths{CWD: tmp}}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Prompts.Dir = promptsDir
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	a := NewAgent(cfg, st, nil, nil)

	sys := a.buildSystemPromptParts("agent", nil, nil, "", nil)
	if !sys.Volatile {
		t.Fatal("a template printing UTCNow and TodoList must be marked volatile")
	}
	if block := a.buildTurnContext(sys); block != "" {
		t.Fatalf("a volatile template must get no turn context block, got %q", block)
	}

	// The built-in template is the other way round.
	cfg.Prompts.Dir = ""
	builtin := a.buildSystemPromptParts("agent", nil, nil, "", nil)
	if builtin.Volatile {
		t.Fatal("the built-in agent template must not be volatile")
	}
	if block := a.buildTurnContext(builtin); !strings.Contains(block, turnContextOpenTag) {
		t.Fatalf("the built-in template must get a turn context block, got %q", block)
	}
}

// A template with no {{.Rules}} in it asked for no rules at all. A rule a tool
// call activates must not be smuggled in after the history either.
func TestTemplateWithoutRulesGetsNoRulesInTheTurnContext(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, ".foxxycode", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ndescription: Go files\nglobs: **/*.go\nalwaysApply: false\n---\n\nNO_RULES_TEMPLATE_TOKEN\n"
	if err := os.WriteFile(filepath.Join(rulesDir, "gofiles.mdc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	promptsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptsDir, "agent.md"), []byte("You are FoxxyCode. {{.CWD}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Paths: config.Paths{CWD: tmp}}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Prompts.Dir = promptsDir
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.ReplaceRulesCatalog(session.DiscoverRules(cfg, tmp))
	a := NewAgent(cfg, st, nil, nil)

	sys := a.buildSystemPromptParts("agent", nil, nil, "", nil)
	a.activateScopedRulesForToolCall("read", `{"path":"main.go"}`, tmp)
	if block := a.buildTurnContext(sys); strings.Contains(block, "NO_RULES_TEMPLATE_TOKEN") {
		t.Fatalf("a template without {{.Rules}} still received a rule: %q", block)
	}
}

// A rule the frozen system prompt already carries must never be repeated after
// the history: that is what rules.Added is for, and repeating it would spend on
// every step exactly the tokens this change is saving.
func TestRuleAlreadyInTheSystemPromptIsNotRepeatedInTheTurnContext(t *testing.T) {
	tmp := t.TempDir()
	rulesDir := filepath.Join(tmp, ".foxxycode", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ndescription: Go files\nglobs: **/*.go\nalwaysApply: true\n---\n\nALREADY_SENT_RULE_TOKEN\n"
	if err := os.WriteFile(filepath.Join(rulesDir, "gofiles.mdc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.go", "other.go"} {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{Paths: config.Paths{CWD: tmp}}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	st.ReplaceRulesCatalog(session.DiscoverRules(cfg, tmp))
	a := NewAgent(cfg, st, nil, nil)

	// An attachment already made the rule sticky, so the frozen prompt carries it.
	sys := a.buildSystemPromptParts("agent", nil, nil, "", []string{filepath.Join(tmp, "main.go")})
	if !strings.Contains(sys.Content, "ALREADY_SENT_RULE_TOKEN") {
		t.Fatal("the attached file did not activate the glob rule")
	}
	a.activateScopedRulesForToolCall("read", `{"path":"other.go"}`, tmp)
	if block := a.buildTurnContext(sys); strings.Contains(block, "ALREADY_SENT_RULE_TOKEN") {
		t.Fatalf("a rule the system prompt already carried was repeated after the history: %q", block)
	}
}

// The lane re-issues a step that produced nothing, and that replay must be the
// request that failed. A clock ticking between the two would make it a
// different request and miss the cache the first attempt just populated.
func TestTurnClockDoesNotTickBetweenTheStepsOfATurn(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	st := &session.State{ID: "t", CWD: tmp, Mode: session.ModeAgent}
	a := NewAgent(cfg, st, nil, nil)

	ticks := 0
	a.clock = func() time.Time {
		ticks++
		return time.Date(2038, 1, 19, 3, 14, 7+ticks, 0, time.UTC)
	}

	sys := a.buildSystemPromptParts("agent", nil, nil, "", nil)
	first := a.buildTurnContext(sys)
	second := a.buildTurnContext(sys)
	if first != second {
		t.Fatalf("the turn context clock moved between two steps of one turn:\n%q\n%q", first, second)
	}
	if !strings.Contains(first, "2038-01-19T03:14:08Z") {
		t.Fatalf("the block does not carry the turn's own stamp: %q", first)
	}
}

// --- Session filing (session_describe) -------------------------------------

func TestApplySessionFilingRefusesATitleTooLongForARowAndWritesNothing(t *testing.T) {
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.SetTitlePinned("A short title")
	st.SetTags([]string{"api"})

	long := strings.Repeat("x", session.MaxSessionTitleRunes+1)
	tags := []string{"backend"}
	if _, err := applySessionFiling(st, tooling.SessionFilingUpdate{Title: &long, Tags: &tags}); err == nil {
		t.Fatal("a title longer than a list row was accepted")
	}
	if got := st.ConversationTitle(); got != "A short title" {
		t.Fatalf("the refused call still renamed the session to %q", got)
	}
	if got := st.GetTags(); !reflect.DeepEqual(got, []string{"api"}) {
		t.Fatalf("the refused call still filed the session under %v", got)
	}
}

func TestApplySessionFilingClearsThePinOnAnEmptyTitle(t *testing.T) {
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "fix the failing test"})
	st.SetTitlePinned("Pinned by the model")

	empty := "   "
	filing, err := applySessionFiling(st, tooling.SessionFilingUpdate{Title: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if filing.Filing.Title == "Pinned by the model" || filing.Filing.Title == "" {
		t.Fatalf("clearing the pin left the title %q, want the one derived from the first message", filing.Filing.Title)
	}
}

func TestApplySessionFilingEditsTheTagsInPlace(t *testing.T) {
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.SetTags([]string{"api", "backend"})

	filing, err := applySessionFiling(st, tooling.SessionFilingUpdate{
		AddTags:    []string{"Session Store"},
		RemoveTags: []string{"API"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(filing.Filing.Tags, []string{"backend", "session-store"}) {
		t.Fatalf("got %v", filing.Filing.Tags)
	}
}

func TestApplySessionFilingClearsTheTagsOnAnEmptyList(t *testing.T) {
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.SetTags([]string{"api"})

	none := []string{}
	filing, err := applySessionFiling(st, tooling.SessionFilingUpdate{Tags: &none})
	if err != nil {
		t.Fatal(err)
	}
	if len(filing.Filing.Tags) != 0 {
		t.Fatalf("got %v, want no tags", filing.Filing.Tags)
	}
}

// http_request sends whatever the model asks to whatever address it names, so
// the owner kept it to the two modes that already change things. plan, docs and
// ask read the web through websearch and webfetch, which refuse a private
// address and cannot be shaped into an upload.
func TestHTTPRequestIsOnlyOfferedToAgentAndDebug(t *testing.T) {
	for _, mode := range []string{"plan", "docs", "ask"} {
		if ToolSetForMode(mode, false).Allows(toolweb.ToolHTTPRequest) {
			t.Errorf("%s mode is offered http_request", mode)
		}
	}
	for _, mode := range []string{"agent", "debug"} {
		if !ToolSetForMode(mode, false).Unrestricted() {
			t.Errorf("%s mode is no longer unrestricted", mode)
		}
	}
	// ask refuses a hidden call at execution time too, which is what holds when
	// a model echoes one out of history recorded in agent mode.
	if _, refused := toolCallRefusedByMode("ask", toolweb.ToolHTTPRequest, false); !refused {
		t.Error("ask mode would run an http_request echoed from history")
	}
}

func TestSessionDescribeIsOfferedInEveryModeOfTheFork(t *testing.T) {
	// Filing writes the session's own title and tags and nothing in the
	// workspace. Upstream offers it to agent and plan; the fork's owner decided
	// that a docs or an ask session files itself too, and debug is unrestricted
	// like agent.
	for _, mode := range []string{"plan", "docs", "ask"} {
		if !ToolSetForMode(mode, false).Allows(tools.ToolSessionDescribe) {
			t.Errorf("%s mode cannot file its own session", mode)
		}
		if _, refused := toolCallRefusedByMode(mode, tools.ToolSessionDescribe, false); refused {
			t.Errorf("%s mode refuses session_describe at execution time", mode)
		}
	}
	for _, mode := range []string{"agent", "debug"} {
		if !ToolSetForMode(mode, false).Unrestricted() {
			t.Errorf("%s mode is no longer unrestricted", mode)
		}
	}
	// The plan guard that withholds plan_exit must not take filing with it.
	if !ToolSetForMode("plan", true).Allows(tools.ToolSessionDescribe) {
		t.Error("plan mode under plan_no_self_run lost session_describe")
	}
}

func TestApplySessionFilingReportsAClearedPinBehindTheSameWords(t *testing.T) {
	// The pinned title and the derived one can read alike; clearing the pin is
	// still a change, and a report built by comparing the effective title
	// before and after would call it nothing.
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "Fix the failing test"})
	derived := st.ConversationTitle()
	st.SetTitlePinned(derived)

	empty := ""
	result, err := applySessionFiling(st, tooling.SessionFilingUpdate{Title: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Changed, []string{"title"}) {
		t.Fatalf("changed = %v, want the title", result.Changed)
	}
	if st.GetTitlePinned() != "" {
		t.Fatalf("the pin survived: %q", st.GetTitlePinned())
	}
}

func TestApplySessionFilingReportsNothingWhenTheCallNamesNothing(t *testing.T) {
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.SetTitlePinned("A title")
	st.SetTags([]string{"api"})

	result, err := applySessionFiling(st, tooling.SessionFilingUpdate{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changed) != 0 {
		t.Fatalf("a read reported %v as changed", result.Changed)
	}
	if result.Filing.Title != "A title" || !reflect.DeepEqual(result.Filing.Tags, []string{"api"}) {
		t.Fatalf("a read reported %+v", result.Filing)
	}
}

func TestUpdateTagsKeepsWhatAnotherWriterFiledMeanwhile(t *testing.T) {
	// The point of add_tags is "keep the rest". Merging outside the session
	// would drop whatever another surface filed between the read and the write,
	// so the merge happens under the session's own lock - which is what makes
	// eight concurrent additions end up with eight labels.
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			st.UpdateTags([]string{fmt.Sprintf("tag-%d", n)}, nil)
		}(i)
	}
	wg.Wait()
	if got := st.GetTags(); len(got) != 8 {
		t.Fatalf("concurrent additions left %v", got)
	}
}

func TestSetTitlePinnedIfUnsetHasOneWinner(t *testing.T) {
	st := &session.State{ID: "sess_filing", CWD: t.TempDir(), Mode: session.ModeAgent}
	var wg sync.WaitGroup
	var wins atomic.Int64
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if _, written := st.SetTitlePinnedIfUnset(fmt.Sprintf("name %d", n)); written {
				wins.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d callers named the session", wins.Load())
	}
	if st.GetTitlePinned() == "" {
		t.Fatal("nobody named it")
	}
}
