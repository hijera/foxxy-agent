package agent

import (
	"context"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// titleModeProvider answers a turn with plain text and records how it was called.
// Stream serves the ReAct turn; Complete serves the title pass that follows it,
// so one stub covers both halves of the exchange.
type titleModeProvider struct {
	answer string
	title  string
}

func (p *titleModeProvider) Complete(context.Context, []llm.Message, []llm.ToolDefinition) (*llm.Response, error) {
	return &llm.Response{Content: p.title, StopReason: "end_turn"}, nil
}

func (p *titleModeProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, onChunk func(llm.StreamChunk)) (*llm.Response, error) {
	onChunk(llm.StreamChunk{TextDelta: p.answer})
	return &llm.Response{Content: p.answer, StopReason: "end_turn"}, nil
}

// TestRunGeneratesSessionTitleInEveryMode pins that the title pass is reached by a
// normal turn in all four session modes. maybeGenerateTitle itself has no mode
// check, but it hangs off the ReAct loop, and the loop's tool sets, prompts, and
// stop conditions differ per mode - so the guarantee is only worth anything when
// exercised through Agent.Run rather than by calling the generator directly.
func TestRunGeneratesSessionTitleInEveryMode(t *testing.T) {
	for _, mode := range []session.Mode{
		session.ModeAgent,
		session.ModePlan,
		session.ModeDocs,
		session.ModeAsk,
	} {
		t.Run(string(mode), func(t *testing.T) {
			cfg := titleConfig(t)
			st := &session.State{ID: "title_" + string(mode), CWD: t.TempDir(), Mode: mode}
			sender := &titleSender{}
			ag := NewAgent(cfg, st, sender, nil)
			provider := &titleModeProvider{answer: "here is the answer", title: "Postgres connection help"}
			ag.providerFactory = func(llm.ProviderInput) (llm.Provider, error) { return provider, nil }

			stop, err := ag.Run(context.Background(), []acp.ContentBlock{
				{Type: "text", Text: "how do I connect postgres to my API"},
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if stop != string(acp.StopReasonEndTurn) {
				t.Fatalf("stop reason = %q, want end_turn", stop)
			}

			// The title pass runs in its own goroutine off the hot path, so the
			// turn returns before it lands.
			deadline := time.Now().Add(5 * time.Second)
			for st.GetTitleAuto() == "" && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			if got := st.GetTitleAuto(); got != "Postgres connection help" {
				t.Fatalf("%s mode: TitleAuto = %q, want the generated title", mode, got)
			}
			if got := sender.last(); got != "Postgres connection help" {
				t.Fatalf("%s mode: broadcast title = %q, want the generated title", mode, got)
			}
		})
	}
}
