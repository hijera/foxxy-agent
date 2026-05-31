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
)

// BuildHTTPClient returns an *http.Client configured to route traffic through proxyURL.
// An empty proxyURL returns http.DefaultClient unchanged.
func BuildHTTPClient(proxyURL string) (*http.Client, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return http.DefaultClient, nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
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
