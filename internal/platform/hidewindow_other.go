//go:build !windows

package platform

import "os/exec"

// HideConsoleWindow does nothing outside Windows. A child process elsewhere is
// given no window of its own to suppress: it inherits the standard streams it is
// handed and draws nothing.
func HideConsoleWindow(_ *exec.Cmd) {}
