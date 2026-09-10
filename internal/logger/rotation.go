package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// logFileMode keeps log files readable by their owner only. The process log
// carries whatever the diagnostics layer captures, and with debug.capture_llm on
// that includes raw LLM request and response bodies — prompts, file contents,
// anything pasted into the chat. On a shared Unix host 0644 would expose all of
// it to every other local account.
const logFileMode = 0o600

// rotatingFile is a size-bounded file writer that rotates on writes once the
// current file exceeds Rotation.MaxSizeMB. Writes are serialised so the
// rotation is safe across goroutines.
//
// Rotation policy: when the current file would grow past LoggerRotation.MaxSizeMB, the
// existing files are renamed in chain (foo.log.(N-1) -> foo.log.N, ...,
// foo.log -> foo.log.1) and a fresh empty foo.log is opened. Files past
// MaxFiles are deleted.
type rotatingFile struct {
	path     string
	maxSize  int64 // bytes; 0 disables rotation
	maxFiles int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// newRotatingFile opens (or creates) the file at path. The directory must
// already exist; the caller is expected to MkdirAll if needed.
func newRotatingFile(path string, r config.LoggerRotation) (*rotatingFile, error) {
	if path == "" {
		return nil, errors.New("rotatingFile: empty path")
	}
	rf := &rotatingFile{
		path:     path,
		maxSize:  int64(r.MaxSizeMB) * 1024 * 1024,
		maxFiles: r.MaxFiles,
	}
	if err := rf.open(); err != nil {
		return nil, err
	}
	rf.tightenRotatedFiles()
	return rf, nil
}

// tightenRotatedFiles applies the owner-only mode to backups left by a previous
// release too. open only reaches the live path; without this pass, foo.log.1
// and friends can continue exposing old captured prompts after an upgrade.
func (rf *rotatingFile) tightenRotatedFiles() {
	dir := filepath.Dir(rf.path)
	prefix := filepath.Base(rf.path) + "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		suffix := strings.TrimPrefix(name, prefix)
		if n, err := strconv.Atoi(suffix); err != nil || n < 1 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		_ = os.Chmod(filepath.Join(dir, name), logFileMode)
	}
}

// open (re)opens the file and refreshes the cached size.
func (rf *rotatingFile) open() error {
	if err := os.MkdirAll(filepath.Dir(rf.path), 0o755); err != nil {
		return fmt.Errorf("rotatingFile: mkdir %s: %w", filepath.Dir(rf.path), err)
	}
	f, err := os.OpenFile(rf.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		return fmt.Errorf("rotatingFile: open %s: %w", rf.path, err)
	}
	// The mode above only applies when the file is created, so a log written by
	// an older release keeps its world-readable bits. Tighten it on every open:
	// with debug.capture_llm on this file holds raw prompts and responses, and
	// the process usually keeps appending to the same path for weeks.
	// Best effort: a filesystem without permission support (or Windows, where
	// Chmod only carries the read-only bit) is no reason to refuse to log.
	_ = f.Chmod(logFileMode)
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("rotatingFile: stat %s: %w", rf.path, err)
	}
	rf.f = f
	rf.size = info.Size()
	return nil
}

// Write implements io.Writer. Rotates after the write if the threshold was
// crossed, so each line stays intact even when it pushes past the boundary.
func (rf *rotatingFile) Write(p []byte) (int, error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	n, err := rf.f.Write(p)
	rf.size += int64(n)
	if err != nil {
		return n, err
	}
	if rf.maxSize > 0 && rf.size >= rf.maxSize {
		if rerr := rf.rotate(); rerr != nil {
			return n, fmt.Errorf("rotatingFile: rotate: %w", rerr)
		}
	}
	return n, nil
}

// Close flushes and closes the underlying file.
func (rf *rotatingFile) Close() error {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.f == nil {
		return nil
	}
	err := rf.f.Close()
	rf.f = nil
	return err
}

// rotate renames foo.log -> foo.log.1, foo.log.1 -> foo.log.2, ..., deletes
// the file beyond maxFiles, and opens a fresh foo.log. Caller must hold mu.
func (rf *rotatingFile) rotate() error {
	if err := rf.f.Close(); err != nil {
		return err
	}
	rf.f = nil

	// Drop the oldest backup if it pushes us over the limit.
	if rf.maxFiles > 0 {
		oldest := fmt.Sprintf("%s.%d", rf.path, rf.maxFiles)
		_ = os.Remove(oldest)
	}

	// Shift existing backups: .(N-1) -> .N, ..., .1 -> .2.
	for i := rf.maxFiles - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", rf.path, i)
		to := fmt.Sprintf("%s.%d", rf.path, i+1)
		if _, err := os.Stat(from); err == nil {
			if err := os.Rename(from, to); err != nil {
				return err
			}
		}
	}

	// Move the current file to .1 (only if backups are kept).
	if rf.maxFiles >= 1 {
		if err := os.Rename(rf.path, rf.path+".1"); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else {
		// MaxFiles == 0: no backups, just truncate the live file.
		_ = os.Remove(rf.path)
	}

	return rf.open()
}

// Compile-time check.
var _ io.WriteCloser = (*rotatingFile)(nil)
