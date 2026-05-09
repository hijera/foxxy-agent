package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/EvilFreelancer/coddy-agent/external/memory"
	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	"github.com/EvilFreelancer/coddy-agent/internal/llm"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
)

const persistBodyWireMax = 12_000

func truncatePersistBodyForWire(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= persistBodyWireMax {
		return s
	}
	return strings.TrimSpace(s[:persistBodyWireMax]) + "\n\n…"
}

func countUserTurns(msgs []llm.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Role == llm.RoleUser {
			n++
		}
	}
	return n
}

func (a *Agent) memoryRowID(turn int) string {
	return fmt.Sprintf("mem-%d", turn)
}

func (a *Agent) sendMemoryPhase(rowID, phase, status string, turn int, durationMs int64, persistOutcome *memory.PersistOutcome, recallReadPaths []string) {
	upd := acp.MemoryPhaseUpdate{
		SessionUpdate: acp.UpdateTypeMemoryPhase,
		MemoryRowID:   rowID,
		Phase:         phase,
		Status:        status,
		UserTurnIndex: turn,
		DurationMs:    durationMs,
	}
	if phase == "recall" && status == "completed" && len(recallReadPaths) > 0 {
		upd.RecallReadPaths = recallReadPaths
	}
	if persistOutcome != nil && phase == "persist" && status == "completed" {
		upd.PersistSaved = persistOutcome.Saved
		upd.PersistRelativePath = persistOutcome.RelativePath
		upd.PersistTitle = persistOutcome.Title
		if persistOutcome.Saved && strings.TrimSpace(persistOutcome.Body) != "" {
			upd.PersistSavedBody = truncatePersistBodyForWire(persistOutcome.Body)
		}
	}
	_ = a.server.SendSessionUpdate(a.state.GetID(), upd)
}

func (a *Agent) sendMemoryChunk(rowID, phase, kind, delta string) {
	if delta == "" {
		return
	}
	_ = a.server.SendSessionUpdate(a.state.GetID(), acp.MemoryMessageChunkUpdate{
		SessionUpdate: acp.UpdateTypeMemoryMessageChunk,
		MemoryRowID:   rowID,
		Phase:         phase,
		Kind:          kind,
		Delta:         delta,
	})
}

func (a *Agent) runMemoryRecall(ctx context.Context, userText string) {
	if !a.cfg.Memory.Enabled {
		return
	}
	mr := strings.TrimSpace(a.cfg.Memory.Model)
	if mr == "" {
		mr = a.state.EffectiveModelID(a.cfg)
	}
	turn := countUserTurns(a.state.GetMessages())
	rowID := a.memoryRowID(turn)
	sd := strings.TrimSpace(a.state.GetPersistedSessionDir())

	var recallText, recallReasoning strings.Builder
	opts := &memory.RunRecallOptions{
		OnPhaseStart: func() {
			a.sendMemoryPhase(rowID, "recall", "started", turn, 0, nil, nil)
		},
		OnStream: func(kind memory.StreamKind, delta string) {
			switch kind {
			case memory.StreamKindReasoning:
				recallReasoning.WriteString(delta)
				a.sendMemoryChunk(rowID, "recall", "reasoning", delta)
			default:
				recallText.WriteString(delta)
				a.sendMemoryChunk(rowID, "recall", "text", delta)
			}
		},
	}

	block, dur, recallReadPaths, err := memory.RunRecall(ctx, a.log, a.cfg, a.state.GetCWD(), userText, mr, opts)
	if err != nil {
		a.log.Warn("memory recall", "error", err)
		a.sendMemoryPhase(rowID, "recall", "completed", turn, dur, nil, recallReadPaths)
		return
	}
	a.sendMemoryPhase(rowID, "recall", "completed", turn, dur, nil, recallReadPaths)

	if strings.TrimSpace(block) != "" {
		a.state.SetMemoryCopilotBlock(block)
	}

	rt := strings.TrimSpace(recallText.String())
	if rt == "" && strings.TrimSpace(block) != "" {
		rt = strings.TrimSpace(block)
	}
	if sd != "" {
		_ = session.AppendMemoryTurn(sd, session.MemoryTurnTraceJSON{
			UserTurnIndex:       turn,
			MemoryRowID:         rowID,
			RecallText:          rt,
			RecallReasoningText: strings.TrimSpace(recallReasoning.String()),
			RecallDurationMs:    dur,
			RecallReadPaths:     recallReadPaths,
		})
	}
}

func (a *Agent) runMemoryPersist(ctx context.Context, userText, assistant string) {
	if !a.cfg.Memory.Enabled {
		return
	}
	mr := strings.TrimSpace(a.cfg.Memory.Model)
	if mr == "" {
		mr = a.state.EffectiveModelID(a.cfg)
	}
	turn := countUserTurns(a.state.GetMessages())
	rowID := a.memoryRowID(turn)
	sd := strings.TrimSpace(a.state.GetPersistedSessionDir())

	var judgeText strings.Builder
	opts := &memory.RunPersistOptions{
		OnPhaseStart: func() {
			a.sendMemoryPhase(rowID, "persist", "started", turn, 0, nil, nil)
		},
		OnStream: func(kind memory.StreamKind, delta string) {
			switch kind {
			case memory.StreamKindReasoning:
				a.sendMemoryChunk(rowID, "persist", "reasoning", delta)
			default:
				judgeText.WriteString(delta)
				a.sendMemoryChunk(rowID, "persist", "text", delta)
			}
		},
	}

	outcome, dur, err := memory.RunPersist(ctx, a.log, a.cfg, a.state.GetCWD(), mr, userText, assistant, opts)
	if err != nil {
		a.log.Warn("memory persist", "error", err)
		a.sendMemoryPhase(rowID, "persist", "completed", turn, dur, nil, nil)
		return
	}
	a.sendMemoryPhase(rowID, "persist", "completed", turn, dur, &outcome, nil)

	if sd == "" {
		return
	}
	row := session.MemoryTurnTraceJSON{
		UserTurnIndex:    turn,
		MemoryRowID:      rowID,
		PersistJudgeText: strings.TrimSpace(outcome.RawJudge),
		PersistDurationMs: dur,
		PersistSaved:     outcome.Saved,
		PersistScope:     outcome.Scope,
		PersistRelativePath: outcome.RelativePath,
		PersistTitle:     outcome.Title,
		PersistReason:    outcome.Reason,
		PersistSavedBody: outcome.Body,
	}
	if judgeText.Len() > 0 && row.PersistJudgeText == "" {
		row.PersistJudgeText = strings.TrimSpace(judgeText.String())
	}
	_ = session.AppendMemoryTurn(sd, row)
}
