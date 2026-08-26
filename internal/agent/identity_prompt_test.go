package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// Every system prompt FoxxyCode sends has to name the product near its start:
// a gateway in front of the model reads only the opening of the prompt to
// tell one client from another. See internal/prompts/identity.go.
const identityWindowChars = 220

func assertPromptIdentifiesFoxxyCode(t *testing.T, prompt, what string) {
	t.Helper()
	head := prompt
	if len(head) > identityWindowChars {
		head = head[:identityWindowChars]
	}
	if !strings.Contains(strings.ToLower(head), "you are foxxycode") {
		t.Errorf("%s does not identify FoxxyCode within the first %d characters:\n%s",
			what, identityWindowChars, head)
	}
}

func identityAgent(t *testing.T, cwd string) *Agent {
	t.Helper()
	st := &session.State{ID: "identity", CWD: cwd, Mode: session.ModeAgent}
	cfg := &config.Config{}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	return NewAgent(cfg, st, nil, nil)
}

func TestBuildSystemPromptIdentifiesFoxxyCodeInEveryMode(t *testing.T) {
	a := identityAgent(t, t.TempDir())
	for _, mode := range []string{"agent", "plan"} {
		assertPromptIdentifiesFoxxyCode(t, a.buildSystemPrompt(mode, nil, nil, "", nil), mode+" mode prompt")
	}
}

// prompts.dir replaces the built-in template wholesale. Users who bring their
// own persona must still be attributable, so the identity line is prepended.
func TestBuildSystemPromptIdentifiesFoxxyCodeWithCustomPromptsDir(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"agent.md", "plan.md"} {
		body := "You are a terse assistant. Working directory: {{.CWD}}\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := identityAgent(t, t.TempDir())
	a.cfg.Prompts.Dir = dir

	for _, mode := range []string{"agent", "plan"} {
		prompt := a.buildSystemPrompt(mode, nil, nil, "", nil)
		assertPromptIdentifiesFoxxyCode(t, prompt, "custom "+mode+" prompt")
		if !strings.Contains(prompt, "You are a terse assistant.") {
			t.Errorf("custom %s template body was dropped:\n%s", mode, prompt)
		}
	}
}

// A broken prompts.dir falls back to a generic stub with no product name.
func TestBuildSystemPromptIdentifiesFoxxyCodeOnRenderFallback(t *testing.T) {
	a := identityAgent(t, t.TempDir())
	a.cfg.Prompts.Dir = filepath.Join(t.TempDir(), "missing")

	assertPromptIdentifiesFoxxyCode(t, a.buildSystemPrompt("agent", nil, nil, "", nil), "fallback prompt")
}

// The summarizer runs as its own request with its own system prompt, so it is
// a separate client from the gateway's point of view.
func TestCompactionRequestIdentifiesFoxxyCode(t *testing.T) {
	msgs := buildCompactionRequest([]llm.Message{{Role: llm.RoleUser, Content: "hi"}}, "")
	if len(msgs) == 0 || msgs[0].Role != llm.RoleSystem {
		t.Fatalf("expected a leading system message, got %+v", msgs)
	}
	assertPromptIdentifiesFoxxyCode(t, msgs[0].Content, "compaction prompt")
	if !strings.Contains(msgs[0].Content, "You are compacting the conversation history") {
		t.Errorf("compaction instructions were lost:\n%s", msgs[0].Content)
	}
}

// No prompt may carry the line twice — that is pure token waste on every turn.
func TestIdentityLineAppearsOnce(t *testing.T) {
	a := identityAgent(t, t.TempDir())
	for _, mode := range []string{"agent", "plan"} {
		prompt := strings.ToLower(a.buildSystemPrompt(mode, nil, nil, "", nil))
		if n := strings.Count(prompt, "you are foxxycode"); n != 1 {
			t.Errorf("%s mode prompt names FoxxyCode %d times, want 1", mode, n)
		}
	}
}
