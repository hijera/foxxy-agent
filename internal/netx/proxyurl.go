package netx

import (
	"fmt"
	"net/url"
	"strings"
)

// proxyEncodingHint is what a proxy URL that does not parse as intended most
// often needs: a password with / ? # % in it cuts the URL short, and a space or a
// non-Latin letter is not allowed in it at all. A raw @ and a second colon are
// fine, but encoding them does no harm.
const proxyEncodingHint = "special characters in the proxy login or password (/ ? # %, spaces, non-Latin letters) must be percent-encoded (/ as %2F, # as %23, @ as %40); the … button next to the proxy field in Settings builds the URL for you"

// ParseProxyURL parses a proxy URL for any outbound leg: http, https, socks5 or
// socks5h, with a host.
//
// Beyond url.Parse it refuses the URLs that parse into something other than what
// was typed. A password containing / ? or # ends the authority early, and when
// digits precede the cut ("user:3128/pw@proxy") the result is a valid URL for host
// "user:3128" with no credentials at all, which would send the traffic somewhere
// else in silence. A proxy URL has no path, query or fragment, so any of them
// means the userinfo was cut.
//
// The errors never contain the password: they reach the settings dialog, the log
// and the dry run report, and url.Error would quote the whole URL.
func ParseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	scheme, _, found := strings.Cut(raw, "://")
	if !found || !knownProxyScheme(scheme) {
		if !found {
			scheme = ""
		}
		return nil, fmt.Errorf("proxy %s: unsupported proxy scheme %q (use http, https, socks5, or socks5h)", RedactProxyURL(raw), scheme)
	}
	u, err := url.Parse(raw)
	if err != nil {
		// The parser's own reason quotes fragments of the password ("invalid port
		// \":pa\""), so only the hint goes out.
		return nil, fmt.Errorf("proxy %s cannot be parsed: %s", RedactProxyURL(raw), proxyEncodingHint)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, fmt.Errorf("proxy %s is cut short: %s", RedactProxyURL(raw), proxyEncodingHint)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("proxy %s has no host", RedactProxyURL(raw))
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return u, nil
}

func knownProxyScheme(s string) bool {
	switch strings.ToLower(s) {
	case "http", "https", "socks5", "socks5h":
		return true
	}
	return false
}

// RedactProxyURL replaces a proxy URL's credentials with "redacted", whether or
// not the URL parses. The credentials are taken to run up to the last @, the
// same split url.Parse makes, so a password with @ / ? # in it is hidden whole.
// A user name alone is redacted too: it is often the token.
func RedactProxyURL(raw string) string {
	prefix, rest := "", raw
	if i := strings.Index(raw, "://"); i >= 0 {
		prefix, rest = raw[:i+3], raw[i+3:]
	}
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return raw
	}
	return prefix + "redacted" + rest[at:]
}
