package bgtask

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A background command's captured output is mirrored to disk verbatim, so it
// holds whatever the command printed — build logs, tool output, tokens echoed by
// a misbehaving script. On a shared Unix host that must not be world-readable.
func TestAttachFileIsOwnerReadableOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "output.log")
	sink := NewOutputSink(1024)
	if err := sink.AttachFile(path); err != nil {
		t.Fatalf("AttachFile: %v", err)
	}
	defer sink.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("output file mode = %#o, want 0600", perm)
	}
}
