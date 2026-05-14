//go:build unix

package session

import (
	"testing"
)

func TestAcquireTurnLockFlockExclusive(t *testing.T) {
	dir := t.TempDir()
	u1, err := acquireTurnLockFlock(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = acquireTurnLockFlock(dir)
	if err != ErrSessionTurnBusy {
		t.Fatalf("want busy got %v", err)
	}
	u1()
	u2, err := acquireTurnLockFlock(dir)
	if err != nil {
		t.Fatal(err)
	}
	u2()
}

func TestTurnLockHeldWhileExclusiveHeld(t *testing.T) {
	dir := t.TempDir()
	u, err := acquireTurnLockFlock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !TurnLockHeld(dir) {
		t.Fatal("expected TurnLockHeld true")
	}
	u()
	if TurnLockHeld(dir) {
		t.Fatal("expected TurnLockHeld false after release")
	}
}
