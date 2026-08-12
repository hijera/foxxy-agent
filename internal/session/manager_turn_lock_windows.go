//go:build windows

package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Cross-process turn lock for Windows.
//
// Without this the lock degrades to a per-process mutex, and two IDE windows are two backend
// processes over one shared home: both accept a turn for the same session, both stream, and
// both persist - so whichever finishes last silently overwrites the other's transcript. That
// is not a rare race; it reproduces on the first try.
//
// LockFileEx rather than a pid file: the OS releases the range when the owning process dies,
// however it dies, so a crashed IDE cannot leave a session wedged.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	// errLockViolation is what LockFileEx reports for "someone else holds it" when asked not
	// to block. ERROR_IO_PENDING cannot occur here: the handle is not opened for overlapped IO.
	errLockViolation = syscall.Errno(33)
)

type turnLockMeta struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
}

func (m *Manager) acquirePromptTurnLock(sessionID string, st *State) (unlock func(), err error) {
	dir := strings.TrimSpace(st.GetPersistedSessionDir())
	if dir == "" {
		// No bundle on disk, so there is nothing for another process to contend over.
		return m.acquireStubTurnLock(sessionID)
	}
	// Both locks: the file lock keeps other processes out, the in-process mutex keeps this
	// process out of its own lock (a second LockFileEx from the same process would succeed).
	stubUnlock, err := m.acquireStubTurnLock(sessionID)
	if err != nil {
		return nil, err
	}
	fileUnlock, err := acquireTurnLockFile(dir)
	if err != nil {
		stubUnlock()
		return nil, err
	}
	return func() {
		fileUnlock()
		stubUnlock()
	}, nil
}

// TurnLockHeld reports whether any process currently holds the turn lock for this bundle.
func TurnLockHeld(sessionDir string) bool {
	dir := strings.TrimSpace(sessionDir)
	if dir == "" {
		return false
	}
	path := filepath.Join(dir, turnLockFileName)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if err := lockFileRange(syscall.Handle(f.Fd()), lockfileExclusiveLock|lockfileFailImmediately); err != nil {
		return errors.Is(err, errLockViolation)
	}
	_ = unlockFileRange(syscall.Handle(f.Fd()))
	return false
}

func acquireTurnLockFile(sessionDir string) (unlock func(), err error) {
	path := filepath.Join(sessionDir, turnLockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("turn lock open: %w", err)
	}
	h := syscall.Handle(f.Fd())
	if err := lockFileRange(h, lockfileExclusiveLock|lockfileFailImmediately); err != nil {
		_ = f.Close()
		if errors.Is(err, errLockViolation) {
			return nil, ErrSessionTurnBusy
		}
		return nil, fmt.Errorf("turn lock: %w", err)
	}
	// Best-effort breadcrumb for whoever debugs a wedged session; the lock itself is the
	// file range, not this content.
	if b, mErr := json.Marshal(turnLockMeta{
		PID:     os.Getpid(),
		Started: time.Now().UTC().Format(time.RFC3339),
	}); mErr == nil {
		_ = f.Truncate(0)
		_, _ = f.WriteAt(b, 0)
	}
	return func() {
		_ = unlockFileRange(h)
		_ = f.Close()
	}, nil
}

// lockFileRange locks the first byte of the file. Locking one byte rather than the whole
// range keeps the call independent of the file's size, which changes as the meta is written.
func lockFileRange(h syscall.Handle, flags uint32) error {
	var overlapped syscall.Overlapped
	r1, _, e1 := procLockFileEx.Call(
		uintptr(h),
		uintptr(flags),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		if e1 != nil {
			return e1
		}
		return errors.New("LockFileEx failed")
	}
	return nil
}

func unlockFileRange(h syscall.Handle) error {
	var overlapped syscall.Overlapped
	r1, _, e1 := procUnlockFileEx.Call(
		uintptr(h),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		if e1 != nil {
			return e1
		}
		return errors.New("UnlockFileEx failed")
	}
	return nil
}
