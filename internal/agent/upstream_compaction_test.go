package agent

import (
	"context"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
	"strings"
	"testing"
)

func TestMaybeAutoCompactUsesProviderInputAboveEstimate(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	provider := &compactCannedProvider{t: t, summary: "provider-sized summary"}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)
	ag.cfg.Models[0].MaxContextTokens = 100
	st.SetLastContextBreakdown(&session.ContextBreakdown{EstimatedTotal: 30})
	ag.recordProviderInputTokens(85, 30)
	// The next step added just a little text; the local estimate remains well
	// below the threshold even though the provider measured a larger prompt.
	st.SetLastContextBreakdown(&session.ContextBreakdown{
		EstimatedTotal: 35, ProviderInputTokens: 85, ProviderEstimateTokens: 30,
	})
	if !ag.maybeAutoCompact(context.Background()) {
		t.Fatal("provider input plus the new text must trigger compaction")
	}
	if len(provider.requests) != 1 {
		t.Fatalf("summarizer calls = %d, want 1", len(provider.requests))
	}
	if b := st.GetLastContextBreakdown(); b == nil || b.ProviderInputTokens != 0 {
		t.Fatalf("provider baseline survived compaction: %+v", b)
	}
}

func TestProviderAnchorSurvivesResponseWithoutUsage(t *testing.T) {
	st := seededCompactState(t, 3)
	keep := 1
	provider := &compactCannedProvider{t: t, summary: "s"}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep}, provider)
	ag.cfg.Models[0].MaxContextTokens = 100
	st.SetLastContextBreakdown(&session.ContextBreakdown{EstimatedTotal: 30})
	ag.recordProviderInputTokens(85, 30)
	ag.recordProviderInputTokens(0, 31)
	b := st.GetLastContextBreakdown()
	if b == nil || b.ProviderInputTokens != 85 || b.ProviderEstimateTokens != 30 {
		t.Fatalf("zero-usage response dropped the provider anchor: %+v", b)
	}
}

func TestManualCompactionRemainsAvailableWhenAutomationIsOff(t *testing.T) {
	st := seededCompactState(t, 3)
	keep, off := 1, false
	provider := &compactCannedProvider{t: t, summary: "manual summary"}
	ag := compactTestAgent(t, st, config.CompactionConfig{KeepRecentTurns: &keep, AutoEnabled: &off}, provider)
	ag.cfg.Models[0].MaxContextTokens = 100
	st.SetLastContextBreakdown(&session.ContextBreakdown{EstimatedTotal: 90})
	if ag.maybeAutoCompact(context.Background()) {
		t.Fatal("automatic trigger ran with auto_enable: false")
	}
	if _, err := ag.CompactSession(context.Background(), CompactOptions{Force: true}); err != nil {
		t.Fatalf("manual compaction with auto_enable: false: %v", err)
	}
}

func TestContextEstimateIncludesToolArgumentsCyrillicReasoningImagesAndSchemas(t *testing.T) {
	content := strings.Repeat("Привет, это содержимое файла. ", 1000)
	msgs := []llm.Message{{
		Role: llm.RoleAssistant, Reasoning: content,
		ToolCalls:  []llm.ToolCall{{Name: "write_file", InputJSON: `{"content":"` + content + `"}`}},
		ImageParts: []llm.ImagePart{{DataURL: "data:image/png;base64,AAAA"}},
	}}
	defs := []llm.ToolDefinition{{Name: "write_file", InputSchema: map[string]interface{}{
		"properties": map[string]interface{}{"content": map[string]interface{}{"description": content}},
	}}}
	b := computeContextBreakdown("system", "", "", "", msgs, true, defs)
	if b.Conversation < 2*session.EstimateContextTokens(content)+1024 {
		t.Fatalf("conversation estimate misses the write or reasoning: %+v", b)
	}
	if b.ToolDefinitions < session.EstimateContextTokens(content) {
		t.Fatalf("tool schema missing: %+v", b)
	}
}
