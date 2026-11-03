package idehost

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/ideenv"
)

func TestEditorNoteRendersOpenBuffers(t *testing.T) {
	snap := ideenv.Snapshot{
		Buffers: []ideenv.Buffer{
			{Path: "/ws/src/a.go", Text: "package main\n", Dirty: true},
			{Path: "/ws/README.md", Text: "hello\n"},
		},
	}
	note := EditorNote("/ws", snap)
	if !strings.Contains(note, "# Open Buffers") {
		t.Fatalf("no open-buffers section in %q", note)
	}
	if !strings.Contains(note, "## src/a.go (unsaved)") {
		t.Fatalf("dirty buffer not marked in %q", note)
	}
	if !strings.Contains(note, "## README.md\nhello") {
		t.Fatalf("clean buffer missing in %q", note)
	}
}

func TestCleanBuffersCaps(t *testing.T) {
	var buffers []ideenv.Buffer
	for i := 0; i < 20; i++ {
		buffers = append(buffers, ideenv.Buffer{Path: "f", Text: strings.Repeat("x", 4096)})
	}
	// Same path is fine for the cap math; the cleaner bounds count and bytes.
	ideenv.SetWithBuffers(nil, "", nil, buffers)
	got := ideenv.Get()
	if len(got.Buffers) == 0 || len(got.Buffers) > 8 {
		t.Fatalf("buffer count = %d, want 1..8", len(got.Buffers))
	}
	total := 0
	for _, b := range got.Buffers {
		if len(b.Text) > 8*1024 {
			t.Fatalf("buffer text %d bytes exceeds the per-buffer cap", len(b.Text))
		}
		total += len(b.Text)
	}
	if total > 32*1024 {
		t.Fatalf("total %d bytes exceeds the cap", total)
	}
}
