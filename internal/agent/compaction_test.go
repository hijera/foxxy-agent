package agent

import (
	"context"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// summarizeProvider is a fake llm.Provider whose Complete returns a canned summary and records
// the messages it was asked to summarize.
type summarizeProvider struct {
	summary string
	seen    []llm.Message
	calls   int
}

func (p *summarizeProvider) Complete(_ context.Context, msgs []llm.Message, _ []llm.ToolDefinition) (*llm.Response, error) {
	p.calls++
	p.seen = append([]llm.Message(nil), msgs...)
	return &llm.Response{Content: p.summary, StopReason: "end_turn"}, nil
}

func (p *summarizeProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.ToolDefinition, _ func(llm.StreamChunk)) (*llm.Response, error) {
	return &llm.Response{StopReason: "end_turn"}, nil
}

func TestIsLLMHistoryMessageExcludesCompacted(t *testing.T) {
	if isLLMHistoryMessage(llm.Message{Role: llm.RoleUser, Content: "x", Compacted: true}) {
		t.Error("compacted message must be excluded from LLM payload")
	}
	if !isLLMHistoryMessage(llm.Message{Role: llm.RoleUser, Content: "s", CompactionSummary: true}) {
		t.Error("compaction summary must be kept in LLM payload")
	}
}
