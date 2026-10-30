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

	"github.com/hijera/foxxycode-agent/internal/textutil"
)

// maxSelectionBytes caps the stored selection text; the tail is kept,
// mirroring internal/ideterm output capping.
const maxSelectionBytes = 16 * 1024

// Buffer caps: the open-buffer snapshot is ambient context ("everything open in
// the IDE"), so it is bounded per file and in total. The tail is kept, like the
// terminal output - the end of a buffer is where the user is working.
const (
	maxBufferBytes = 8 * 1024
	maxBufferTotal = 32 * 1024
	maxBuffers     = 8
)

// Buffer is one open editor buffer: its absolute path, the document text (the
// unsaved one, when the user has edits the disk does not) and whether the
// document holds unsaved changes.
type Buffer struct {
	Path  string
	Text  string
	Dirty bool
}

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
	// Buffers holds the open editor buffers with their document text, filled
	// by the pre-send snapshot an IDE plugin pushes. Empty when the client
	// reports only tabs.
	Buffers []Buffer
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
	SetWithBuffers(openFiles, activeFile, sel, nil)
}

// SetWithBuffers additionally stores the open editor buffers a pre-send
// snapshot carries (their unsaved document text).
func SetWithBuffers(openFiles []string, activeFile string, sel *Selection, buffers []Buffer) {
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
		Buffers:    cleanBuffers(buffers),
		At:         time.Now().UTC(),
	}
	mu.Unlock()
}

// cleanBuffers keeps named buffers with non-blank text, the tail of each capped
// at maxBufferBytes and the whole set at maxBuffers / maxBufferTotal. A blank
// text is dropped: a buffer the IDE opened but never filled adds nothing.
func cleanBuffers(buffers []Buffer) []Buffer {
	out := make([]Buffer, 0, len(buffers))
	total := 0
	for _, b := range buffers {
		path := strings.TrimSpace(b.Path)
		if path == "" {
			continue
		}
		text := textutil.TailBytes(b.Text, maxBufferBytes)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if len(out) >= maxBuffers || total+len(text) > maxBufferTotal {
			break
		}
		total += len(text)
		out = append(out, Buffer{Path: path, Text: text, Dirty: b.Dirty})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanSelection(sel *Selection) *Selection {
	if sel == nil {
		return nil
	}
	file := strings.TrimSpace(sel.File)
	if file == "" || sel.StartLine < 1 || sel.EndLine < sel.StartLine || strings.TrimSpace(sel.Text) == "" {
		return nil
	}
	return &Selection{File: file, StartLine: sel.StartLine, EndLine: sel.EndLine, Text: textutil.TailBytes(sel.Text, maxSelectionBytes)}
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
	var bufs []Buffer
	if len(current.Buffers) > 0 {
		bufs = make([]Buffer, len(current.Buffers))
		copy(bufs, current.Buffers)
	}
	return Snapshot{OpenFiles: cp, ActiveFile: current.ActiveFile, Selection: sel, Buffers: bufs, At: current.At}
}

// Reset clears the stored snapshot. Intended for tests.
func Reset() {
	mu.Lock()
	current = Snapshot{}
	mu.Unlock()
}
