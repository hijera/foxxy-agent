package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// countingProvider records how many requests it was handed, safely — the whole
// point of the check below is that a second, concurrent caller may exist.
type countingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingProvider) bump() {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *countingProvider) Complete(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	p.bump()
	return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
}

func (p *countingProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	p.bump()
	onChunk(llm.StreamChunk{TextDelta: "done"})
	return &llm.Response{Content: "done", StopReason: "end_turn"}, nil
}

func titlePassConfig(t *testing.T, disable bool) *config.Config {
	t.Helper()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:    []config.ModelEntry{{Model: "fake/model", MaxTokens: 100, MaxContextTokens: 128000}},
		Agent:     config.Agent{Model: "fake/model"},
	}
	cfg.Agent.ApplyDefaults()
	cfg.Prompts.ApplyDefaults()
	if disable {
		disableTitlePass(cfg)
	}
	return cfg
}

func runOneTurn(t *testing.T, cfg *config.Config) *countingProvider {
	t.Helper()
	provider := &countingProvider{}
	dir := t.TempDir()
	st := &session.State{ID: "sess_titlepass", CWD: dir, Mode: session.ModeAgent, SessionDir: dir}
	ag := NewAgent(cfg, st, resumePermissionSender{}, nil)
	ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }
	if _, err := ag.Run(context.Background(), []acp.ContentBlock{{Type: "text", Text: "probe"}}); err != nil {
		t.Fatalf("Run(): %v", err)
	}
	return provider
}

// The title pass is detached on purpose, so a scenario's stub keeps being called
// after the turn returns. That is the mechanism behind the data races the race
// detector reports in the agent BDD suites, and disableTitlePass is what removes
// it — this pins that it actually does.
func TestDisableTitlePassStopsTheDetachedProviderCall(t *testing.T) {
	provider := runOneTurn(t, titlePassConfig(t, true))

	// Generous: the detached goroutine starts during the turn, so if it were still
	// enabled it would have landed well inside this window.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := provider.count(); got > 1 {
			t.Fatalf("provider was called %d times; the title pass is still running", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("provider calls = %d, want exactly the turn's own request", got)
	}
}

// The counterpart: with the pass left on, the same turn produces a second,
// detached call. Without this the check above would also pass if the agent simply
// stopped generating titles altogether.
func TestTitlePassStillRunsWhenEnabled(t *testing.T) {
	provider := runOneTurn(t, titlePassConfig(t, false))

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if provider.count() > 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("provider calls = %d, want a second call from the title pass", provider.count())
}
