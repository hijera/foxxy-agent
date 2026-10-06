//go:build gateway || gateway.telegram

// Package proxyutil builds HTTP clients with optional proxy support for gateway adapters.
// Supported schemes: http, https, socks5, socks5h.
package proxyutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/netx"
)

// BuildHTTPClient accepts inherit (including empty), none, or a proxy URL.
// Inherit includes the fork's system proxy resolver when no environment proxy
// is set. None bypasses both the environment and the system proxy.
func BuildHTTPClient(setting string) (*http.Client, error) {
	mode, u, err := config.ParseProxySetting(setting)
	if err != nil {
		return nil, err
	}
	switch mode {
	case config.ProxyModeInherit:
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("default transport is not *http.Transport")
		}
		t := base.Clone()
		resolve := netx.EnvironmentProxyResolver()
		t.Proxy = func(req *http.Request) (*url.URL, error) {
			route, err := resolve(req.URL)
			return route.Proxy, err
		}
		return &http.Client{Transport: t}, nil
	case config.ProxyModeNone:
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("default transport is not *http.Transport")
		}
		t := base.Clone()
		t.Proxy = nil
		return &http.Client{Transport: t}, nil
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(u)},
		}, nil
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(u, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 proxy: %w", err)
		}
		return &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (use http, https, socks5, or socks5h)", u.Scheme)
	}
}
