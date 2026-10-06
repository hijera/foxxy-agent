package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var ErrResponseRequestConflict = errors.New("idempotency key was already used for a different request")

// ResponseReceipt is an immutable request claim followed by a durable wire response.
// An incomplete claim is never automatically reclaimed: side effects may have occurred.
type ResponseReceipt struct {
	Fingerprint string              `json:"fingerprint"`
	SessionID   string              `json:"sessionId,omitempty"`
	Completed   bool                `json:"completed"`
	Code        int                 `json:"code,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	dir         string
}

// ClaimResponseRequest uses exclusive directory creation across processes. Keys are
// hashed, never used as paths. Claims last until the operator removes the store.
func (f *FileStore) ClaimResponseRequest(key, fingerprint, sessionID string) (*ResponseReceipt, bool, error) {
	root := filepath.Join(f.Root, ".response_requests")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, false, err
	}
	dir := filepath.Join(root, ObservationHash(key))
	err := os.Mkdir(dir, 0o700)
	if os.IsExist(err) {
		b, err := os.ReadFile(filepath.Join(dir, "receipt.json"))
		if err != nil {
			return nil, false, err
		}
		var receipt ResponseReceipt
		if err := json.Unmarshal(b, &receipt); err != nil {
			return nil, false, err
		}
		receipt.dir = dir
		if receipt.Fingerprint != fingerprint {
			return nil, false, ErrResponseRequestConflict
		}
		return &receipt, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	receipt := &ResponseReceipt{Fingerprint: fingerprint, SessionID: sessionID, dir: dir}
	if err := receipt.Save(); err != nil {
		return nil, false, err
	}
	return receipt, true, nil
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
