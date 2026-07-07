//go:build desktop && windows

package desktop

import (
	"net"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestFreeLoopbackPort(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}
	addr, err := FreeLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("host %q want 127.0.0.1", host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		t.Fatalf("port %q invalid", portStr)
	}
	addr2, err := FreeLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	if addr == addr2 {
		t.Logf("same port assigned twice (possible on fast reuse): %s", addr)
	}
	if !strings.HasPrefix(addr2, "127.0.0.1:") {
		t.Fatalf("addr2 %q", addr2)
	}
}
