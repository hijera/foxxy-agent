//go:build android

package platform

import "os"

// init runs before anything opens a TLS connection or a temporary file, so the
// Termux files replace the Linux paths first. A process the system linker
// started learns the path of its binary here, for Executable and AdaptCommand.
func init() {
	exe, _ := os.Readlink("/proc/self/exe")
	cwd, _ := os.Getwd()
	if self, ok := linkerSelf(exe, os.Args, cwd); ok {
		androidSelf = self
		androidLinkerExec = true
	}
	useTermuxFiles(termuxPrefix(os.Getenv), os.Getenv, os.Setenv)
}
