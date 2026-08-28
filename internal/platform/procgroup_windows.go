//go:build windows

package platform

import (
	"errors"
	"fmt"
	"math"
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
	HideConsoleWindow(kill)
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

	// A process object that is not yet signalled is one whose process is still
	// running. Anything else - signalled, or a wait that could not answer at all -
	// is not a process to offer for killing.
	if _, running := waitProcess(handle, 0); !running {
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

// TerminateProcessGroupByPID kills a tree this process did not start, which is
// what reaping survivors of a previous run needs. startedAt is the identity the
// record persisted, and it is checked here rather than trusted from the caller's
// earlier probe.
//
// The verified handle is deliberately held open for the whole of the kill.
// Windows keeps a pid allocated for as long as any handle to its process object
// is open, so holding one is what stops the number from being handed to a
// stranger between the check and the kill - and it also keeps taskkill /T
// resolving the tree by the parent pid that the children actually have. Without
// it the identity check would only narrow the window rather than close it.
//
// grace bounds how long taskkill is given, not how long this may report success
// without having killed anything. taskkill is a heavy helper - starting it and
// letting it walk one child costs about a second on an idle machine - so a grace
// that expires first is ordinary rather than exceptional, and a grace shorter
// than that is what the caller asked for. When it expires, taskkill is left
// running on purpose: it finishes walking the tree even if nothing is watching
// any more, and that walk is the only thing that reaches grandchildren. The
// leader is then terminated directly, through the pid the held handle has
// pinned. Killing taskkill instead would abandon the tree to save a short-lived
// helper and leave the leader running behind a nil error, which is worse than
// either failure alone: background_reap reports a survivor as reaped and stops
// looking at it.
//
// Liveness is deliberately not required: a leader that has already exited can
// still have running children, and those are exactly what reaping is for.
func TerminateProcessGroupByPID(pid int, startedAt time.Time, grace time.Duration) error {
	if pid <= 0 {
		return nil
	}

	// Query and wait rights only. PROCESS_TERMINATE is asked for later and
	// separately, so that a target refusing it still costs nothing here: every
	// right asked for up front is one more that can fail the open, and a failed
	// open loses the identity check and the taskkill attempt with it.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
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
		grace = 5 * time.Second
	}
	deadline := time.Now().Add(grace)

	done := make(chan struct{})
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	HideConsoleWindow(kill)
	if err := kill.Start(); err != nil {
		// No tree walk this run; the leader below is all that can be reached.
		close(done)
	} else {
		go func() {
			_ = kill.Wait()
			close(done)
		}()
	}

	select {
	case <-done:
	case <-time.After(time.Until(deadline)):
	}

	// Whether taskkill finished, refused the tree, or is still walking it, its
	// exit status is not the answer to whether anything is still running. The
	// process object is, so that is what the rest of the grace waits on.
	if exited, _ := waitProcess(handle, time.Until(deadline)); exited {
		return nil
	}
	return terminateProcess(pid, handle)
}

// confirmExit bounds how long a refused termination is given to turn out to have
// been unnecessary. It is not a second grace period - nothing is being waited out
// politely here - only the short time a kernel teardown that is already under way
// needs to reach the process object.
const confirmExit = 2 * time.Second

// terminateProcess kills the leader directly, for when taskkill did not reach it
// within the grace. The pid is not racy here even though it is opened a second
// time: the caller still holds a handle to the process object it verified, and
// Windows keeps a pid allocated for as long as any handle to it is open, so this
// open cannot land on a stranger.
func terminateProcess(pid int, verified windows.Handle) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("terminate pid %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	if err := windows.TerminateProcess(handle, 1); err != nil {
		// Windows refuses to terminate a process that has already exited or is
		// already terminating, and reports both as access denied. That is the
		// ordinary outcome of the grace running out while taskkill was mid-kill,
		// not a failure to kill anything, so the refusal is re-read against the
		// process object once the teardown it is complaining about has had time
		// to finish. Reading it as an error instead would report a process that
		// is on its way out as a survivor nothing could touch.
		if exited, _ := waitProcess(verified, confirmExit); exited {
			return nil
		}
		return fmt.Errorf("terminate pid %d: %w", pid, err)
	}
	return nil
}

// waitProcess reports the state of a process object, waiting at most timeout for
// it to settle: a process object is signalled exactly when the process has
// exited, and unsignalled while it runs.
//
// Both answers are returned rather than one being the other's negation, because
// a wait that fails outright proves neither, and the two callers have to fail
// closed towards opposite answers: an unproven process is not alive enough to
// offer for killing, and it is also not dead enough to stop killing.
func waitProcess(handle windows.Handle, timeout time.Duration) (exited, running bool) {
	event, err := windows.WaitForSingleObject(handle, waitMilliseconds(timeout))
	if err != nil {
		return false, false
	}
	return event == uint32(windows.WAIT_OBJECT_0), event == uint32(windows.WAIT_TIMEOUT)
}

// waitMilliseconds converts a timeout into the count WaitForSingleObject takes,
// clamped so that it can never come out as INFINITE. A probe that waits forever
// on a process that is still running is the whole failure this file exists to
// avoid, and it must not be reachable by arithmetic.
func waitMilliseconds(timeout time.Duration) uint32 {
	if timeout <= 0 {
		return 0
	}
	const longestWait = time.Duration(math.MaxInt32) * time.Millisecond
	if timeout > longestWait {
		timeout = longestWait
	}
	return uint32(timeout.Milliseconds())
}
