package config

import (
	"strings"

	"github.com/hijera/foxxycode-agent/internal/netx"
)

// validateProviderProxyURL checks optional per-provider proxy URL (http, https, socks5, socks5h).
// It shares netx.ParseProxyURL with every outbound leg, so a URL whose password
// cuts it short is refused here, at save time, and the error never quotes the
// password.
func validateProviderProxyURL(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	_, err := netx.ParseProxyURL(s)
	return err
}
