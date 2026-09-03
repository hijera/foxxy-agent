// Package ideenv holds a process-global snapshot of the IDE editor state
// (open tabs + active file) reported by IDE extensions over HTTP.
//
// There is one foxxycode process per workspace, so a single package-level
// snapshot is sufficient. The HTTP layer writes it when an editor reports a
// change; internal/agent reads it when assembling a turn's user message so the
// model knows which files the user is actively viewing (mirroring the
// environment context other coding agents inject each turn).
package ideenv

import (
	"strings"
	"sync"
	"time"
)

// maxSelectionBytes caps the stored selection text; the tail is kept,
// mirroring internal/ideterm output capping.
const maxSelectionBytes = 16 * 1024

// Selection is the current editor text selection: absolute file path, 1-based
// inclusive line range, and the selected text.
type Selection struct {
	File      string
	StartLine int
	EndLine   int
	Text      string
}

// Snapshot is the latest editor state reported by an IDE client.
type Snapshot struct {
	// OpenFiles lists the absolute paths of the open editor tabs.
	OpenFiles []string
	// ActiveFile is the absolute path of the focused editor, or "" when none.
	ActiveFile string
	// Selection is the current text selection, or nil when none.
	Selection *Selection
	// At is the time the snapshot was last set (UTC).
	At time.Time
}

var (
	mu      sync.RWMutex
	current Snapshot
)

// Set stores the latest editor snapshot. Blank paths are dropped and the
// open-files slice is copied defensively so later mutation by the caller does
// not race with readers. A selection with a blank file, invalid line range, or
// blank text is dropped; selection text is capped at maxSelectionBytes.
func Set(openFiles []string, activeFile string, sel *Selection) {
	cleaned := make([]string, 0, len(openFiles))
	for _, f := range openFiles {
		if f = strings.TrimSpace(f); f != "" {
			cleaned = append(cleaned, f)
		}
	}
	mu.Lock()
	current = Snapshot{
		OpenFiles:  cleaned,
		ActiveFile: strings.TrimSpace(activeFile),
		Selection:  cleanSelection(sel),
		At:         time.Now().UTC(),
	}
	mu.Unlock()
}

func cleanSelection(sel *Selection) *Selection {
	if sel == nil {
		return nil
	}
	file := strings.TrimSpace(sel.File)
	if file == "" || sel.StartLine < 1 || sel.EndLine < sel.StartLine || strings.TrimSpace(sel.Text) == "" {
		return nil
	}
	text := sel.Text
	if len(text) > maxSelectionBytes {
		text = text[len(text)-maxSelectionBytes:]
	}
	return &Selection{File: file, StartLine: sel.StartLine, EndLine: sel.EndLine, Text: text}
}

// Get returns a copy of the latest editor snapshot. The zero snapshot (no
// OpenFiles, empty ActiveFile) is returned when no IDE has reported state.
func Get() Snapshot {
	mu.RLock()
	defer mu.RUnlock()
	cp := make([]string, len(current.OpenFiles))
	copy(cp, current.OpenFiles)
	var sel *Selection
	if current.Selection != nil {
		s := *current.Selection
		sel = &s
	}
	return Snapshot{OpenFiles: cp, ActiveFile: current.ActiveFile, Selection: sel, At: current.At}
}

// Reset clears the stored snapshot. Intended for tests.
func Reset() {
	mu.Lock()
	current = Snapshot{}
	mu.Unlock()
}
