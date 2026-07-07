//go:build desktop && windows

package desktop

import (
	"fmt"
	"net"
)

// FreeLoopbackPort binds 127.0.0.1:0 and returns the assigned host:port address.
func FreeLoopbackPort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen loopback: %w", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", fmt.Errorf("close probe listener: %w", err)
	}
	return addr, nil
}
