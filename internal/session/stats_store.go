package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const statsFileVersion = 1

// maxStoredTurns bounds the per-turn history kept in stats.json. A long session would
// otherwise grow the file without limit, and nothing reads more than the recent rows.
// Trimmed turns are not lost: their counters move into TokenUsageTrimmed so the session
// total stays whole.
const maxStoredTurns = 200

type TokenUsageTotals struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

type TokenUsageTurn struct {
	TurnIndex    int    `json:"turnIndex"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	TotalTokens  int    `json:"totalTokens"`
	Timestamp    string `json:"timestamp"`
}

type SessionStats struct {
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt"`
	// TokenUsageTotal is what the session has spent since it was created, not what the
	// running turn has spent. The SPA treats it as the baseline the live token_usage
	// deltas of the current turn are measured against.
	TokenUsageTotal TokenUsageTotals `json:"tokenUsageTotal"`
	// TokenUsageTrimmed carries the turns dropped from TokenUsageByTurn by the cap, so
	// the total keeps counting them.
	TokenUsageTrimmed TokenUsageTotals  `json:"tokenUsageTrimmed,omitempty"`
	TokenUsageByTurn  []TokenUsageTurn  `json:"tokenUsageByTurn,omitempty"`
	ContextBreakdown  *ContextBreakdown `json:"contextBreakdown,omitempty"`
}

// ApplyTurnUsage records what the turn identified by turnIndex has spent so far and
// returns the updated document.
//
// The ReAct loop calls this repeatedly while one turn runs, with counters that already
// cover everything that turn spent, so the row for turnIndex is *replaced* rather than
// added - a rewrite must not inflate the session total. Everything else in prior is
// carried through untouched, including ContextBreakdown: compaction writes that field to
// the same file through WriteSessionContextBreakdown, and this function has no business
// reverting it.
func ApplyTurnUsage(prior *SessionStats, turnIndex int, turn TokenUsageTotals, at time.Time) SessionStats {
	out := SessionStats{}
	if prior != nil {
		out = *prior
		out.TokenUsageByTurn = append([]TokenUsageTurn(nil), prior.TokenUsageByTurn...)
	}
	out.Version = statsFileVersion
	out.UpdatedAt = at.UTC().Format(time.RFC3339)

	row := TokenUsageTurn{
		TurnIndex:    turnIndex,
		InputTokens:  turn.InputTokens,
		OutputTokens: turn.OutputTokens,
		TotalTokens:  turn.InputTokens + turn.OutputTokens,
		Timestamp:    out.UpdatedAt,
	}
	replaced := false
	for i := range out.TokenUsageByTurn {
		if out.TokenUsageByTurn[i].TurnIndex == turnIndex {
			out.TokenUsageByTurn[i] = row
			replaced = true
			break
		}
	}
	if !replaced {
		out.TokenUsageByTurn = append(out.TokenUsageByTurn, row)
	}

	if excess := len(out.TokenUsageByTurn) - maxStoredTurns; excess > 0 {
		for _, dropped := range out.TokenUsageByTurn[:excess] {
			out.TokenUsageTrimmed.InputTokens += dropped.InputTokens
			out.TokenUsageTrimmed.OutputTokens += dropped.OutputTokens
			out.TokenUsageTrimmed.TotalTokens += dropped.TotalTokens
		}
		out.TokenUsageByTurn = append([]TokenUsageTurn(nil), out.TokenUsageByTurn[excess:]...)
	}

	out.TokenUsageTotal = out.TokenUsageTrimmed
	for _, t := range out.TokenUsageByTurn {
		out.TokenUsageTotal.InputTokens += t.InputTokens
		out.TokenUsageTotal.OutputTokens += t.OutputTokens
		out.TokenUsageTotal.TotalTokens += t.TotalTokens
	}
	return out
}

func statsPath(sessionDir string) (string, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return "", fmt.Errorf("session directory is empty")
	}
	return filepath.Join(sessionDir, "stats.json"), nil
}

// NextTurnIndex is the session-global index the next user turn should record under.
// Derived from the highest index already stored rather than from the row count, so a
// history trimmed by maxStoredTurns cannot hand out an index that is already taken.
// A session with no readable stats starts at 0.
func NextTurnIndex(sessionDir string) int {
	st, err := ReadSessionStats(sessionDir)
	if err != nil || st == nil {
		return 0
	}
	next := 0
	for _, t := range st.TokenUsageByTurn {
		if t.TurnIndex >= next {
			next = t.TurnIndex + 1
		}
	}
	return next
}

func ReadSessionStats(sessionDir string) (*SessionStats, error) {
	p, err := statsPath(sessionDir)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var st SessionStats
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func WriteSessionStats(sessionDir string, st SessionStats) error {
	p, err := statsPath(sessionDir)
	if err != nil {
		return err
	}
	if st.Version == 0 {
		st.Version = statsFileVersion
	}
	if strings.TrimSpace(st.UpdatedAt) == "" {
		st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return writeJSONAtomic(p, st)
}

// WriteSessionContextBreakdown updates only the live context estimate while
// preserving the provider token counters already stored for the session.
func WriteSessionContextBreakdown(sessionDir string, b *ContextBreakdown) error {
	if b == nil {
		return nil
	}
	st, err := ReadSessionStats(sessionDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		st = &SessionStats{}
	}
	cp := *b
	st.ContextBreakdown = &cp
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return WriteSessionStats(sessionDir, *st)
}
