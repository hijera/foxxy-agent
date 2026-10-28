package web

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrDisallowedURL is returned when a URL must not be fetched (SSRF guard).
var ErrDisallowedURL = fmt.Errorf("url is not allowed for fetch")

// ValidateFetchURL checks scheme, host, and that DNS resolution yields only public IPs.
func ValidateFetchURL(ctx context.Context, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q", ErrDisallowedURL, u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: userinfo in url", ErrDisallowedURL)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("%w: empty host", ErrDisallowedURL)
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, fmt.Errorf("%w: localhost", ErrDisallowedURL)
	}
	if strings.HasSuffix(strings.ToLower(host), ".local") {
		return nil, fmt.Errorf("%w: .local host", ErrDisallowedURL)
	}
	if err := verifyHostIPs(ctx, host); err != nil {
		return nil, err
	}
	return u, nil
}

func verifyHostIPs(ctx context.Context, host string) error {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		return checkIPAllowed(ip)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("dns lookup: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: no IPs for host", ErrDisallowedURL)
	}
	for _, ia := range ips {
		if err := checkIPAllowed(ia.IP); err != nil {
			return err
		}
	}
	return nil
}

func checkIPAllowed(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("%w: nil ip", ErrDisallowedURL)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("%w: non-public ip %s", ErrDisallowedURL, ip)
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return fmt.Errorf("%w: metadata range %s", ErrDisallowedURL, ip)
		}
		// RFC 6598 shared address space, 100.64.0.0/10: carrier-grade NAT and
		// tailnets, never the public internet, and Alibaba Cloud answers
		// instance metadata at 100.100.100.200 in it. Go's IsPrivate leaves it
		// out; internal/netx refuses it the same way.
		if ip4[0] == 100 && ip4[1]&0xc0 == 64 {
			return fmt.Errorf("%w: shared address space %s", ErrDisallowedURL, ip)
		}
	}
	return nil
}
