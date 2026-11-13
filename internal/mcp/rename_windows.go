//go:build windows

package mcp

import (
	"errors"
	"syscall"
)

// Windows rejects renaming over a file another handle holds open, surfacing
// ERROR_ACCESS_DENIED (5) or ERROR_SHARING_VIOLATION (32). Under concurrent
// access these are transient and worth a brief retry: the state file's readers
// take neither lock and open through os.ReadFile, which shares read and write
// but not delete.
const (
	errAccessDenied     = syscall.Errno(5)
	errSharingViolation = syscall.Errno(32)
)

func isRetryableFileLockError(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == errAccessDenied || errno == errSharingViolation
	}
	return false
}
