package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeStateFile replaces the state file through a temporary of its own, and
// every reader of it - readOverrides on each list and dial, another process
// sharing the home - is a plain os.ReadFile. Windows hands such a reader a
// handle that shares read and write but not delete, so a replace over it
// fails with a sharing violation ("replace ...: Access is denied") for as
// long as the reader is in flight. The write waits that reader out.
func TestWriteStateFileWaitsOutAReaderHoldingTheDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-overrides.json")
	if err := os.WriteFile(path, []byte(`{"workspaces":{"old":{}}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	done := make(chan error, 1)
	go func() { done <- writeStateFile(path, []byte(`{"workspaces":{"new":{}}}`+"\n")) }()
	// Hold the reader across the writer's replace, then let it go inside
	// the retry budget (well under its ~210ms). Without the wait the replace
	// fails at once; without the reader it never overlaps the write.
	time.Sleep(80 * time.Millisecond)
	_ = reader.Close()
	if err := <-done; err != nil {
		t.Fatalf("writeStateFile over a reader that lets go: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"new"`) {
		t.Fatalf("state file = %s, want the new document", data)
	}
}

// A reader that never lets go still fails the replace - a write must not
// quietly tear the file in place to get around one - but only after the retry
// budget, which is what makes the strategy visible without a race of its own.
func TestWriteStateFileReportsAReaderThatNeverLetsGo(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("POSIX replaces a destination another handle holds open")
	}
	path := filepath.Join(t.TempDir(), "mcp-overrides.json")
	if err := os.WriteFile(path, []byte(`{"workspaces":{}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	start := time.Now()
	err = writeStateFile(path, []byte(`{"workspaces":{"new":{}}}`+"\n"))
	if err == nil {
		t.Fatal("a reader that never lets go must not be replaced over")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v, want it to name %s", err, path)
	}
	// Without the retry the replace fails on its first attempt, in well
	// under a millisecond; a lower bound cannot flake by being slow.
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("replace gave up after %v, want the retry budget to have run", elapsed)
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if strings.Contains(string(data), `"new"`) {
		t.Fatalf("a failed replace left %s behind", data)
	}
}
