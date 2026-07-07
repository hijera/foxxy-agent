// Package project tracks the current project directory and a persisted
// list of recently opened project folders (~/.foxxycode/projects.json).
// New HTTP sessions inherit the current project as their working directory.
package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	fileName  = "projects.json"
	maxRecent = 15
)

// RecentEntry is one recently opened project folder.
type RecentEntry struct {
	Path         string `json:"path"`
	LastOpenedAt string `json:"last_opened_at"`
}

type fileShape struct {
	Version int           `json:"version"`
	Current string        `json:"current"`
	Recent  []RecentEntry `json:"recent"`
}

// Store persists the current project directory and the recent list.
type Store struct {
	mu       sync.Mutex
	filePath string
	data     fileShape
}

// Open loads the store from <home>/projects.json. A missing or corrupt
// file yields an empty store; only I/O setup problems return an error.
func Open(home string) (*Store, error) {
	s := &Store{
		filePath: filepath.Join(home, fileName),
		data:     fileShape{Version: 1},
	}
	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	var parsed fileShape
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// Corrupt file: start fresh rather than blocking startup.
		return s, nil
	}
	parsed.Version = 1
	if len(parsed.Recent) > maxRecent {
		parsed.Recent = parsed.Recent[:maxRecent]
	}
	s.data = parsed
	return s, nil
}

// Current returns the current project directory, or "" when unset or
// when the directory no longer exists on disk.
func (s *Store) Current() string {
	s.mu.Lock()
	cur := s.data.Current
	s.mu.Unlock()
	if cur == "" {
		return ""
	}
	if _, err := ValidateDir(cur); err != nil {
		return ""
	}
	return cur
}

// SetCurrent validates path, makes it the current project, bumps the
// recent list (dedupe, move-to-front, cap) and saves to disk.
func (s *Store) SetCurrent(path string) error {
	clean, err := ValidateDir(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Current = clean
	entry := RecentEntry{Path: clean, LastOpenedAt: time.Now().UTC().Format(time.RFC3339)}
	next := make([]RecentEntry, 0, len(s.data.Recent)+1)
	next = append(next, entry)
	for _, r := range s.data.Recent {
		if samePath(r.Path, clean) {
			continue
		}
		next = append(next, r)
		if len(next) == maxRecent {
			break
		}
	}
	s.data.Recent = next
	return s.save()
}

// Recent returns a copy of the recent list, most recently opened first.
// Entries whose directories vanished are kept so the UI can flag them.
func (s *Store) Recent() []RecentEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RecentEntry, len(s.data.Recent))
	copy(out, s.data.Recent)
	return out
}

// ValidateDir checks that path names an existing directory and returns
// the cleaned absolute form.
func ValidateDir(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("project: path is empty")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project: not a directory: " + abs)
	}
	return abs, nil
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, raw, 0o644)
}
