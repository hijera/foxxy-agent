package logger

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// The process log carries whatever the diagnostics layer captures — with
// debug.capture_llm on that includes raw LLM request and response bodies: the
// full prompt, file contents the agent read, and anything the user pasted into
// the chat. A world-readable log file hands all of that to every other local
// account, so the file must be owner-only.
func TestRotatingFileIsOwnerReadableOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "foxxycode.log")
	rf, err := newRotatingFile(path, config.LoggerRotation{MaxSizeMB: 1, MaxFiles: 2})
	if err != nil {
		t.Fatalf("newRotatingFile: %v", err)
	}
	defer func() { _ = rf.Close() }()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log file mode = %#o, want 0600", perm)
	}
}

// An upgrade must not leave the previous release's world-readable log in place:
// the file is long-lived and keeps collecting new entries, so opening it also
// tightens it.
func TestRotatingFileTightensExistingLooseMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "foxxycode.log")
	if err := os.WriteFile(path, []byte("old entry\n"), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	rf, err := newRotatingFile(path, config.LoggerRotation{MaxSizeMB: 1, MaxFiles: 2})
	if err != nil {
		t.Fatalf("newRotatingFile: %v", err)
	}
	defer func() { _ = rf.Close() }()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("pre-existing log mode = %#o, want it tightened to 0600", perm)
	}
}

// Rotation opens a fresh file through the same path, so the replacement must be
// owner-only too — otherwise the tightening only lasts until the first rotation.
func TestRotatedFileKeepsOwnerOnlyMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "foxxycode.log")
	rf, err := newRotatingFile(path, config.LoggerRotation{MaxSizeMB: 1, MaxFiles: 3})
	if err != nil {
		t.Fatalf("newRotatingFile: %v", err)
	}
	defer func() { _ = rf.Close() }()

	// Force a rotation without writing a megabyte: rotate() is what reopens the file.
	if err := rf.rotate(); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	for _, name := range []string{"foxxycode.log", "foxxycode.log.1"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("%s mode = %#o, want 0600", name, perm)
		}
	}
}
