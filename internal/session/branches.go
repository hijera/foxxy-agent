package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/EvilFreelancer/coddy-agent/internal/llm"
)

const branchesFile = "branches.json"
const branchesFileVersion = 1

// BranchSessionRef identifies one session at a branch point.
type BranchSessionRef struct {
	SessionID   string `json:"sessionId"`
	BranchIndex int    `json:"branchIndex"`
	// Preview holds the trimmed first N chars of the user message at this branch.
	Preview string `json:"preview,omitempty"`
}

// BranchPoint records all sessions branching from the same user-message index within a session tree.
type BranchPoint struct {
	// UserMessageIndex is the 0-based index of the user message where branching occurred.
	UserMessageIndex int                `json:"userMessageIndex"`
	Sessions         []BranchSessionRef `json:"sessions"`
}

// BranchOrigin records that this session is a branch of another session.
type BranchOrigin struct {
	ParentSessionID  string `json:"parentSessionId"`
	UserMessageIndex int    `json:"userMessageIndex"`
	MyBranchIndex    int    `json:"myBranchIndex"`
}

// BranchFile is persisted as branches.json inside a session directory.
type BranchFile struct {
	Version      int           `json:"version"`
	Origin       *BranchOrigin `json:"origin,omitempty"`
	BranchPoints []BranchPoint `json:"branchPoints,omitempty"`
}

// ReadBranchFile reads branches.json from sessionDir; returns an empty file if missing.
func ReadBranchFile(sessionDir string) (*BranchFile, error) {
	p := filepath.Join(sessionDir, branchesFile)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &BranchFile{Version: branchesFileVersion}, nil
		}
		return nil, err
	}
	var bf BranchFile
	if err := json.Unmarshal(b, &bf); err != nil {
		return nil, fmt.Errorf("branches.json: %w", err)
	}
	return &bf, nil
}

// WriteBranchFile atomically writes bf to branches.json in sessionDir.
func WriteBranchFile(sessionDir string, bf *BranchFile) error {
	if bf.Version == 0 {
		bf.Version = branchesFileVersion
	}
	p := filepath.Join(sessionDir, branchesFile)
	return writeJSONAtomic(p, bf)
}

// branchPointForIndex returns a pointer to the BranchPoint for userMessageIndex, creating one if absent.
func branchPointForIndex(bf *BranchFile, idx int) *BranchPoint {
	for i := range bf.BranchPoints {
		if bf.BranchPoints[i].UserMessageIndex == idx {
			return &bf.BranchPoints[i]
		}
	}
	bf.BranchPoints = append(bf.BranchPoints, BranchPoint{UserMessageIndex: idx})
	return &bf.BranchPoints[len(bf.BranchPoints)-1]
}

// messagePreview returns the first 80 chars of a user message content.
func messagePreview(content string) string {
	r := []rune(content)
	if len(r) <= 80 {
		return content
	}
	return string(r[:80]) + "…"
}

// CreateBranchParams holds the inputs for Manager.CreateBranchSession.
type CreateBranchParams struct {
	// SourceSessionID is the session being branched from.
	SourceSessionID string
	// UserMessageIndex is the 0-based index of the user message at which to branch.
	// The branch session receives all messages BEFORE that user message.
	UserMessageIndex int
}

// CreateBranchResult is the output of Manager.CreateBranchSession.
type CreateBranchResult struct {
	NewSessionID  string
	BranchIndex   int
	TotalBranches int
}

