package platform

import (
	"testing"
	"time"
)

// Every platform has to answer the whole ProcessTree contract, whether or not it
// has anything to pin: the caller is teardown code that must not have to know
// which one it is running on.
func TestProcessTreeIsSafeWhenNothingWasCaptured(t *testing.T) {
	tree := CaptureProcessTree(0)
	if tree == nil {
		t.Fatal("CaptureProcessTree(0) = nil, want an empty tree")
	}
	if !tree.WaitExit(time.Second) {
		t.Error("WaitExit() = false for an empty tree, want true")
	}
	tree.TerminateSurvivors()
	tree.Release()
	// Release is idempotent so that a deferred one cannot turn a second call into
	// a double close of somebody else's handle.
	tree.Release()
}
