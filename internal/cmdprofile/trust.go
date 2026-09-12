package cmdprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrustFileName is the per-home approval store for portable command profiles,
// the same pattern as the MCP project-trust file: a declaration travels with
// the document, the authorization stays on this machine.
const TrustFileName = "command-trust.json"

// trustFile is the on-disk shape. Entries are keyed by the profile's canonical
// hash; the approval binds to the exact binary path the operator saw.
type trustFile struct {
	Version int                   `json:"version"`
	Entries map[string]trustEntry `json:"entries"`
}

type trustEntry struct {
	Binary     string    `json:"binary"`
	ApprovedAt time.Time `json:"approved_at"`
}

// TrustStore reads and records profile approvals under one home directory.
// The file is re-read on every call so concurrent processes converge.
type TrustStore struct {
	path string
	mu   sync.Mutex
}

// NewTrustStore returns the store for the given home directory. An empty home
// yields an inert store: nothing is trusted and nothing can be recorded.
func NewTrustStore(home string) *TrustStore {
	if home == "" {
		return &TrustStore{}
	}
	return &TrustStore{path: filepath.Join(home, TrustFileName)}
}

// Trusted reports whether this exact profile content, resolved to this exact
// binary path, was approved on this machine. Either changing invalidates the
// approval: an edited profile has a new hash, a moved binary a new path.
func (s *TrustStore) Trusted(hash, binaryPath string) bool {
	if s == nil || s.path == "" || hash == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.readLocked()
	entry, ok := entries[hash]
	if !ok {
		return false
	}
	return canonicalPath(entry.Binary) == canonicalPath(binaryPath)
}

// Record persists an approval for the (profile hash, binary path) pair,
// replacing any previous approval for the same hash.
func (s *TrustStore) Record(hash, binaryPath string) error {
	if s == nil || s.path == "" {
		return errors.New("command trust store has no home directory")
	}
	if hash == "" || binaryPath == "" {
		return errors.New("a profile hash and a binary path are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.readLocked()
	entries[hash] = trustEntry{Binary: filepath.Clean(binaryPath), ApprovedAt: time.Now().UTC()}
	raw, err := json.MarshalIndent(trustFile{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode command trust file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	// Write-temp-then-rename so a concurrent reader never sees a torn file.
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, s.path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

// readLocked loads the current entries; a missing or corrupt file yields an
// empty set rather than an error, so a damaged store fails closed (nothing
// trusted) and the next Record rewrites it whole.
func (s *TrustStore) readLocked() map[string]trustEntry {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return map[string]trustEntry{}
	}
	var file trustFile
	if err := json.Unmarshal(raw, &file); err != nil || file.Entries == nil {
		return map[string]trustEntry{}
	}
	return file.Entries
}
