//go:build http && !windows

package httpserver

import (
	"errors"
	"syscall"
)

// listenErrorIsAddrInUse reports whether a bind failed because the address was taken.
func listenErrorIsAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
