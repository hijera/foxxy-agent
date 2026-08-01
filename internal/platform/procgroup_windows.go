//go:build windows

package platform

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess reports for a process that has
// not exited yet (STILL_ACTIVE / STATUS_PENDING, 0x103). x/sys/windows does not
// export it under that name.
const stillActive = 259

// DetachProcessGroup puts the command in its own console process group so that
// TerminateProcessGroup can reach the whole tree a shell spawns, not just the
// shell itself.
func DetachProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// TerminateProcessGroup kills the process tree started by cmd. Windows has no
// graceful group signal comparable to SIGTERM for a non-console child, so the
// grace period only bounds how long taskkill is given before the process handle
// is closed directly. A process that already exited is not an error.
func TerminateProcessGroup(cmd *exec.Cmd, grace time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := strconv.Itoa(cmd.Process.Pid)

	kill := exec.Command("taskkill", "/T", "/F", "/PID", pid)
	if err := kill.Start(); err == nil {
		done := make(chan struct{})
		go func() {
			_ = kill.Wait()
			close(done)
		}()
		if grace <= 0 {
			grace = 5 * time.Second
		}
		select {
		case <-done:
			return nil
		case <-time.After(grace):
			_ = kill.Process.Kill()
		}
	}

	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// ProcessGroupAlive reports whether the process still exists. Windows has no
// process group to probe the way a unix signal 0 does, so this opens a handle
// to the pid and asks for its exit code: only STILL_ACTIVE means running. Its
// children are handled by taskkill /T when terminating.
//
// os.FindProcess is deliberately not used (upstream does), because it answers
// "can I open this pid", not "is it running", and that differs in both
// directions:
//
//   - It opens with PROCESS_QUERY_INFORMATION, which is denied for a process in
//     another security context. A survivor started elevated or by another user
//     would be reported dead and never reaped.
//   - A process object outlives the process itself while any handle stays open,
//     so an exited child is reported alive until its last handle is closed.
//
// PROCESS_QUERY_LIMITED_INFORMATION is granted across integrity levels, and the
// exit code distinguishes a released process object from a running one. Access
// denied even for the limited right means the process exists but is protected,
// which still counts as alive.
//
// Deliberately no Wait() here - on Windows that blocks until a live process
// exits, which would hang a liveness probe.
func ProcessGroupAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// A pid that was never valid, or one whose process object is already
		// gone, cannot be reached. Access denied means the process does exist
		// but belongs to another security context, so treat it as alive.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// TerminateProcessGroupByPID kills a tree this process did not start, which is
// what reaping survivors of a previous run needs.
func TerminateProcessGroupByPID(pid int, grace time.Duration) error {
	if pid <= 0 {
		return nil
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	if err := kill.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		_ = kill.Wait()
		close(done)
	}()
	if grace <= 0 {
		grace = 5 * time.Second
	}
	select {
	case <-done:
		return nil
	case <-time.After(grace):
		_ = kill.Process.Kill()
		return nil
	}
}
