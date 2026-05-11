//go:build scheduler

package daemon

import (
	"testing"
	"time"
)

func TestSpawnDedupeSkipsSecondLaunchSameSlot(t *testing.T) {
	p := "/tmp/demo.md"
	slot, err := time.Parse(time.RFC3339, "2026-05-12T00:02:00Z")
	if err != nil {
		t.Fatal(err)
	}
	var zero time.Time
	if shouldSkipDuplicateCronSpawn(p, slot, zero) {
		t.Fatal("first check should not skip")
	}
	noteSpawnDispatched(p, slot)
	if !shouldSkipDuplicateCronSpawn(p, slot, zero) {
		t.Fatal("second launch same slot should skip while disk last still empty")
	}
}

func TestSpawnDedupeClearsWhenDiskCaughtUp(t *testing.T) {
	p := "/tmp/other.md"
	slot, err := time.Parse(time.RFC3339, "2026-05-12T00:05:00Z")
	if err != nil {
		t.Fatal(err)
	}
	var zero time.Time
	noteSpawnDispatched(p, slot)
	last := slot
	if shouldSkipDuplicateCronSpawn(p, slot, last) {
		t.Fatal("when last on disk is at due slot, do not skip (dedupe cleared)")
	}
	if shouldSkipDuplicateCronSpawn(p, slot, zero) {
		t.Fatal("after disk catch-up path, mem entry should be gone")
	}
}
