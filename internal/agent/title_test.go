package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// titleSender captures SessionTitleUpdate events for assertions.
type titleSender struct {
	resumePermissionSender
	mu     sync.Mutex
	titles []string
}

func (s *titleSender) SendSessionUpdate(_ string, update interface{}) error {
	if u, ok := update.(acp.SessionTitleUpdate); ok {
		s.mu.Lock()
		s.titles = append(s.titles, u.Title)
		s.mu.Unlock()
	}
	return nil
}

func (s *titleSender) last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.titles) == 0 {
		return ""
	}
	return s.titles[len(s.titles)-1]
}

func titleConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "p", Type: "openai", APIKey: "k"}},
		Models:    []config.ModelEntry{{Model: "p/m", MaxTokens: 100, MaxContextTokens: 1000, Temperature: 0.1}},
	}
	cfg.Agent.Model = "p/m"
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	cfg.Title.ApplyDefaults() // Enabled=true, MaxTokens=64
	return cfg
}

func firstExchange() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleUser, Content: "how do I connect postgres to my API"},
		{Role: llm.RoleAssistant, Content: "You can use a connection pool..."},
	}
}

func TestMaybeGenerateTitleGeneratesAndPersists(t *testing.T) {
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(firstExchange())
	prov := &summarizeProvider{summary: "Postgres API connection"}
	sender := &titleSender{}
	a := NewAgent(cfg, st, sender, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 1 {
		t.Fatalf("title provider called %d times, want 1", prov.calls)
	}
	if got := st.GetTitleAuto(); got != "Postgres API connection" {
		t.Fatalf("TitleAuto = %q, want %q", got, "Postgres API connection")
	}
	if got := sender.last(); got != "Postgres API connection" {
		t.Fatalf("broadcast title = %q, want %q", got, "Postgres API connection")
	}
}

func TestMaybeGenerateTitleSkipsWhenPinned(t *testing.T) {
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(firstExchange())
	st.SetTitlePinnedWithoutPersist("User pinned")
	prov := &summarizeProvider{summary: "X"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 0 {
		t.Fatal("must not generate a title when one is pinned")
	}
	if st.GetTitleAuto() != "" {
		t.Fatalf("TitleAuto should stay empty, got %q", st.GetTitleAuto())
	}
}

func TestMaybeGenerateTitleSkipsWhenAlreadyGenerated(t *testing.T) {
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(firstExchange())
	st.SetTitleAutoWithoutPersist("Existing auto title")
	prov := &summarizeProvider{summary: "New"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 0 {
		t.Fatal("must generate a title at most once")
	}
	if st.GetTitleAuto() != "Existing auto title" {
		t.Fatalf("TitleAuto changed to %q", st.GetTitleAuto())
	}
}

func TestMaybeGenerateTitleFromFirstUserMessageAlone(t *testing.T) {
	// Titling depends only on the first user message, so a tool-only first assistant turn (or no
	// assistant text yet) must still produce a title.
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist([]llm.Message{{Role: llm.RoleUser, Content: "add rate limiting to the API"}})
	prov := &summarizeProvider{summary: "Rate limiting for API"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
	if st.GetTitleAuto() != "Rate limiting for API" {
		t.Fatalf("TitleAuto = %q", st.GetTitleAuto())
	}
}

func TestMaybeGenerateTitleSkipsWithoutUserMessage(t *testing.T) {
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist([]llm.Message{{Role: llm.RoleAssistant, Content: "hello"}})
	prov := &summarizeProvider{summary: "X"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 0 {
		t.Fatal("must not title without a user message")
	}
}

func TestMaybeGenerateTitleDisabled(t *testing.T) {
	cfg := titleConfig(t)
	off := false
	cfg.Title.Enabled = &off
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(firstExchange())
	prov := &summarizeProvider{summary: "X"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 0 {
		t.Fatal("must not generate a title when disabled")
	}
}

func TestCleanTitleStripsThinkAndClamps(t *testing.T) {
	got := cleanTitle("<think>let me think</think>\n\n\"Rate limiting implementation\"\n")
	if got != "Rate limiting implementation" {
		t.Fatalf("cleanTitle = %q", got)
	}

	long := cleanTitle(strings.Repeat("a", 200))
	if r := []rune(long); len(r) > titleMaxRunes {
		t.Fatalf("clamped title too long: %d runes", len(r))
	}
}

func TestMaybeGenerateTitleStripsSessionAssets(t *testing.T) {
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist([]llm.Message{
		{Role: llm.RoleUser, Content: "review this <foxxycode_session_assets>a.png</foxxycode_session_assets> file"},
		{Role: llm.RoleAssistant, Content: "sure"},
	})
	prov := &summarizeProvider{summary: "Config review"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
	// The assets XML must not appear in the message sent to the title model.
	for _, m := range prov.seen {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, "<foxxycode_session_assets>") {
			t.Fatalf("assets XML leaked to title model: %q", m.Content)
		}
	}
}

func TestMaybeGenerateTitleStripsInjectedIDEAndTerminalContext(t *testing.T) {
	// The always-on <foxxycode_ide_context> / <foxxycode_terminal_context> environment blocks and
	// attribute-bearing <foxxycode_terminal_output name="..."> mentions are appended to the stored
	// user message each turn. None of them must leak into the title-model prompt.
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	content := "тест\n\n<foxxycode_ide_context>\n# Active File\nsites/all/modules/foo.php\n</foxxycode_ide_context>" +
		"\n\n<foxxycode_terminal_context>\nnpm run dev\n</foxxycode_terminal_context>" +
		"\n\n<foxxycode_terminal_output name=\"zsh\">\n$ ls\n</foxxycode_terminal_output>"
	st.ReplaceMessagesWithoutPersist([]llm.Message{
		{Role: llm.RoleUser, Content: content},
		{Role: llm.RoleAssistant, Content: "sure"},
	})
	prov := &summarizeProvider{summary: "Some title"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if prov.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", prov.calls)
	}
	for _, m := range prov.seen {
		if m.Role != llm.RoleUser {
			continue
		}
		for _, tag := range []string{"<foxxycode_ide_context>", "<foxxycode_terminal_context>", "<foxxycode_terminal_output"} {
			if strings.Contains(m.Content, tag) {
				t.Fatalf("injected block %q leaked to title model: %q", tag, m.Content)
			}
		}
		if !strings.Contains(m.Content, "тест") {
			t.Fatalf("user text dropped from title prompt: %q", m.Content)
		}
	}
}

func TestMaybeGenerateTitleUsesMainProviderWhenNoTitleModel(t *testing.T) {
	// With title.model unset, the title pass must run on the same provider the chat request uses
	// (the one passed into maybeGenerateTitle), not skip generation.
	cfg := titleConfig(t)
	if cfg.Title.Model != "" {
		t.Fatalf("precondition: title model must be empty, got %q", cfg.Title.Model)
	}
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(firstExchange())
	main := &summarizeProvider{summary: "Main-model title"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), main)

	if main.calls != 1 {
		t.Fatalf("main provider called %d times, want 1 (fallback to request model)", main.calls)
	}
	if got := st.GetTitleAuto(); got != "Main-model title" {
		t.Fatalf("TitleAuto = %q, want title from the main provider", got)
	}
}
