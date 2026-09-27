package netx

import (
	"os"
	"testing"
)

// The tests name their own proxies. The machine's system proxy (a corporate PAC
// on a developer's laptop) must not route their made-up hosts.
func TestMain(m *testing.M) {
	restore := SetSystemProxyForTesting(nil)
	code := m.Run()
	restore()
	os.Exit(code)
}
