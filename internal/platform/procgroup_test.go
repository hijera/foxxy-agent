package platform

import (
	"os/exec"
	"testing"
	"time"
)

// startProbeHelper launches a long-running process that leads its own group,
// which is the shape ProcessGroupAlive is asked about in production. The
// returned cmd is kept by the caller on purpose: on Windows an exec.Cmd holds a
// handle to the process object, and a retained handle is exactly what makes a
// dead pid still resolvable.
func startProbeHelper(t *testing.T) (*exec.Cmd, int, time.Time) {
	t.Helper()

	sh := CurrentShell()
	var command string
	switch sh.Kind {
	case ShellPwsh, ShellPowerShell:
		command = "Start-Sleep -Seconds 600"
	case ShellBash, ShellSh:
		command = "sleep 600"
	default:
		t.Skipf("no portable sleep for shell %q", sh.Kind)
	}

	executable, args := sh.Command(command)
	cmd := exec.Command(executable, args...) // #nosec G204 -- fixed test command
	DetachProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a helper process: %v", err)
	}
	// identity is what the platform itself can prove about this pid, and it is
	// what termination has to be given. Killing the helper is not allowed to lean
	// on the substitute below.
	identity := ProcessStartedAt(cmd.Process.Pid)
	t.Cleanup(func() { _ = TerminateProcessGroupByPID(cmd.Process.Pid, identity, time.Second) })

	started := identity
	if started.IsZero() {
		// Unix does not need a process creation identity; the group probe ignores
		// the value. Keeping a real timestamp makes the helper useful there too.
		started = time.Now()
	}

	return cmd, cmd.Process.Pid, started
}

// waitForProbe polls until ProcessGroupAlive reports want, and returns whether
// it got there. Termination is not instantaneous on either platform.
func waitForProbe(pid int, startedAt time.Time, want bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ProcessGroupAlive(pid, startedAt) == want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return ProcessGroupAlive(pid, startedAt) == want
}

// waitForExit reaps the helper without letting a failed kill become a
// package-wide timeout. cmd.Wait blocks until the process actually exits, so
// calling it bare turns "termination did not work" into a hang that reports
// nothing: the helper sleeps for ten minutes, go test gives up long before that,
// and the panic names a test that never got to assert anything.
func waitForExit(t *testing.T, cmd *exec.Cmd, timeout time.Duration) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("process %d was still running %v after it was terminated", cmd.Process.Pid, timeout)
	}
}

func TestProcessGroupAliveRejectsNonPositivePIDs(t *testing.T) {
	for _, pid := range []int{0, -1, -4242} {
		if ProcessGroupAlive(pid, time.Now()) {
			t.Fatalf("ProcessGroupAlive(%d) = true, want false", pid)
		}
	}
}

func TestProcessGroupAliveFindsARunningProcess(t *testing.T) {
	_, pid, started := startProbeHelper(t)

	if !ProcessGroupAlive(pid, started) {
		t.Fatalf("ProcessGroupAlive(%d) = false for a process that is still running", pid)
	}
}

// The probe is called on every background_list, so it must not consume a
// resource per call. On Windows the natural implementation opens a handle, and
// leaking one both exhausts the table and keeps dead pids resolvable.
func TestProcessGroupAliveIsRepeatable(t *testing.T) {
	_, pid, started := startProbeHelper(t)

	for i := range 2000 {
		if !ProcessGroupAlive(pid, started) {
			t.Fatalf("ProcessGroupAlive(%d) = false on probe %d of a running process", pid, i)
		}
	}
}

// A reaped process is gone on both platforms: its group is empty on unix, and
// on Windows nothing holds its object open any more.
func TestProcessGroupAliveRejectsAProcessThatExited(t *testing.T) {
	cmd, pid, started := startProbeHelper(t)

	if err := TerminateProcessGroupByPID(pid, started, time.Second); err != nil {
		t.Fatalf("TerminateProcessGroupByPID(): %v", err)
	}
	waitForExit(t, cmd, 10*time.Second)

	if !waitForProbe(pid, started, false) {
		t.Fatalf("ProcessGroupAlive(%d) = true for a process that exited", pid)
	}
}

// A grace shorter than the kill itself is ordinary rather than exceptional: on
// Windows the kill is taskkill, and simply starting it and letting it walk one
// child costs about a second on an idle machine, so the three-second grace the
// task pool passes is a coin flip on a loaded one. Termination that gives up
// when its own grace expires is not termination, and reporting success for it is
// worse than reporting failure - the caller stops looking at a process that is
// still running. This pins the outcome rather than the mechanism: whatever the
// grace, a nil error means the process is gone.
func TestTerminateProcessGroupByPIDKillsWhenTheGraceExpiresFirst(t *testing.T) {
	cmd, pid, started := startProbeHelper(t)

	if err := TerminateProcessGroupByPID(pid, started, time.Millisecond); err != nil {
		t.Fatalf("TerminateProcessGroupByPID(): %v", err)
	}
	waitForExit(t, cmd, 10*time.Second)

	if !waitForProbe(pid, started, false) {
		t.Fatalf("ProcessGroupAlive(%d) = true after a termination that returned nil", pid)
	}
}

// The probe runs on every background_list, against processes that are meant to
// keep running - a dev server, a watcher, a build. Asking the OS whether one of
// those has exited must never be answered by waiting until it does. The margin
// here is enormous on purpose: the helper sleeps for ten minutes, so anything
// that blocks at all blows a one-second budget by orders of magnitude, and the
// bound is nowhere near tight enough to trip on a slow runner.
func TestProcessGroupAliveDoesNotWaitOnARunningProcess(t *testing.T) {
	_, pid, started := startProbeHelper(t)

	begin := time.Now()
	alive := ProcessGroupAlive(pid, started)
	elapsed := time.Since(begin)

	if !alive {
		t.Fatalf("ProcessGroupAlive(%d) = false for a process that is still running", pid)
	}
	if elapsed > time.Second {
		t.Fatalf("ProcessGroupAlive(%d) took %v; the probe must not wait on a running process", pid, elapsed)
	}
}
