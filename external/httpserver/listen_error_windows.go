//go:build http && windows

package httpserver

import (
	"errors"
	"syscall"
)

// wsaeAddrInUse is Winsock's "address already in use". The standard syscall package does not
// export the WSA* names, and the POSIX syscall.EADDRINUSE it does define on Windows carries a
// different value that Errno.Is will not bridge - so the number is spelled out here rather
// than pulling golang.org/x/sys/windows in for one constant.
const wsaeAddrInUse = syscall.Errno(10048)

// listenErrorIsAddrInUse reports whether a bind failed because the address was taken.
func listenErrorIsAddrInUse(err error) bool {
	return errors.Is(err, wsaeAddrInUse) || errors.Is(err, syscall.EADDRINUSE)
}
