package session

import (
	"errors"
	"strings"
	"testing"
)

// TestNewSessionIDFormat checks the minted shape used by folder persistence.
func TestNewSessionIDFormat(t *testing.T) {
	id, err := newSessionID()
	if err != nil {
		t.Fatalf("newSessionID: %v", err)
	}
	if !strings.HasPrefix(id, "sess_") {
		t.Fatalf("id %q missing sess_ prefix", id)
	}
	if err := ValidateFolderSessionID(id); err != nil {
		t.Fatalf("generated id %q invalid: %v", id, err)
	}
}

// TestNewSessionIDEntropyFailure pins the no-panic contract: entropy
// exhaustion surfaces as an error instead of killing the process that serves
// every other session.
func TestNewSessionIDEntropyFailure(t *testing.T) {
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("boom") }
	defer func() { randRead = old }()

	if _, err := newSessionID(); err == nil {
		t.Fatal("expected error when rand.Read fails")
	}
}
