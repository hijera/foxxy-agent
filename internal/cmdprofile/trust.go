package cmdprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TrustFileName is the per-home approval store for portable command profiles,
// the same pattern as the MCP project-trust file: a declaration travels with
// the document, the authorization stays on this machine.
const TrustFileName = "command-trust.json"

// trustFile is the on-disk shape. Entries are keyed by the profile's canonical
// hash; the approval binds to the exact binary path the operator saw and to the
// SHA-256 of the bytes that path held at approval time.
type trustFile struct {
	Version int                   `json:"version"`
	Entries map[string]trustEntry `json:"entries"`
}

type trustEntry struct {
	Binary     string    `json:"binary"`
	Digest     string    `json:"digest,omitempty"`
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

// BinaryDigest returns the SHA-256 of the file at path. The second half of the
// approval binding: an approval follows the profile content and the exact bytes
// the resolved binary held when the operator saw it, so replacing the file at
// an approved path re-prompts even though the path itself is unchanged.
func BinaryDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Trusted reports whether this exact profile content, resolved to this exact
// binary path with these exact bytes, was approved on this machine. Any of the
// three changing invalidates the approval: an edited profile has a new hash, a
// moved binary a new path, a replaced binary a new digest.
func (s *TrustStore) Trusted(hash, binaryPath string) bool {
	if s == nil || s.path == "" || hash == "" || binaryPath == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.readLocked()[hash]
	if !ok {
		return false
	}
	if entry.Digest != "" {
		digest, err := BinaryDigest(binaryPath)
		if err != nil || digest != entry.Digest {
			return false
		}
	}
	// Entries recorded before digest pinning carried no digest and fall back
	// to the resolved path alone.
	return sameBinaryPath(canonicalPath(entry.Binary), canonicalPath(binaryPath))
}

// sameBinaryPath compares two resolved binary paths. Windows paths differ in
// case by spelling, not by file, so the same binary spelled differently must
// not re-prompt; case is significant everywhere else.
func sameBinaryPath(a, b string) bool {
	if a == b {
		return true
	}
	return runtime.GOOS == "windows" && strings.EqualFold(a, b)
}

// Record persists an approval for the (profile hash, binary path, binary
// digest) triple, replacing any previous approval for the same hash. The
// binary must still be readable at binaryPath: an approval without its digest
// would degrade to path-only binding.
func (s *TrustStore) Record(hash, binaryPath string) error {
	if s == nil || s.path == "" {
		return errors.New("command trust store has no home directory")
	}
	if hash == "" || binaryPath == "" {
		return errors.New("a profile hash and a binary path are required")
	}
	digest, err := BinaryDigest(binaryPath)
	if err != nil {
		return fmt.Errorf("bind approval to binary %q: %w", filepath.Clean(binaryPath), err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.readLocked()
	entries[hash] = trustEntry{Binary: filepath.Clean(binaryPath), Digest: digest, ApprovedAt: time.Now().UTC()}
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
