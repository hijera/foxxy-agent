package session_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// TestHelperTurnLockHolder is not a real test: with GO_WANT_TURN_LOCK_HOLDER=1 it stands in
// for a second FoxxyCode process (a second IDE window) that is running a turn for the session
// bundle at FOXXYCODE_TURN_LOCK_DIR. It reports on stdout once the lock is taken, then holds
// it until its stdin closes.
func TestHelperTurnLockHolder(t *testing.T) {
	if os.Getenv("GO_WANT_TURN_LOCK_HOLDER") != "1" {
		t.Skip("helper process")
	}
	dir := os.Getenv("FOXXYCODE_TURN_LOCK_DIR")
	st := session.NewStateForTurnLockTest(dir)
	unlock, err := session.AcquireTurnLockForTest(st)
	if err != nil {
		_, _ = os.Stdout.WriteString("ERR " + err.Error() + "\n")
		os.Exit(1)
	}
	defer unlock()
	_, _ = os.Stdout.WriteString("LOCKED\n")
	// Held for a fixed time when asked, otherwise until stdin closes. The timer exists for
	// runs driven by hand (`go test -run TestHelperTurnLockHolder`): `go test` does not hand
	// its stdin to the test binary, so the read returns EOF at once and the lock is gone
	// before anything can observe it - which is exactly how one earlier measurement ended up
	// concluding the wrong thing.
	if raw := strings.TrimSpace(os.Getenv("FOXXYCODE_TURN_LOCK_HOLD_MS")); raw != "" {
		if ms, convErr := strconv.Atoi(raw); convErr == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
			return
		}
	}
	_, _ = os.Stdin.Read(make([]byte, 1))
}

// Two IDE windows are two backend processes over one shared home. Without a cross-process
// lock both accept a turn for the same session and both persist it, so the one that finishes
// last silently overwrites the other's transcript - the whole turn, prompt included, is lost.
// On Windows the lock used to be a per-process mutex, which cannot see the other process at
// all.
func TestTurnLockIsHeldAcrossProcesses(t *testing.T) {
	dir := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperTurnLockHolder")
	cmd.Env = append(os.Environ(),
		"GO_WANT_TURN_LOCK_HOLDER=1",
		"FOXXYCODE_TURN_LOCK_DIR="+dir,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})

	// Wait for the other process to actually own the lock.
	buf := make([]byte, 64)
	deadline := time.Now().Add(30 * time.Second)
	got := ""
	for !strings.Contains(got, "LOCKED") && time.Now().Before(deadline) {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			got += string(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
	if !strings.Contains(got, "LOCKED") {
		t.Fatalf("the holder process never took the lock, got %q", got)
	}

	if !session.TurnLockHeld(dir) {
		t.Fatal("TurnLockHeld says the bundle is free while another process holds it")
	}

	st := session.NewStateForTurnLockTest(dir)
	unlock, err := session.AcquireTurnLockForTest(st)
	if err == nil {
		unlock()
		t.Fatal("a second process was allowed to start a turn on the same session")
	}
	if err != session.ErrSessionTurnBusy {
		t.Fatalf("expected ErrSessionTurnBusy, got %v", err)
	}

	// Releasing it hands the session back.
	_ = stdin.Close()
	_ = cmd.Wait()
	waitUntil(t, 10*time.Second, func() bool { return !session.TurnLockHeld(dir) })
	unlock, err = session.AcquireTurnLockForTest(st)
	if err != nil {
		t.Fatalf("the lock was not released when the other process exited: %v", err)
	}
	unlock()
}

// A lock whose owner died is not a wedged session: the OS drops it with the process.
func TestTurnLockSurvivesAHolderThatDies(t *testing.T) {
	dir := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperTurnLockHolder")
	cmd.Env = append(os.Environ(),
		"GO_WANT_TURN_LOCK_HOLDER=1",
		"FOXXYCODE_TURN_LOCK_DIR="+dir,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	got := ""
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(got, "LOCKED") && time.Now().Before(deadline) {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			got += string(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
	if !strings.Contains(got, "LOCKED") {
		t.Fatalf("the holder process never took the lock, got %q", got)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	waitUntil(t, 15*time.Second, func() bool { return !session.TurnLockHeld(dir) })
	st := session.NewStateForTurnLockTest(dir)
	unlock, err := session.AcquireTurnLockForTest(st)
	if err != nil {
		t.Fatalf("a killed holder left the session wedged: %v", err)
	}
	unlock()

	// The lock file itself may stay behind; only the lock matters.
	if _, statErr := os.Stat(filepath.Join(dir, ".foxxycode-turn.lock")); statErr != nil && !os.IsNotExist(statErr) {
		t.Fatalf("unexpected lock file state: %v", statErr)
	}
}

func waitUntil(t *testing.T, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition never became true")
}
