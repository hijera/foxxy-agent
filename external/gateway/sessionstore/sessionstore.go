//go:build gateway || gateway.telegram

// Package sessionstore maps stable messenger chat/user keys to Coddy session IDs.
// Each unique (gateway, chatID, userID, isolation) combination yields a single session ID
// that is replaced when the user sends /clear.
package sessionstore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/EvilFreelancer/coddy-agent/internal/config"
)

// Store maps session keys to Coddy session IDs.
type Store struct {
	mu   sync.Mutex
	data map[string]string // sessionKey -> sessionID
}

// New creates an empty store.
func New() *Store {
	return &Store{data: make(map[string]string)}
}

// SessionKey returns the string key for the given gateway/chat/user context.
// Private chats always use individual isolation regardless of the configured mode.
func SessionKey(gateway string, chatID, userID int64, mode config.IsolationMode, isGroup bool) string {
	if !isGroup {
		// Private chat: always per-user.
		return fmt.Sprintf("%s:user:%d", gateway, userID)
	}
	switch mode {
	case config.IsolationShared:
		return fmt.Sprintf("%s:chat:%d", gateway, chatID)
	case config.IsolationAdmin:
		return fmt.Sprintf("%s:chat:%d:admin", gateway, chatID)
	default: // IsolationIndividual
		return fmt.Sprintf("%s:chat:%d:user:%d", gateway, chatID, userID)
	}
}

// Get returns the Coddy session ID for key, creating a new random one when absent.
func (s *Store) Get(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.data[key]; ok {
		return id
	}
	id := newID()
	s.data[key] = id
	return id
}

// Reset replaces the session ID for key with a fresh one and returns it.
// Used by the /clear command.
func (s *Store) Reset(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newID()
	s.data[key] = id
	return id
}

// newID generates a random session ID that satisfies ValidateFolderSessionID (alphanumeric + hyphen, max 256).
func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("gateway sessionstore: crypto/rand unavailable: " + err.Error())
	}
	return "gw-" + strings.ToLower(hex.EncodeToString(b))
}