// CreateBranchSession creates a new session that shares the conversation history of
// sourceSessID up to (not including) userMessageIndex, then persists branch metadata
// in both the source session and the new branch session.
//
// The caller must subsequently send the edited user message to the new session via the
// normal compose flow. The workspace files are NOT touched here; see ReverseApplyDiffs.
func (m *Manager) CreateBranchSession(params CreateBranchParams) (*CreateBranchResult, error) {
	if m.store == nil || m.store.Root == "" {
		return nil, fmt.Errorf("session store unavailable")
	}

	srcID := params.SourceSessionID
	snap, err := m.store.ReadSnapshot(srcID)
	if err != nil {
		return nil, fmt.Errorf("read source session: %w", err)
	}

	// Collect the messages up to (not including) the Nth user message.
	prefix, preview := sliceMessagesBeforeUserN(snap.Messages, params.UserMessageIndex)

	// Generate new session ID.
	newID := newSessionID()

	// Create the directory layout for the new session.
	newDir, err := m.store.EnsureLayout(newID)
	if err != nil {
		return nil, fmt.Errorf("branch layout: %w", err)
	}

	// Write messages.json for the new session with the copied prefix.
	msgPath := filepath.Join(newDir, messagesFile)
	wrap := messagesFileData{Version: messagesLayout, Messages: prefix}
	if err := writeJSONAtomic(msgPath, wrap); err != nil {
		return nil, fmt.Errorf("branch messages: %w", err)
	}

	// Read existing branch metadata for the source session.
	srcBF, err := ReadBranchFile(snap.Dir)
	if err != nil {
		return nil, fmt.Errorf("read source branches: %w", err)
	}

	// If the source session itself is a branch, work with its parent's branch file.
	// (For now, we only support one level of linear chain from the root.)
	if srcBF.Origin != nil {
		// The branch point already exists in the parent. We still add a new entry there.
		parentDir := m.store.SessionPath(srcBF.Origin.ParentSessionID)
		parentBF, err := ReadBranchFile(parentDir)
		if err != nil {
			return nil, fmt.Errorf("read parent branches: %w", err)
		}
		bp := branchPointForIndex(parentBF, srcBF.Origin.UserMessageIndex)
		newBranchIndex := len(bp.Sessions)
		bp.Sessions = append(bp.Sessions, BranchSessionRef{
			SessionID:   newID,
			BranchIndex: newBranchIndex,
			Preview:     preview,
		})
		if err := WriteBranchFile(parentDir, parentBF); err != nil {
			return nil, fmt.Errorf("write parent branches: %w", err)
		}
		// Write origin for the new session pointing to the parent.
		newBF := &BranchFile{
			Version: branchesFileVersion,
			Origin: &BranchOrigin{
				ParentSessionID:  srcBF.Origin.ParentSessionID,
				UserMessageIndex: srcBF.Origin.UserMessageIndex,
				MyBranchIndex:    newBranchIndex,
			},
		}
		if err := WriteBranchFile(newDir, newBF); err != nil {
			return nil, fmt.Errorf("write new branch file: %w", err)
		}
		return &CreateBranchResult{
			NewSessionID:  newID,
			BranchIndex:   newBranchIndex,
			TotalBranches: len(bp.Sessions),
		}, nil
	}

	// Source session is the root (no parent). Ensure it appears at index 0.
	srcBP := branchPointForIndex(srcBF, params.UserMessageIndex)
	if len(srcBP.Sessions) == 0 {
		// Add the source session itself as index 0.
		srcPreview := messagePreview(userMessageAt(snap.Messages, params.UserMessageIndex))
		srcBP.Sessions = append(srcBP.Sessions, BranchSessionRef{
			SessionID:   srcID,
			BranchIndex: 0,
			Preview:     srcPreview,
		})
	}
	newBranchIndex := len(srcBP.Sessions)
	srcBP.Sessions = append(srcBP.Sessions, BranchSessionRef{
		SessionID:   newID,
		BranchIndex: newBranchIndex,
		Preview:     preview,
	})
	if err := WriteBranchFile(snap.Dir, srcBF); err != nil {
		return nil, fmt.Errorf("write source branches: %w", err)
	}

	// Write origin for the new session.
	newBF := &BranchFile{
		Version: branchesFileVersion,
		Origin: &BranchOrigin{
			ParentSessionID:  srcID,
			UserMessageIndex: params.UserMessageIndex,
			MyBranchIndex:    newBranchIndex,
		},
	}
	if err := WriteBranchFile(newDir, newBF); err != nil {
		return nil, fmt.Errorf("write new branch file: %w", err)
	}

	return &CreateBranchResult{
		NewSessionID:  newID,
		BranchIndex:   newBranchIndex,
		TotalBranches: len(srcBP.Sessions),
	}, nil
}

