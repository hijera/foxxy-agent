package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrResponseRequestConflict = errors.New("idempotency key was already used for a different request")

// ResponseReceipt is an immutable request claim followed by a durable wire response.
// An incomplete claim is never automatically reclaimed: side effects may have occurred.
type ResponseReceipt struct {
	KeyHash     string              `json:"keyHash,omitempty"`
	Fingerprint string              `json:"fingerprint"`
	SessionID   string              `json:"sessionId,omitempty"`
	Completed   bool                `json:"completed"`
	Code        int                 `json:"code,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	dir         string
}

// ClaimResponseRequest publishes a nonempty directory by atomic rename, so another
// process never sees a new claim without its receipt. Keys are opaque strings,
// hashed without JSON normalization. Claims last until the operator removes them.
func (f *FileStore) ClaimResponseRequest(key, fingerprint, sessionID string) (*ResponseReceipt, bool, error) {
	root := filepath.Join(f.Root, ".response_requests")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, false, err
	}
	// Preserve the old path for non-JSON keys, while distinguishing JSON-shaped
	// strings that the original ObservationHash incorrectly normalized together.
	keyHash := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", len(key), key)))
	keyDigest := hex.EncodeToString(keyHash[:])
	dir := filepath.Join(root, keyDigest)
	if receipt, err := readResponseReceipt(dir, fingerprint); !os.IsNotExist(err) {
		return receipt, false, err
	}
	legacyDir := filepath.Join(root, ObservationHash(key))
	if legacyDir != dir {
		receipt, err := readResponseReceipt(legacyDir, "")
		if err != nil && !os.IsNotExist(err) {
			return nil, false, err
		}
		if receipt != nil && receipt.KeyHash == "" {
			// Old receipts cannot distinguish keys with the same normalized JSON.
			// Replay or refuse them conservatively rather than repeating their actions.
			if receipt.Fingerprint != "" && receipt.Fingerprint != fingerprint {
				return nil, false, ErrResponseRequestConflict
			}
			return receipt, false, nil
		}
	}
	staging, err := os.MkdirTemp(root, ".claim-")
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	receipt := &ResponseReceipt{KeyHash: keyDigest, Fingerprint: fingerprint, SessionID: sessionID, dir: staging}
	if err := receipt.Save(); err != nil {
		return nil, false, err
	}
	// A competing published directory contains receipt.json, so neither POSIX
	// nor Windows can replace it with this rename. Only one process owns the key.
	if err := os.Rename(staging, dir); err != nil {
		if existing, readErr := readResponseReceipt(dir, fingerprint); !os.IsNotExist(readErr) {
			return existing, false, readErr
		}
		return nil, false, err
	}
	receipt.dir = dir
	return receipt, true, nil
}

func readResponseReceipt(dir, fingerprint string) (*ResponseReceipt, error) {
	b, err := readFileWithRetry(filepath.Join(dir, "receipt.json"))
	if os.IsNotExist(err) {
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			// A pre-publication crash in the old implementation left an empty
			// reserved directory. Its ownership and payload are unknown: do not
			// reclaim it or ask the caller to retry an unrecoverable 503 forever.
			return &ResponseReceipt{dir: dir}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	var receipt ResponseReceipt
	if err := json.Unmarshal(b, &receipt); err != nil {
		return nil, err
	}
	receipt.dir = dir
	if fingerprint != "" && receipt.Fingerprint != fingerprint {
		return nil, ErrResponseRequestConflict
	}
	return &receipt, nil
}

func (r *ResponseReceipt) Save() error {
	return writeJSONAtomic(filepath.Join(r.dir, "receipt.json"), r)
}
func (r *ResponseReceipt) CreateBody() (*os.File, error) {
	return os.OpenFile(filepath.Join(r.dir, "response.bin"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}
func (r *ResponseReceipt) OpenBody() (*os.File, error) {
	return os.Open(filepath.Join(r.dir, "response.bin"))
}
