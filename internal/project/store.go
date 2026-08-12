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

// RecentEntry is one recently opened project folder. LastSessionID remembers
// the session the user last had open there, so an editor plugin can reopen it
// on the next launch; editor plugins bind the backend to a fresh random port
// every time, which rules out browser-side storage.
type RecentEntry struct {
	Path          string `json:"path"`
	LastOpenedAt  string `json:"last_opened_at"`
	LastSessionID string `json:"last_session_id,omitempty"`
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
			// Reopening a project must not forget which session was open in
			// it: editor plugins re-seed the current project on every launch.
			next[0].LastSessionID = r.LastSessionID
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

// SetLastSession records sessionID as the session last opened in dir and
// saves to disk. An empty sessionID clears the record. The recent list keeps
// its order and the current project is left alone: this is a bookmark, not a
// project switch. A directory missing from the recent list is appended, so a
// server started without an explicit -cwd still remembers its session.
func (s *Store) SetLastSession(dir, sessionID string) error {
	clean, err := ValidateDir(dir)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(sessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Recent {
		if samePath(s.data.Recent[i].Path, clean) {
			s.data.Recent[i].LastSessionID = id
			return s.save()
		}
	}
	if len(s.data.Recent) >= maxRecent {
		s.data.Recent = s.data.Recent[:maxRecent-1]
	}
	s.data.Recent = append(s.data.Recent, RecentEntry{
		Path:          clean,
		LastOpenedAt:  time.Now().UTC().Format(time.RFC3339),
		LastSessionID: id,
	})
	return s.save()
}

// LastSession returns the session id last opened in dir, or "" when the
// directory is unknown, invalid, or has no record. Callers must still check
// that the session exists before routing to it.
func (s *Store) LastSession(dir string) string {
	clean, err := ValidateDir(dir)
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.data.Recent {
		if samePath(r.Path, clean) {
			return r.LastSessionID
		}
	}
	return ""
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
