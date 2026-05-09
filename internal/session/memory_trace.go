package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	memoryTraceFile    = "memory_trace.json"
	memoryTraceVersion = 1
)

// MemoryTurnTraceJSON is one user-turn memory copilot observability payload (persisted outside messages.json).
type MemoryTurnTraceJSON struct {
	UserTurnIndex int `json:"userTurnIndex"`

	MemoryRowID string `json:"memoryRowId"`

	RecallSkipped bool `json:"recallSkipped,omitempty"`

	RecallText          string `json:"recallText,omitempty"`
	RecallReasoningText string `json:"recallReasoningText,omitempty"`
	RecallDurationMs    int64  `json:"recallDurationMs,omitempty"`
	// RecallReadPaths are scope:relative paths read via coddy_memory_read during recall (deduped).
	RecallReadPaths []string `json:"recallReadPaths,omitempty"`

	PersistJudgeText string `json:"persistJudgeText,omitempty"`
	PersistDurationMs int64 `json:"persistDurationMs,omitempty"`
	PersistSaved      bool    `json:"persistSaved,omitempty"`
	PersistScope      string  `json:"persistScope,omitempty"`
	PersistRelativePath string `json:"persistRelativePath,omitempty"`
	PersistTitle      string  `json:"persistTitle,omitempty"`
	PersistReason     string  `json:"persistReason,omitempty"`
	PersistSavedBody  string  `json:"persistSavedBody,omitempty"` // markdown body written when PersistSaved true
}

// MemoryTraceEnvelope is the on-disk envelope for session memory traces.
type MemoryTraceEnvelope struct {
	Version int                   `json:"version"`
	Turns   []MemoryTurnTraceJSON `json:"turns"`
}

func memoryTracePath(sessionDir string) string {
	return filepath.Join(sessionDir, memoryTraceFile)
}

// ReadMemoryTrace loads memory_trace.json or returns empty envelope when missing.
func ReadMemoryTrace(sessionDir string) (*MemoryTraceEnvelope, error) {
	sessionDir = strings.TrimSpace(sessionDir)
	if sessionDir == "" {
		return &MemoryTraceEnvelope{Version: memoryTraceVersion, Turns: nil}, nil
	}
	p := memoryTracePath(sessionDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &MemoryTraceEnvelope{Version: memoryTraceVersion, Turns: nil}, nil
		}
		return nil, fmt.Errorf("read memory trace: %w", err)
	}
	var env MemoryTraceEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("parse memory trace: %w", err)
	}
	if env.Version == 0 {
		env.Version = memoryTraceVersion
	}
	return &env, nil
}

// AppendMemoryTurn saves or merges a trace row keyed by UserTurnIndex.
func AppendMemoryTurn(sessionDir string, row MemoryTurnTraceJSON) error {
	sessionDir = strings.TrimSpace(sessionDir)
	if sessionDir == "" {
		return nil
	}
	env, err := ReadMemoryTrace(sessionDir)
	if err != nil {
		return err
	}
	env.Version = memoryTraceVersion
	replaced := false
	for i := range env.Turns {
		if env.Turns[i].UserTurnIndex == row.UserTurnIndex {
			env.Turns[i] = mergeMemoryTurn(env.Turns[i], row)
			replaced = true
			break
		}
	}
	if !replaced {
		env.Turns = append(env.Turns, row)
	}
	return writeJSONAtomic(memoryTracePath(sessionDir), env)
}

func mergeMemoryTurn(old, upd MemoryTurnTraceJSON) MemoryTurnTraceJSON {
	out := old
	if upd.MemoryRowID != "" {
		out.MemoryRowID = upd.MemoryRowID
	}
	if upd.RecallSkipped {
		out.RecallSkipped = true
	}
	if upd.RecallText != "" {
		out.RecallText = upd.RecallText
	}
	if upd.RecallReasoningText != "" {
		out.RecallReasoningText = upd.RecallReasoningText
	}
	if upd.RecallDurationMs > 0 {
		out.RecallDurationMs = upd.RecallDurationMs
	}
	if len(upd.RecallReadPaths) > 0 {
		out.RecallReadPaths = mergeRecallReadPathsDedup(out.RecallReadPaths, upd.RecallReadPaths)
	}
	if upd.PersistJudgeText != "" {
		out.PersistJudgeText = upd.PersistJudgeText
	}
	if upd.PersistDurationMs > 0 {
		out.PersistDurationMs = upd.PersistDurationMs
	}
	if upd.PersistSaved {
		out.PersistSaved = true
	}
	if upd.PersistScope != "" {
		out.PersistScope = upd.PersistScope
	}
	if upd.PersistRelativePath != "" {
		out.PersistRelativePath = upd.PersistRelativePath
	}
	if upd.PersistTitle != "" {
		out.PersistTitle = upd.PersistTitle
	}
	if upd.PersistReason != "" {
		out.PersistReason = upd.PersistReason
	}
	if upd.PersistSavedBody != "" {
		out.PersistSavedBody = upd.PersistSavedBody
	}
	return out
}

func mergeRecallReadPathsDedup(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, xs := range [][]string{a, b} {
		for _, raw := range xs {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			if _, ok := seen[raw]; ok {
				continue
			}
			seen[raw] = struct{}{}
			out = append(out, raw)
		}
	}
	return out
}
