package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
)

const toolCallMetaVersion = 1

type ToolCallMeta struct {
	Version      int             `json:"version"`
	ToolCallID   string          `json:"toolCallId"`
	Name         string          `json:"name,omitempty"`
	Kind         string          `json:"kind,omitempty"`
	Status       string          `json:"status,omitempty"`
	StartedAt    string          `json:"startedAt,omitempty"`
	FinishedAt   string          `json:"finishedAt,omitempty"`
	PlanSnapshot []acp.PlanEntry `json:"planSnapshot,omitempty"`
}

func toolCallDir(sessionDir, toolCallID string) (string, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return "", fmt.Errorf("session directory is empty")
	}
	id := strings.TrimSpace(toolCallID)
	if id == "" {
		return "", fmt.Errorf("toolCallId is empty")
	}
	return filepath.Join(sessionDir, toolCallsDirName, toolCallDirName(id)), nil
}

// toolCallDirName maps a tool call id to its directory name. Ids made of
// letters, digits, '.', '_' and '-' are used verbatim, which keeps every store
// written so far readable. Anything else (NeuralDeep-hosted models answer with
// harmony ids such as "functions.foxxycode_todo_plan_replace:0", and ':' is not a
// legal path character on Windows) is rewritten to that alphabet with a short
// hash suffix so distinct ids never share a directory.
func toolCallDirName(id string) string {
	safe := true
	for _, r := range id {
		if !toolCallDirRuneOK(r) {
			safe = false
			break
		}
	}
	if safe {
		return id
	}
	var b strings.Builder
	for _, r := range id {
		if toolCallDirRuneOK(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	sum := sha256.Sum256([]byte(id))
	return b.String() + "-" + hex.EncodeToString(sum[:4])
}

func toolCallDirRuneOK(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}

func ensureToolCallDir(sessionDir, toolCallID string) (string, error) {
	dir, err := toolCallDir(sessionDir, toolCallID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func WriteToolCallArgs(sessionDir, toolCallID, argsJSON string) error {
	dir, err := ensureToolCallDir(sessionDir, toolCallID)
	if err != nil {
		return err
	}
	// Pretty-print the raw bytes: a round trip through interface{} would
	// turn integers past 2^53 into rounded floats, and a permission resume
	// binds to these exact arguments.
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, []byte(argsJSON), "", "  "); err != nil {
		return writeTextAtomic(filepath.Join(dir, "args.json"), argsJSON)
	}
	pretty.WriteByte('\n')
	return writeBytesAtomic(filepath.Join(dir, "args.json"), pretty.Bytes())
}

func WriteToolCallResult(sessionDir, toolCallID, resultMarkdown string) error {
	dir, err := ensureToolCallDir(sessionDir, toolCallID)
	if err != nil {
		return err
	}
	return writeTextAtomic(filepath.Join(dir, "result.md"), resultMarkdown)
}

func WriteToolCallMeta(sessionDir, toolCallID string, meta ToolCallMeta) error {
	dir, err := ensureToolCallDir(sessionDir, toolCallID)
	if err != nil {
		return err
	}
	if meta.Version == 0 {
		meta.Version = toolCallMetaVersion
	}
	if meta.ToolCallID == "" {
		meta.ToolCallID = strings.TrimSpace(toolCallID)
	}
	return writeJSONAtomic(filepath.Join(dir, "meta.json"), meta)
}

// WriteToolCallPlanSnapshot stores the final todo state that a mutating tool call produced.
// It lets historical tool cards render their original rows after later plan mutations.
// A call whose meta.json does not exist yet gets a fresh one, so the snapshot may be
// written before the call is marked started or finished.
func WriteToolCallPlanSnapshot(sessionDir, toolCallID string, entries []acp.PlanEntry) error {
	meta, err := ReadToolCallMeta(sessionDir, toolCallID)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		meta = &ToolCallMeta{ToolCallID: strings.TrimSpace(toolCallID)}
	}
	meta.PlanSnapshot = append([]acp.PlanEntry(nil), entries...)
	return WriteToolCallMeta(sessionDir, toolCallID, *meta)
}

// AttachTodoPlanMeta returns meta with _meta.foxxycode.todoPlan set to entries, creating
// the envelope when needed and leaving meta untouched when there is nothing to attach.
// The value stays typed as []acp.PlanEntry: the ACP replay and the SSE bridge rely on
// that shape, and so do the agent tests that read it back.
func AttachTodoPlanMeta(meta map[string]interface{}, entries []acp.PlanEntry) map[string]interface{} {
	if len(entries) == 0 {
		return meta
	}
	if meta == nil {
		meta = map[string]interface{}{}
	}
	foxxycodeMeta, _ := meta["foxxycode"].(map[string]interface{})
	if foxxycodeMeta == nil {
		foxxycodeMeta = map[string]interface{}{}
		meta["foxxycode"] = foxxycodeMeta
	}
	foxxycodeMeta["todoPlan"] = append([]acp.PlanEntry(nil), entries...)
	return meta
}

func ReadToolCallMeta(sessionDir, toolCallID string) (*ToolCallMeta, error) {
	dir, err := toolCallDir(sessionDir, toolCallID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var meta ToolCallMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func ReadToolCallArgs(sessionDir, toolCallID string) (string, error) {
	dir, err := toolCallDir(sessionDir, toolCallID)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, "args.json"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ReadToolCallResult(sessionDir, toolCallID string) (string, error) {
	dir, err := toolCallDir(sessionDir, toolCallID)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, "result.md"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ListToolCalls(sessionDir string) ([]string, error) {
	if strings.TrimSpace(sessionDir) == "" {
		return nil, fmt.Errorf("session directory is empty")
	}
	root := filepath.Join(sessionDir, toolCallsDirName)
	de, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(de))
	for _, e := range de {
		if !e.IsDir() {
			continue
		}
		name := strings.TrimSpace(e.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// MarkToolCallStarted resets meta.json for a new attempt of the call: nothing from an
// earlier attempt (timestamps, plan snapshot) may survive a restart.
func MarkToolCallStarted(sessionDir, toolCallID, name, kind, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	meta := ToolCallMeta{
		Version:    toolCallMetaVersion,
		ToolCallID: strings.TrimSpace(toolCallID),
		Name:       strings.TrimSpace(name),
		Kind:       strings.TrimSpace(kind),
		Status:     strings.TrimSpace(status),
		StartedAt:  now,
	}
	return WriteToolCallMeta(sessionDir, toolCallID, meta)
}

// MarkToolCallFinished stamps the outcome while keeping what the running call already
// recorded: StartedAt and the plan snapshot a todo tool wrote before finishing.
func MarkToolCallFinished(sessionDir, toolCallID, name, kind, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	meta := ToolCallMeta{
		Version:    toolCallMetaVersion,
		ToolCallID: strings.TrimSpace(toolCallID),
		Name:       strings.TrimSpace(name),
		Kind:       strings.TrimSpace(kind),
		Status:     strings.TrimSpace(status),
		FinishedAt: now,
	}
	if prev, err := ReadToolCallMeta(sessionDir, toolCallID); err == nil && prev != nil {
		if prev.Version != 0 {
			meta.Version = prev.Version
		}
		if strings.TrimSpace(meta.Name) == "" {
			meta.Name = prev.Name
		}
		if strings.TrimSpace(meta.Kind) == "" {
			meta.Kind = prev.Kind
		}
		if strings.TrimSpace(prev.StartedAt) != "" {
			meta.StartedAt = prev.StartedAt
		}
		if len(prev.PlanSnapshot) > 0 {
			meta.PlanSnapshot = append([]acp.PlanEntry(nil), prev.PlanSnapshot...)
		}
	}
	return WriteToolCallMeta(sessionDir, toolCallID, meta)
}

func writeTextAtomic(path, text string) error {
	var data []byte
	if strings.TrimSpace(text) != "" {
		data = []byte(text)
		if data[len(data)-1] != '\n' {
			data = append(data, '\n')
		}
	}
	return writeBytesAtomic(path, data)
}

// CopyToolCallStore copies the per-call files (meta.json, args.json, result.md) of
// toolCallID from srcSessionDir into dstSessionDir. A call with no store in the
// source is skipped silently: the transcript still carries the call itself.
func CopyToolCallStore(srcSessionDir, dstSessionDir, toolCallID string) error {
	srcDir, err := toolCallDir(srcSessionDir, toolCallID)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	dstDir, err := ensureToolCallDir(dstSessionDir, toolCallID)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return err
		}
		if err := writeBytesAtomic(filepath.Join(dstDir, e.Name()), data); err != nil {
			return err
		}
	}
	return nil
}
