//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// The start-up half of the Android support: what the android init reads
// before main runs. It is compiled wherever its tests run, every host but
// Windows, which has no Android build to start.

// isSystemLinker reports whether path is Android's dynamic linker, which is
// what /proc/self/exe names for a program the linker was asked to run.
func isSystemLinker(path string) bool {
	switch filepath.Base(path) {
	case "linker64", "linker":
	default:
		return false
	}
	return strings.HasPrefix(path, "/system/") || strings.HasPrefix(path, "/apex/")
}

// linkerSelf recognises a process the system linker started as a program and
// returns the path of that program: exe is what /proc/self/exe names, the
// linker then, and args are the arguments Bionic passed to main, which start
// with the path the linker was given - termux-exec gives it the absolute one.
// A relative path is taken from cwd.
func linkerSelf(exe string, args []string, cwd string) (string, bool) {
	if !isSystemLinker(exe) || len(args) == 0 || args[0] == "" {
		return "", false
	}
	self := args[0]
	if !filepath.IsAbs(self) {
		self = filepath.Join(cwd, self)
	}
	return filepath.Clean(self), true
}

// useTermuxFiles points what the Go runtime looks for at Linux paths to
// Termux's copies, as Termux patches its own Go: the CA bundle of the
// ca-certificates package and the prefix's tmp directory (Go's Android default
// is /data/local/tmp, which an app cannot write). A value the user exported
// stays.
func useTermuxFiles(prefix string, getenv func(string) string, setenv func(string, string) error) {
	if getenv("SSL_CERT_FILE") == "" {
		if bundle := filepath.Join(prefix, "etc", "tls", "cert.pem"); isRegularFile(bundle) {
			_ = setenv("SSL_CERT_FILE", bundle)
		}
	}
	if getenv("TMPDIR") == "" {
		if tmp := filepath.Join(prefix, "tmp"); isDir(tmp) {
			_ = setenv("TMPDIR", tmp)
		}
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
