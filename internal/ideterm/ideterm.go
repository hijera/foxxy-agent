// Package ideterm holds a process-global snapshot of the IDE terminal state
// (every open terminal plus its recent output) reported by IDE extensions over
// HTTP.
//
// There is one foxxycode process per workspace, so a single package-level
// snapshot is sufficient. The HTTP layer writes it when a terminal reports a
// change; internal/agent reads it when assembling a turn's user message so the
// model can see the terminals the user is actively working in (mirroring the
// open-files context in package ideenv, and cline's @terminal mention).
package ideterm

import (
	"strings"
	"sync"
	"time"
)

// maxOutputBytes bounds the stored recent-output tail per terminal. The IDE
// side also caps its buffer; this is a defensive server-side re-cap so a
// misbehaving client cannot balloon memory or the injected prompt.
const maxOutputBytes = 16 * 1024

// Terminal is a single IDE terminal reported by a client.
type Terminal struct {
	// ID is a client-stable identifier for the terminal.
	ID string
	// Name is the terminal's title (e.g. "zsh", "dev server"). Required.
	Name string
	// Shell is the shell path or name, or "" when unknown.
	Shell string
	// Cwd is the terminal's working directory, or "" when unknown.
	Cwd string
	// LastCommand is the most recently run command, or "" when unknown.
	LastCommand string
	// Output is a bounded tail of the terminal's recent output.
	Output string
	// Active reports whether this is the focused terminal.
	Active bool
}

// Snapshot is the latest terminal state reported by an IDE client.
type Snapshot struct {
	// Terminals lists every open terminal, active-first is not guaranteed here
	// (readers order as needed).
	Terminals []Terminal
	// At is the time the snapshot was last set (UTC).
	At time.Time
}

var (
	mu      sync.RWMutex
	current Snapshot
)

// Set stores the latest terminal snapshot. Entries with a blank name are
// dropped, per-terminal output is capped to maxOutputBytes (keeping the tail),
// and the slice is copied defensively so later mutation by the caller does not
// race with readers.
func Set(terminals []Terminal) {
	cleaned := make([]Terminal, 0, len(terminals))
	for _, tm := range terminals {
		tm.Name = strings.TrimSpace(tm.Name)
		if tm.Name == "" {
			continue
		}
		tm.Output = capTail(tm.Output, maxOutputBytes)
		cleaned = append(cleaned, tm)
	}
	mu.Lock()
	current = Snapshot{Terminals: cleaned, At: time.Now().UTC()}
	mu.Unlock()
}

// Get returns a copy of the latest terminal snapshot. The zero snapshot (no
// terminals) is returned when no IDE has reported state.
func Get() Snapshot {
	mu.RLock()
	defer mu.RUnlock()
	cp := make([]Terminal, len(current.Terminals))
	copy(cp, current.Terminals)
	return Snapshot{Terminals: cp, At: current.At}
}

// Reset clears the stored snapshot. Intended for tests.
func Reset() {
	mu.Lock()
	current = Snapshot{}
	mu.Unlock()
}

// capTail returns the last maxBytes bytes of s (on a rune boundary) when s
// exceeds the cap, otherwise s unchanged.
func capTail(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	tail := s[len(s)-maxBytes:]
	// Trim a possibly-split leading rune so the result stays valid UTF-8.
	for i := 0; i < len(tail) && i < 4; i++ {
		if tail[i]&0xC0 != 0x80 { // first non-continuation byte
			return tail[i:]
		}
	}
	return tail
}
