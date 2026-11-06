package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/internal/platform"
)

// stateFileLocks holds one mutex per state file for the whole process, so
// every write of one path queues behind the others whichever TrustStore or
// caller it comes through.
var stateFileLocks sync.Map

// updateStateFile runs one read-modify-write cycle of a state file in the
// foxxycode home (mcp-trust.json, mcp-overrides.json) against every other writer
// of it. The process-wide mutex of the path covers goroutines; the lock on
// path+".lock" covers the other processes sharing the home - the console,
// foxxycode serve, foxxycode mcp trust - which would otherwise replace the file with
// a copy read before another writer's change landed. Readers take neither:
// every write replaces the file whole (writeStateFile).
func updateStateFile(path string, update func() error) error {
	mu, _ := stateFileLocks.LoadOrStore(path, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	unlock, err := platform.LockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	return update()
}

// writeStateFile replaces path with data through a temporary file of its own
// next to it, readable by the operator only: two writers never share a
// temporary, and a reader sees the old file or the new one, never a
// half-written one. The replace waits out the unlocked readers of the file -
// readOverrides on every list and dial - whose handles make it fail on
// Windows (renameWithRetry).
func writeStateFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := renameWithRetry(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// renameWithRetry renames tmpPath over path, retrying briefly on transient
// Windows sharing errors that occur when another handle holds the destination
// open. On POSIX isRetryableFileLockError is always false, so this is a single
// os.Rename with no behavior change. The document stays a temporary-plus
// rename throughout: a crash mid-write cannot leave a torn file behind.
func renameWithRetry(tmpPath, path string) error {
	const maxAttempts = 20
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err = os.Rename(tmpPath, path); err == nil || !isRetryableFileLockError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
	return err
}
