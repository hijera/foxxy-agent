//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

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
		done := make(chan error, 1)
		go func() { done <- kill.Wait() }()
		if grace <= 0 {
			grace = 5 * time.Second
		}
		select {
		case waitErr := <-done:
			if waitErr == nil {
				return nil
			}
			// taskkill refused the tree: enumeration failed, the pid vanished
			// while it walked, or access was denied. Its exit status is the
			// only report of that, so a non-zero one falls through to killing
			// the leader rather than being read as success.
		case <-time.After(grace):
			// taskkill is deliberately left running: it walks the tree on its
			// own and finishes that work even if this process exits first,
			// which is the only thing that reaches grandchildren. Killing it
			// here would abandon the tree to save a short-lived helper.
		}
	}

	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

// ProcessGroupAlive reports whether the process the record describes is still
// running. startedAt is the exact creation time captured from Windows when the
// task process was launched, and is what tells the process apart from a stranger
// that inherited its pid.
//
// Windows has no process group to probe the way a unix signal 0 does, and the
// obvious substitute - opening the process by pid - answers a different
// question in two damaging ways. A process object outlives its own process for
// as long as anybody holds a handle to it, and os.FindProcess opens one per call
// and leaves it to the runtime to close, so probing a pid is itself what keeps a
// corpse resolvable; and opening by number matches any process at all, where the
// unix probe only ever matches a group leader and so filters out most pid reuse
// for free. Both mistakes end at background_reap running taskkill /T /F on the
// wrong tree.
//
// So this asks two questions instead. WaitForSingleObject with a zero timeout
// reports whether the process object is signalled, which is the difference
// between a running process and a retained corpse - it is not os.Process.Wait,
// which would block until a live process exits and hang the probe.
// GetProcessTimes then confirms the process has the exact creation time captured
// when the task launched. Children are handled by taskkill /T when terminating.
func ProcessGroupAlive(pid int, startedAt time.Time) bool {
	if pid <= 0 {
		return false
	}

	// PROCESS_QUERY_LIMITED_INFORMATION is the narrowest right that answers the
	// question, so a child this run may not open with the wider rights
	// os.FindProcess asks for does not read as gone. It is not a guarantee:
	// access is still the target's to refuse. Failing to open means the pid
	// cannot be shown to be this task, and an unproven pid is not one to offer
	// for killing.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	// WAIT_OBJECT_0 means the process is signalled, which for a process object
	// means it has exited. WAIT_TIMEOUT means it is still running.
	event, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || event != uint32(windows.WAIT_TIMEOUT) {
		return false
	}

	return processStartedAround(handle, startedAt)
}

// ProcessStartedAt returns the exact Windows creation time of pid. A zero value
// means the process identity could not be proven and must not later be used to
// authorize terminating a persisted pid.
func ProcessStartedAt(pid int) time.Time {
	if pid <= 0 {
		return time.Time{}
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return time.Time{}
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	startedAt, err := processStartedAt(handle)
	if err != nil {
		return time.Time{}
	}
	return startedAt
}

// processStartedAround reports whether the open process has the exact identity
// recorded when the task was launched. Any missing or unreadable identity fails
// closed because a false positive is handed to taskkill /T /F.
func processStartedAround(handle windows.Handle, startedAt time.Time) bool {
	if startedAt.IsZero() {
		return false
	}
	actual, err := processStartedAt(handle)
	return err == nil && actual.Equal(startedAt)
}

func processStartedAt(handle windows.Handle) (time.Time, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, creation.Nanoseconds()), nil
}

const (
	// defaultTerminateGrace applies when the caller names no budget.
	defaultTerminateGrace = 5 * time.Second
	// minTaskkillWait is the floor for taskkill's own runtime. Spawning a process
	// on Windows can take the better part of a second under load, and cutting the
	// helper short leaves the target alive - the failure this floor exists for.
	minTaskkillWait = 5 * time.Second
	// terminatePollInterval paces the "is it gone yet" probe.
	terminatePollInterval = 20 * time.Millisecond
)

// TerminateProcessGroupByPID kills a tree this process did not start, which is
// what reaping survivors of a previous run needs. startedAt is the identity the
// record persisted, and it is checked here rather than trusted from the caller's
// earlier probe.
//
// The verified handle is deliberately held open for the whole of taskkill.
// Windows keeps a pid allocated for as long as any handle to its process object
// is open, so holding one is what stops the number from being handed to a
// stranger between the check and the kill - and it also keeps taskkill /T
// resolving the tree by the parent pid that the children actually have. Without
// it the identity check would only narrow the window rather than close it.
//
// Liveness is deliberately not required: a leader that has already exited can
// still have running children, and those are exactly what reaping is for.
//
// grace is how long the leader is given to disappear, and a nil return means it
// did. It is not a budget for taskkill's own runtime - applying it there is what
// made this function report success over a process that was still running.
func TerminateProcessGroupByPID(pid int, startedAt time.Time, grace time.Duration) error {
	if pid <= 0 {
		return nil
	}

	// Query rights only. SYNCHRONIZE buys nothing here - nothing waits on the
	// object - and every right asked for is one more the target may refuse.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Nothing answers to this pid any more, so there is nothing to kill and
		// nothing was left running. Same outcome as ESRCH on unix.
		return nil
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	if !processStartedAround(handle, startedAt) {
		return fmt.Errorf("pid %d is not the process the task recorded", pid)
	}

	if grace <= 0 {
		grace = defaultTerminateGrace
	}

	// grace bounds how long the TARGET may take to die, not how long taskkill may
	// run. Those are not the same thing, and conflating them is what made this
	// function lie: taskkill.exe cold-starting under load routinely needs more
	// than a second, the helper was killed mid-flight, and the caller was told the
	// group was gone while it went on running. `/F` has no graceful phase to wait
	// out, so the helper gets a floor of its own.
	killWait := grace
	if killWait < minTaskkillWait {
		killWait = minTaskkillWait
	}
	ctx, cancel := context.WithTimeout(context.Background(), killWait)
	defer cancel()
	// taskkill's exit status is not evidence: it reports "not found" for a process
	// that died on its own, and success before the kernel has finished tearing the
	// tree down. The probe below is the only thing that settles it.
	_ = exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()

	deadline := time.Now().Add(grace)
	for {
		if !ProcessGroupAlive(pid, startedAt) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("pid %d is still running %s after taskkill /T /F", pid, grace)
		}
		time.Sleep(terminatePollInterval)
	}
}
