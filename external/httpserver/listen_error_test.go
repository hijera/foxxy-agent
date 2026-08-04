//go:build http

package httpserver

import (
	"errors"
	"net"
	"strings"
	"testing"
)

// A busy port must be reported as such. The OS wording differs per platform, so the check
// has to survive on the errno rather than on the message text.
func TestDescribeListenErrorNamesABusyPort(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer held.Close()
	addr := held.Addr().String()

	second, err := net.Listen("tcp", addr)
	if err == nil {
		second.Close()
		t.Skip("this platform allows two listeners on the same address")
	}

	got := describeListenError(err, addr)
	if !strings.Contains(got.Error(), "already in use") {
		t.Fatalf("expected a port-in-use message, got %q", got.Error())
	}
	if !strings.Contains(got.Error(), addr) {
		t.Fatalf("expected the address %q in %q", addr, got.Error())
	}
	if !errors.Is(got, err) {
		t.Fatalf("expected the original error to stay wrapped")
	}
}

func TestDescribeListenErrorPassesThroughOtherFailures(t *testing.T) {
	if got := describeListenError(nil, "127.0.0.1:1"); got != nil {
		t.Fatalf("expected nil for a successful listen, got %v", got)
	}
	own := errors.New("something else entirely")
	if got := describeListenError(own, "127.0.0.1:1"); got != own {
		t.Fatalf("expected the error to pass through unchanged, got %v", got)
	}
}
