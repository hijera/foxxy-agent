//go:build windows

package platform

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

const (
	treeProbeEnv    = "FOXXYCODE_PROCTREE_PROBE"
	treeProbeParent = "parent"
	treeProbeChild  = "child"
)

// TestProcessTreeProbeHelper is not a test of its own: it is the tree the tests
// below need, and it does nothing unless one of them started it. "parent" spawns
// a child and then outlives nothing in particular; the sleep only has to be
// longer than the test, because the test is what ends both.
func TestProcessTreeProbeHelper(t *testing.T) {
	role := os.Getenv(treeProbeEnv)
	if role == "" {
		t.Skip("child half of the process tree tests")
	}
	if role == treeProbeParent {
		child := exec.Command(os.Args[0], "-test.run", "^TestProcessTreeProbeHelper$") // #nosec G204 -- the test binary itself
		child.Env = append(os.Environ(), treeProbeEnv+"="+treeProbeChild)
		if err := child.Start(); err != nil {
			t.Fatalf("spawn the child half: %v", err)
		}
	}
	time.Sleep(2 * time.Minute)
}

// startProbeTree starts a two-deep process tree and returns its leader. The
// child is deliberately not waited on by anybody: killing the leader leaves it
// running, which is the state the whole type exists for - Chrome's crashpad
// handler is exactly this, a child that keeps a file in the profile directory
// mapped for a moment after the browser process it belonged to is gone.
func startProbeTree(t *testing.T) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run", "^TestProcessTreeProbeHelper$") // #nosec G204 -- the test binary itself
	cmd.Env = append(os.Environ(), treeProbeEnv+"="+treeProbeParent)
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a helper process: %v", err)
	}
	t.Cleanup(func() {
		// Belt and braces: whatever the test did or failed to do, neither half is
		// left sleeping out its two minutes.
		leftovers := CaptureProcessTree(cmd.Process.Pid)
		defer leftovers.Release()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		leftovers.TerminateSurvivors()
	})

	// The leader spawns its child as the first thing it does, but "first thing"
	// still trails a Go test binary's own start-up, so wait for the tree to
	// actually be two deep rather than racing it.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		tree := CaptureProcessTree(cmd.Process.Pid)
		captured := len(tree.procs)
		tree.Release()
		if captured > 0 {
			return cmd
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the helper process never spawned a child")
	return nil
}

// The capture is the half that has to happen while the leader is alive, and
// finding nothing would make everything after it pass for the wrong reason.
func TestCaptureProcessTreeFindsAChildOfTheProcess(t *testing.T) {
	cmd := startProbeTree(t)

	tree := CaptureProcessTree(cmd.Process.Pid)
	defer tree.Release()

	if len(tree.procs) == 0 {
		t.Fatalf("CaptureProcessTree(%d) captured nothing for a process with a child", cmd.Process.Pid)
	}
	for _, p := range tree.procs {
		if p.pid == cmd.Process.Pid {
			t.Errorf("CaptureProcessTree(%d) captured the process itself", cmd.Process.Pid)
		}
	}
	if tree.WaitExit(0) {
		t.Fatal("WaitExit(0) = true while the captured child is still running")
	}
}

// This is the bug in miniature: killing a process and reaping it says nothing
// about what it spawned. WaitExit is what turns "the process I started is gone"
// into "everything it started is gone", and on Windows that is the difference
// between a directory that can be deleted and one that cannot.
func TestWaitExitOutlastsALeaderThatIsAlreadyReaped(t *testing.T) {
	cmd := startProbeTree(t)

	tree := CaptureProcessTree(cmd.Process.Pid)
	defer tree.Release()

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill the leader: %v", err)
	}
	if _, err := cmd.Process.Wait(); err != nil {
		t.Fatalf("reap the leader: %v", err)
	}

	if tree.WaitExit(200 * time.Millisecond) {
		t.Fatal("WaitExit() = true, but the leader's child is sleeping for two minutes")
	}

	tree.TerminateSurvivors()

	if !tree.WaitExit(10 * time.Second) {
		t.Fatal("WaitExit() = false after TerminateSurvivors(), so something below the leader is still running")
	}
}

// A pid that cannot be running is not an error, the same way terminating one is
// not: the caller has a process it may or may not have managed to start.
func TestCaptureProcessTreeIsQuietAboutAnUnknownPID(t *testing.T) {
	for _, pid := range []int{0, -1, -4242} {
		tree := CaptureProcessTree(pid)
		if len(tree.procs) != 0 {
			t.Errorf("CaptureProcessTree(%d) captured %d processes, want none", pid, len(tree.procs))
		}
		if !tree.WaitExit(time.Second) {
			t.Errorf("CaptureProcessTree(%d).WaitExit() = false, want true", pid)
		}
		tree.Release()
	}
}