// BranchPointView is the read-only view of a branch point returned to the UI.
type BranchPointView struct {
	UserMessageIndex int                `json:"userMessageIndex"`
	CurrentIndex     int                `json:"currentIndex"`
	Total            int                `json:"total"`
	Sessions         []BranchSessionRef `json:"sessions"`
}

// LoadBranchPointViews resolves the branch points visible from sessionDir.
// It returns a slice of BranchPointView so the UI can render navigation.
// If the session is itself a branch, it follows the parent to get the full list.
func (m *Manager) LoadBranchPointViews(sessionID string) ([]BranchPointView, error) {
	if m.store == nil {
		return nil, nil
	}
	dir := m.store.SessionPath(sessionID)
	bf, err := ReadBranchFile(dir)
	if err != nil {
		return nil, err
	}

	// Case 1: this session is a branch – load from parent.
	if bf.Origin != nil {
		parentDir := m.store.SessionPath(bf.Origin.ParentSessionID)
		parentBF, err := ReadBranchFile(parentDir)
		if err != nil {
			return nil, err
		}
		var out []BranchPointView
		for _, bp := range parentBF.BranchPoints {
			if bp.UserMessageIndex != bf.Origin.UserMessageIndex {
				continue
			}
			out = append(out, BranchPointView{
				UserMessageIndex: bp.UserMessageIndex,
				CurrentIndex:     bf.Origin.MyBranchIndex,
				Total:            len(bp.Sessions),
				Sessions:         bp.Sessions,
			})
		}
		return out, nil
	}

	// Case 2: source session — return its own branch points with currentIndex=0.
	var out []BranchPointView
	for _, bp := range bf.BranchPoints {
		if len(bp.Sessions) < 2 {
			continue // no actual branches yet
		}
		out = append(out, BranchPointView{
			UserMessageIndex: bp.UserMessageIndex,
			CurrentIndex:     0,
			Total:            len(bp.Sessions),
			Sessions:         bp.Sessions,
		})
	}
	return out, nil
}

// sliceMessagesBeforeUserN returns messages before the Nth (0-based) user message,
// and a preview of the Nth user message content (empty if N is out of range).
func sliceMessagesBeforeUserN(msgs []llm.Message, n int) ([]llm.Message, string) {
	userCount := 0
	for i, m := range msgs {
		if m.Role == llm.RoleUser {
			if userCount == n {
				// msgs[0..i-1] are the prefix; msgs[i] is the Nth user message.
				preview := messagePreview(m.Content)
				prefix := make([]llm.Message, i)
				copy(prefix, msgs[:i])
				return prefix, preview
			}
			userCount++
		}
	}
	// N is beyond the last user message — return all messages, no preview.
	cp := make([]llm.Message, len(msgs))
	copy(cp, msgs)
	return cp, ""
}

// userMessageAt returns the content of the Nth (0-based) user message, or "".
func userMessageAt(msgs []llm.Message, n int) string {
	count := 0
	for _, m := range msgs {
		if m.Role == llm.RoleUser {
			if count == n {
				return m.Content
			}
			count++
		}
	}
	return ""
}

// TurnDiffsDir returns the directory where per-turn git diffs are stored.
func TurnDiffsDir(sessionDir string) string {
	return filepath.Join(sessionDir, "diffs")
}

// TurnNumber returns the current user-turn count (= number of user messages).
// This is called before persisting the next turn's diff so we match "after turn N" to the Nth user message.
func TurnNumber(msgs []llm.Message) int {
	return CountUserTurns(msgs)
}

// nowUTC returns an RFC3339 UTC timestamp.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
