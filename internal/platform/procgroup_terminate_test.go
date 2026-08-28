package platform

import (
	"testing"
	"time"
)

// TestTerminateProcessGroupActuallyKillsWithATightGrace is the regression for a
// hang that made the whole package time out on Windows roughly five runs in six.
//
// The Windows implementation shelled out to taskkill and applied `grace` to
// *taskkill's own runtime*: if the helper had not finished within it, taskkill was
// killed and the function returned nil. Spawning taskkill.exe under load routinely
// takes longer than a second, so the target survived while the caller was told the
// group was gone — and the next `cmd.Wait()` blocked on a process that was still
// sleeping for ten minutes.
//
// A tight grace is exactly the case that used to fail, so the test uses one.
func TestTerminateProcessGroupActuallyKillsWithATightGrace(t *testing.T) {
	for attempt := range 5 {
		cmd, pid, started := startProbeHelper(t)

		if err := TerminateProcessGroupByPID(pid, started, 50*time.Millisecond); err != nil {
			t.Fatalf("attempt %d: TerminateProcessGroupByPID(): %v", attempt, err)
		}
		// Reporting success has to mean the process is gone. A caller told
		// "stopped" while it still runs will rebind its port or double-write.
		if ProcessGroupAlive(pid, started) {
			t.Fatalf("attempt %d: terminate reported success but pid %d is still alive", attempt, pid)
		}

		// Wait must return promptly now; before the fix this blocked for the
		// helper's full sleep and took the package's timeout with it.
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("attempt %d: cmd.Wait() still blocked 10s after a successful terminate", attempt)
		}
	}
}

// TestTerminateProcessGroupIsQuietAboutAnUnknownPID keeps the "nothing to do"
// path silent: a task whose process is already reaped must not report an error.
func TestTerminateProcessGroupIsQuietAboutAnUnknownPID(t *testing.T) {
	// A pid that cannot be running: 0 and negatives are rejected outright.
	if err := TerminateProcessGroupByPID(0, time.Now(), time.Second); err != nil {
		t.Errorf("terminate(0) = %v, want nil", err)
	}
	if err := TerminateProcessGroupByPID(-7, time.Now(), time.Second); err != nil {
		t.Errorf("terminate(-7) = %v, want nil", err)
	}
}
