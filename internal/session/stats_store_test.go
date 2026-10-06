package session

import (
	"testing"
	"time"
)

func TestApplyTurnUsageAccumulatesAcrossTurns(t *testing.T) {
	at := time.Unix(0, 0).UTC()
	first := ApplyTurnUsage(nil, 0, TokenUsageTotals{InputTokens: 100, OutputTokens: 20}, at)
	if got := first.TokenUsageTotal; got.InputTokens != 100 || got.OutputTokens != 20 || got.TotalTokens != 120 {
		t.Fatalf("first turn totals = %+v", got)
	}

	second := ApplyTurnUsage(&first, 1, TokenUsageTotals{InputTokens: 300, OutputTokens: 50}, at)
	if got := second.TokenUsageTotal; got.InputTokens != 400 || got.OutputTokens != 70 || got.TotalTokens != 470 {
		t.Fatalf("second turn totals = %+v, want the session sum", got)
	}
	if len(second.TokenUsageByTurn) != 2 {
		t.Fatalf("per-turn rows = %+v, want one per turn", second.TokenUsageByTurn)
	}
}

func TestApplyTurnUsageIsIdempotentWithinOneTurn(t *testing.T) {
	at := time.Unix(0, 0).UTC()
	prior := ApplyTurnUsage(nil, 0, TokenUsageTotals{InputTokens: 100, OutputTokens: 20}, at)

	// The ReAct loop rewrites the running turn every 750ms with counters that already
	// include everything the turn spent, so a rewrite must replace, never add.
	mid := ApplyTurnUsage(&prior, 1, TokenUsageTotals{InputTokens: 200, OutputTokens: 10}, at)
	end := ApplyTurnUsage(&mid, 1, TokenUsageTotals{InputTokens: 500, OutputTokens: 40}, at)

	if got := end.TokenUsageTotal; got.InputTokens != 600 || got.OutputTokens != 60 || got.TotalTokens != 660 {
		t.Fatalf("totals after a mid-turn rewrite = %+v", got)
	}
	if len(end.TokenUsageByTurn) != 2 {
		t.Fatalf("a rewrite appended a row: %+v", end.TokenUsageByTurn)
	}
}

func TestApplyTurnUsageTrimsHistoryWithoutLosingTheTotal(t *testing.T) {
	at := time.Unix(0, 0).UTC()
	stats := SessionStats{}
	want := 0
	for i := 0; i < maxStoredTurns+10; i++ {
		stats = ApplyTurnUsage(&stats, i, TokenUsageTotals{InputTokens: 2, OutputTokens: 1}, at)
		want += 3
	}
	if len(stats.TokenUsageByTurn) != maxStoredTurns {
		t.Fatalf("history length = %d, want it capped at %d", len(stats.TokenUsageByTurn), maxStoredTurns)
	}
	if stats.TokenUsageTotal.TotalTokens != want {
		t.Fatalf("total = %d, want %d: trimmed turns must stay counted", stats.TokenUsageTotal.TotalTokens, want)
	}
	if first := stats.TokenUsageByTurn[0].TurnIndex; first != 10 {
		t.Fatalf("oldest kept turn = %d, want the newest %d rows", first, maxStoredTurns)
	}
}

// The token write and the compaction write share stats.json. ApplyTurnUsage owns the
// counters only; whatever breakdown the document already carries has to survive.
func TestApplyTurnUsageKeepsThePriorContextBreakdown(t *testing.T) {
	at := time.Unix(0, 0).UTC()
	prior := SessionStats{ContextBreakdown: &ContextBreakdown{Conversation: 300, EstimatedTotal: 400}}
	got := ApplyTurnUsage(&prior, 0, TokenUsageTotals{InputTokens: 5, OutputTokens: 5}, at)
	if got.ContextBreakdown == nil || got.ContextBreakdown.EstimatedTotal != 400 {
		t.Fatalf("context breakdown = %+v", got.ContextBreakdown)
	}
}

func TestWriteSessionContextBreakdownPreservesTokenTotals(t *testing.T) {
	dir := t.TempDir()
	before := SessionStats{
		TokenUsageTotal: TokenUsageTotals{
			InputTokens:  123,
			OutputTokens: 45,
			TotalTokens:  168,
		},
		TokenUsageByTurn: []TokenUsageTurn{{
			TurnIndex:    2,
			InputTokens:  123,
			OutputTokens: 45,
			TotalTokens:  168,
		}},
		ContextBreakdown: &ContextBreakdown{
			SystemPrompt:   100,
			Conversation:   900,
			EstimatedTotal: 1000,
		},
	}
	if err := WriteSessionStats(dir, before); err != nil {
		t.Fatal(err)
	}

	afterBreakdown := &ContextBreakdown{
		SystemPrompt:   100,
		Conversation:   300,
		EstimatedTotal: 400,
	}
	if err := WriteSessionContextBreakdown(dir, afterBreakdown); err != nil {
		t.Fatal(err)
	}

	got, err := ReadSessionStats(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.TokenUsageTotal != before.TokenUsageTotal {
		t.Fatalf("token totals changed: got %+v, want %+v", got.TokenUsageTotal, before.TokenUsageTotal)
	}
	if len(got.TokenUsageByTurn) != 1 || got.TokenUsageByTurn[0].TurnIndex != 2 {
		t.Fatalf("per-turn usage changed: %+v", got.TokenUsageByTurn)
	}
	if got.ContextBreakdown == nil || got.ContextBreakdown.EstimatedTotal != 400 {
		t.Fatalf("context breakdown not updated: %+v", got.ContextBreakdown)
	}
}

func TestWriteSessionContextBreakdownCreatesMissingStatsFile(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSessionContextBreakdown(dir, &ContextBreakdown{Conversation: 12, EstimatedTotal: 12}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSessionStats(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextBreakdown == nil || got.ContextBreakdown.EstimatedTotal != 12 {
		t.Fatalf("context breakdown = %+v", got.ContextBreakdown)
	}
	if got.Version == 0 {
		t.Fatal("stats version not stamped")
	}
}
