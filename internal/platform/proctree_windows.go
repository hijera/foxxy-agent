//go:build windows

package platform

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcessTree pins the processes running below another one so they can still be
// waited on, and killed if they outstay it, after the process that started them
// is gone.
//
// Killing a process does not kill what it spawned. On unix that rarely matters
// for anything but tidiness, because a file keeps working for whoever still has
// it open after its name is gone; on Windows a surviving child holding a mapping
// makes the file undeletable, and the directory holding it undeletable with it.
// So "the browser is closed" is only true once the whole tree is gone, and this
// is what lets a caller wait for that.
//
// The handles are the point rather than an implementation detail: Windows keeps
// a pid allocated for as long as any handle to its process object is open, so
// holding one is what stops the number being handed to a stranger between the
// capture and the wait - or, worse, between the wait and TerminateSurvivors.
type ProcessTree struct {
	procs []treeProcess
}

type treeProcess struct {
	pid    int
	handle windows.Handle
}

// CaptureProcessTree records the processes descended from pid, so that they can
// be waited on once pid itself has exited.
//
// Call it while pid is still running. Children spawned after the capture are not
// in it, and a leader that has already exited can no longer prove which of the
// processes claiming it as a parent are really its own - Windows keeps the
// recorded parent pid on a child forever, including after that number has been
// handed to somebody else.
//
// Whatever is captured must be given back with Release.
func CaptureProcessTree(pid int) *ProcessTree {
	tree := &ProcessTree{}
	if pid <= 0 {
		return tree
	}

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return tree
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	type parented struct{ pid, parent uint32 }
	var all []parented
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		all = append(all, parented{pid: entry.ProcessID, parent: entry.ParentProcessID})
	}

	// The snapshot is a flat list, so descend one generation per pass until a
	// pass adds nobody. Chrome is two deep (browser, then renderers and the
	// crashpad handler), and a tree that deep costs a handful of passes over a
	// few hundred entries.
	below := map[uint32]bool{uint32(pid): true}
	for grew := true; grew; {
		grew = false
		for _, p := range all {
			if below[p.pid] || !below[p.parent] {
				continue
			}
			below[p.pid] = true
			grew = true
		}
	}
	delete(below, uint32(pid))

	for child := range below {
		// The narrowest rights that answer "has it exited yet"; termination is
		// asked for separately and only if it comes to that, so a process that
		// would refuse PROCESS_TERMINATE is still one this can wait for.
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, child)
		if err != nil {
			// Already gone, or not ours to look at. Either way there is nothing
			// here to wait for.
			continue
		}
		tree.procs = append(tree.procs, treeProcess{pid: int(child), handle: handle})
	}
	return tree
}

// WaitExit reports whether every captured process has exited, waiting at most
// grace in total for the ones that have not.
//
// A wait that cannot answer at all counts as "still running", because the caller
// uses the answer to decide whether anything needs killing and an unproven
// process is not one to declare gone.
func (t *ProcessTree) WaitExit(grace time.Duration) bool {
	if t == nil {
		return true
	}
	deadline := time.Now().Add(grace)
	for _, p := range t.procs {
		// Once the deadline has passed this degrades to a zero-timeout poll,
		// which is what makes grace a budget for the whole tree rather than for
		// each process in it.
		if exited, _ := waitProcess(p.handle, time.Until(deadline)); !exited {
			return false
		}
	}
	return true
}

// TerminateSurvivors kills whatever the capture pinned and is still running. It
// is the end of the grace WaitExit bounds, not a substitute for it: Chrome's
// children exit on their own once the browser process is gone, and killing one
// mid-write is a worse way to end a session than waiting the extra moment.
func (t *ProcessTree) TerminateSurvivors() {
	if t == nil {
		return
	}
	for _, p := range t.procs {
		if exited, _ := waitProcess(p.handle, 0); exited {
			continue
		}
		_ = terminateProcess(p.pid, p.handle)
	}
}

// Release gives back the process objects the capture pinned. Until it is called
// the pids cannot be reused, so a tree that is never released holds a handle -
// and a pid - for the lifetime of the process that captured it.
func (t *ProcessTree) Release() {
	if t == nil {
		return
	}
	for _, p := range t.procs {
		_ = windows.CloseHandle(p.handle)
	}
	t.procs = nil
}
