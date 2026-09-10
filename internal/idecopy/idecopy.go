// Package idecopy holds a process-global ring of "recently copied in the IDE"
// candidates reported by IDE extensions over HTTP. The chat composer asks the
// backend to classify pasted text against this ring (paste-to-chip): a paste
// that matches a candidate 1:1 becomes a file or terminal mention instead of
// raw text.
//
// There is one foxxycode process per workspace, so a single package-level
// store is sufficient, mirroring internal/ideenv and internal/ideterm.
package idecopy

import (
	"strings"
	"sync"
	"time"
)

// Candidate kinds.
const (
	// KindFile marks a fragment copied from an open editor file.
	KindFile = "file"
	// KindTerminal marks a fragment copied from an IDE terminal.
	KindTerminal = "terminal"
)

const (
	// maxCandidates bounds the ring; older entries are evicted first.
	maxCandidates = 5
	// candidateTTL expires stale entries so an old selection cannot chip an
	// unrelated paste minutes later.
	candidateTTL = 15 * time.Minute
	// maxCandidateBytes drops oversize offers outright: truncating would break
	// the exact-match contract.
	maxCandidateBytes = 64 * 1024
)

// Candidate is one copied fragment. File candidates carry the absolute source
// path and a 1-based inclusive line range; terminal candidates carry the
// terminal name (may be empty).
type Candidate struct {
	Kind         string
	PathAbs      string
	StartLine    int
	EndLine      int
	TerminalName string
	Text         string
	At           time.Time
}

var (
	mu   sync.RWMutex
	ring []Candidate // newest first
)

// Normalize maps text to its comparison form: CRLF becomes LF and trailing
// newlines are dropped, so a CRLF copy matches an LF paste.
func Normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}

// Offer records a copy candidate. Blank and oversize texts are dropped; an
// offer identical to the newest entry only refreshes its timestamp.
func Offer(c Candidate) {
	if strings.TrimSpace(c.Text) == "" || len(c.Text) > maxCandidateBytes {
		return
	}
	if c.At.IsZero() {
		c.At = time.Now().UTC()
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ring) > 0 && sameSource(ring[0], c) && Normalize(ring[0].Text) == Normalize(c.Text) {
		ring[0].At = c.At
		return
	}
	ring = append([]Candidate{c}, ring...)
	if len(ring) > maxCandidates {
		ring = ring[:maxCandidates]
	}
}

func sameSource(a, b Candidate) bool {
	return a.Kind == b.Kind && a.PathAbs == b.PathAbs && a.TerminalName == b.TerminalName &&
		a.StartLine == b.StartLine && a.EndLine == b.EndLine
}

// MatchFile returns the newest unexpired file candidate whose normalized text
// equals the normalized input.
func MatchFile(text string) (Candidate, bool) {
	return match(KindFile, text)
}

// MatchTerminal returns the newest unexpired terminal candidate whose
// normalized text equals the normalized input.
func MatchTerminal(text string) (Candidate, bool) {
	return match(KindTerminal, text)
}

func match(kind, text string) (Candidate, bool) {
	want := Normalize(text)
	if want == "" {
		return Candidate{}, false
	}
	cutoff := time.Now().Add(-candidateTTL)
	mu.RLock()
	defer mu.RUnlock()
	for _, c := range ring {
		if c.Kind != kind || c.At.Before(cutoff) {
			continue
		}
		if Normalize(c.Text) == want {
			return c, true
		}
	}
	return Candidate{}, false
}

// Candidates returns a copy of the ring, newest first. Intended for tests.
func Candidates() []Candidate {
	mu.RLock()
	defer mu.RUnlock()
	cp := make([]Candidate, len(ring))
	copy(cp, ring)
	return cp
}

// Reset clears the ring. Intended for tests.
func Reset() {
	mu.Lock()
	ring = nil
	mu.Unlock()
}
