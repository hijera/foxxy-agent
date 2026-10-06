package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExecutionCheckpoint survives process exits independently of transcript compaction.
// Callers hold the session turn lock while updating it.
type ExecutionCheckpoint struct {
	Status   string   `json:"status"`
	Scope    string   `json:"scope,omitempty"`
	Seen     []string `json:"seen,omitempty"`
	Repeats  int      `json:"repeats,omitempty"`
	LastTool string   `json:"lastTool,omitempty"`
}

func ReadExecutionCheckpoint(dir string) (*ExecutionCheckpoint, error) {
	cp := &ExecutionCheckpoint{}
	if dir == "" {
		return cp, nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "execution.json"))
	if os.IsNotExist(err) {
		return cp, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, cp); err != nil {
		return nil, fmt.Errorf("execution checkpoint: %w", err)
	}
	return cp, nil
}

func (c *ExecutionCheckpoint) Save(dir string) error {
	if dir == "" {
		return nil
	}
	return writeJSONAtomic(filepath.Join(dir, "execution.json"), c)
}

// ObservationHash canonicalizes JSON arguments, including object key ordering.
func ObservationHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		part = strings.TrimSpace(part)
		var value interface{}
		dec := json.NewDecoder(strings.NewReader(part))
		dec.UseNumber()
		if json.Valid([]byte(part)) && dec.Decode(&value) == nil {
			if b, err := json.Marshal(value); err == nil {
				part = string(b)
			}
		}
		_, _ = fmt.Fprintf(h, "%d:%s", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Observe detects repeated observations, including short A/B cycles. New information
// resets the consecutive stall count; the bounded window survives that reset.
func (c *ExecutionCheckpoint) Observe(signature string) {
	for _, old := range c.Seen {
		if old == signature {
			c.Repeats++
			return
		}
	}
	c.Repeats = 0
	c.Seen = append(c.Seen, signature)
	if len(c.Seen) > 12 {
		c.Seen = c.Seen[len(c.Seen)-12:]
	}
}
