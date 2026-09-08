package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mcp"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/skills"
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
	last := provider.seen[len(provider.seen)-1]
	if last.Role != llm.RoleTool || last.ToolCallID != "call_blocked" || last.Content != "permission denied by user" {
		t.Fatalf("provider did not receive denied tool result as latest message: %+v", last)
	}
	if got := st.GetMessages()[len(st.GetMessages())-1]; got.Role != llm.RoleAssistant || got.Content != "continued" {
		t.Fatalf("missing continuation assistant message: %+v", got)
	}
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
