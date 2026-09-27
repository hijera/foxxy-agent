//go:build !windows

package platform

import "errors"

// Elsewhere the system proxy reaches FoxxyCode the usual way, through
// HTTP_PROXY / HTTPS_PROXY in the environment the desktop session sets.
func readSystemProxyConfigOS() (SystemProxyConfig, error) { return SystemProxyConfig{}, nil }

func pacLookupOS(string, SystemProxyConfig) (string, error) {
	return "", errors.New("PAC is evaluated only on Windows")
}
