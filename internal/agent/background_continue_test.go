package agent

import (
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestBackgroundContinuationPolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta *session.SubagentMeta
		want int
	}{
		{"interactive", nil, 3},
		{"delegated", &session.SubagentMeta{Name: "general"}, 5},
		{"scheduled", &session.SubagentMeta{Name: "job", Scheduler: &session.SchedulerRunMeta{JobID: "job"}}, 5},
		{"memory", &session.SubagentMeta{Name: "memory", Kind: session.SubagentKindMemory}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{cfg: &config.Config{}, subagent: tc.meta}
			if got := a.continuationBudget(); got != tc.want {
				t.Fatalf("budget = %d, want %d", got, tc.want)
			}
			if tc.want == 5 {
				for n, seconds := range []int{5, 20, 80, 120, 120, 120} {
					if got := a.continuationPause(n, false, nil); got != time.Duration(seconds)*time.Second {
						t.Fatalf("pause %d = %s", n, got)
					}
				}
			}
		})
	}
}

func TestBackgroundContinuationExplicitZero(t *testing.T) {
	zero := 0
	a := &Agent{cfg: &config.Config{Agent: config.Agent{LLMContinueBackgroundMax: &zero}}, subagent: &session.SubagentMeta{Name: "general"}}
	if a.continuationBudget() != 0 {
		t.Fatal("explicit zero must disable background continuations")
	}
}

func TestBackgroundContinuationsDoNotConsumeMaxTurns(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta *session.SubagentMeta
		off  bool
		want int
	}{
		{"delegated", &session.SubagentMeta{Name: "general"}, false, 6},
		{"scheduled", &session.SubagentMeta{Name: "job", Scheduler: &session.SchedulerRunMeta{JobID: "job"}}, false, 6},
		{"interactive", nil, false, 1},
		{"memory", &session.SubagentMeta{Kind: session.SubagentKindMemory}, false, 1},
		{"disabled", &session.SubagentMeta{Name: "general"}, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &stallProvider{script: []stallBehaviour{
				{partial: "First part."}, {partial: "Second part."}, {partial: "Third part."},
				{partial: "Fourth part."}, {partial: "Fifth part."}, {answer: "Finished."},
			}}
			h := newStallHarness(t, provider, func(cfg *config.Agent) {
				zero, enabled := 0, !tc.off
				cfg.MaxTurns = 1
				cfg.LLMContinueMax = &zero
				cfg.LLMContinue = &enabled
				cfg.LLMContinueBackgroundDelaysMS = []int{0}
			})
			h.ag.subagent = tc.meta
			stop, err := h.run(t)
			if tc.want == 6 && (err != nil || stop != string(acp.StopReasonEndTurn)) {
				t.Fatalf("background answer did not finish: stop=%s error=%v", stop, err)
			}
			if got := provider.callCount(); got != tc.want {
				t.Fatalf("calls = %d, want %d", got, tc.want)
			}
		})
	}
}
