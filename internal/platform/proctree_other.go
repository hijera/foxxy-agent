//go:build !windows

package platform

import "time"

// ProcessTree pins the processes running below another one so they can still be
// waited on after the process that started them is gone. Outside Windows there
// is nothing to pin and nothing to wait for, and the type exists so that callers
// do not have to know that.
//
// The reason is unlink, not process lifetimes: a child here can go on holding a
// file open for as long as it likes without keeping the name in the directory,
// so removing the directory its parent was using never has to wait for it.
// Windows refuses that removal instead, which is what gives the wait a job to
// do - see the file next to this one.
//
// Reaching for a process group to wait on here would also mean taking over the
// child's own process-group arrangements at spawn time, which is a real change
// (a group of its own stops a Ctrl-C in the operator's terminal reaching it) in
// exchange for solving a problem this platform does not have.
type ProcessTree struct{}

// CaptureProcessTree records nothing outside Windows. See ProcessTree.
func CaptureProcessTree(int) *ProcessTree { return &ProcessTree{} }

// WaitExit has nothing to wait for outside Windows and says so immediately.
func (t *ProcessTree) WaitExit(time.Duration) bool { return true }

// TerminateSurvivors has nothing pinned to kill outside Windows.
func (t *ProcessTree) TerminateSurvivors() {}

// Release has nothing pinned to give back outside Windows.
func (t *ProcessTree) Release() {}
