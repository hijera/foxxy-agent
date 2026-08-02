//go:build windows

package platform

import (
	"os/exec"
	"testing"
	"time"
)

// TestProcessGroupAliveRejectsExitedProcessWithOpenHandle pins the fork's
// divergence from upstream. A Windows process object outlives the process while
// any handle stays open, so upstream's os.FindProcess probe answers "yes" for a
// child that has already exited but has not been Wait()ed on. bgtask holds
// exactly such a handle for every task it started, so the probe has to read the
// exit code rather than merely open the pid.
func TestProcessGroupAliveRejectsExitedProcessWithOpenHandle(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit", "0")
	DetachProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	// Wait() is deliberately deferred, not called now: it closes the handle and
	// releases the process object, which is what makes upstream's probe look
	// correct.
	t.Cleanup(func() { _ = cmd.Wait() })

	deadline := time.Now().Add(5 * time.Second)
	for ProcessGroupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if ProcessGroupAlive(pid) {
		t.Fatalf("pid %d still reported alive after the process exited", pid)
	}
}

// TestProcessGroupAliveReportsRunningProcess is the other half: a process that
// is still running must be reported alive, or survivors would never be reaped.
func TestProcessGroupAliveReportsRunningProcess(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "ping", "-n", "10", "127.0.0.1")
	DetachProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = TerminateProcessGroup(cmd, time.Second)
		_ = cmd.Wait()
	})

	if !ProcessGroupAlive(pid) {
		t.Fatalf("pid %d reported dead while the process is running", pid)
	}
}

// TestProcessGroupAliveReportsProtectedProcess covers the other direction of the
// same divergence: PROCESS_QUERY_INFORMATION (what os.FindProcess asks for) is
// denied across security contexts, so upstream reports a live elevated process
// as dead and a reap would skip it. pid 4 is the Windows System process, which
// always exists and is always protected.
func TestProcessGroupAliveReportsProtectedProcess(t *testing.T) {
	const systemPID = 4
	if !ProcessGroupAlive(systemPID) {
		t.Fatalf("System process (pid %d) reported dead", systemPID)
	}
}

func TestProcessGroupAliveRejectsNonPositivePID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if ProcessGroupAlive(pid) {
			t.Fatalf("pid %d reported alive", pid)
		}
	}
}
