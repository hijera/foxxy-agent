package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResponseRequestReplaysLegacyJSONKey(t *testing.T) {
	fs := &FileStore{Root: t.TempDir()}
	key := "[1, 2]"
	dir := filepath.Join(fs.Root, ".response_requests", ObservationHash(key))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := &ResponseReceipt{Fingerprint: "original", SessionID: "sess_original", dir: dir}
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}
	receipt, claimed, err := fs.ClaimResponseRequest(key, "original", "")
	if err != nil || claimed || receipt.SessionID != "sess_original" {
		t.Fatalf("legacy request was lost: receipt=%+v claimed=%v err=%v", receipt, claimed, err)
	}
	if _, _, err := fs.ClaimResponseRequest(key, "changed", ""); !errors.Is(err, ErrResponseRequestConflict) {
		t.Fatalf("legacy conflict was lost: %v", err)
	}
}

func TestResponseRequestLeavesUnpublishedStagingDirectoryAlone(t *testing.T) {
	fs := &FileStore{Root: t.TempDir()}
	dir := filepath.Join(fs.Root, ".response_requests", ".claim-abandoned")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	staged := &ResponseReceipt{Fingerprint: "original", dir: dir}
	if err := staged.Save(); err != nil {
		t.Fatal(err)
	}
	receipt, claimed, err := fs.ClaimResponseRequest("key", "original", "")
	if err != nil || !claimed {
		t.Fatalf("unpublished request was reserved: claimed=%v err=%v", claimed, err)
	}
	if _, err := os.Stat(filepath.Join(receipt.dir, "receipt.json")); err != nil {
		t.Fatalf("published claim has no receipt: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("another process's staging directory was removed: %v", err)
	}
}
