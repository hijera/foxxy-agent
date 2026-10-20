package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/netx"
)

const (
	ProxyInherit = "inherit"
	ProxyNone    = "none"
)

type ProxyMode int

const (
	ProxyModeInherit ProxyMode = iota
	ProxyModeNone
	ProxyModeURL
)

// ParseProxySetting accepts inherit (or empty), none, or a proxy URL. URL
// validation uses the shared netx parser, including its password-safe errors.
func ParseProxySetting(setting string) (ProxyMode, *url.URL, error) {
	setting = strings.TrimSpace(setting)
	switch {
	case setting == "" || strings.EqualFold(setting, ProxyInherit):
		return ProxyModeInherit, nil, nil
	case strings.EqualFold(setting, ProxyNone):
		return ProxyModeNone, nil, nil
	}
	if !strings.ContainsAny(setting, "/:") {
		return 0, nil, fmt.Errorf("proxy: unknown value; use %q to connect directly, %q to inherit proxy settings, or a proxy URL", ProxyNone, ProxyInherit)
	}
	u, err := netx.ParseProxyURL(setting)
	if err != nil {
		return 0, nil, err
	}
	return ProxyModeURL, u, nil
}

func normalizeProxySetting(setting string) string {
	setting = strings.TrimSpace(setting)
	if strings.EqualFold(setting, ProxyInherit) {
		return ProxyInherit
	}
	if strings.EqualFold(setting, ProxyNone) {
		return ProxyNone
	}
	return setting
}

func validateProxySetting(setting string) error {
	_, _, err := ParseProxySetting(setting)
	return err
}

// Kept for callers that still name the URL-only validator.
func validateProviderProxyURL(setting string) error { return validateProxySetting(setting) }
