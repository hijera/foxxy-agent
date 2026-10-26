package config

import (
	"fmt"
	"net"
	"net/textproto"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// ToolHTTPRequest is the YAML tools.http_request section: the policy of the
// http_request tool and the headers every request it sends carries.
type ToolHTTPRequest struct {
	// Allowlist names the destinations a request may reach without a
	// permission prompt: a host ("api.github.com"), a subdomain wildcard
	// ("*.example.com"), either with an optional port, an origin
	// ("http://localhost:8080") or an address prefix
	// ("https://api.example.com/v1/"). "*" allows every destination. An entry
	// covers everything a request to that destination carries - its files, an
	// unchecked certificate - but not the file it would write, which follows
	// the write policy, nor a proxy, which is a destination of its own.
	Allowlist []string `yaml:"allowlist"`

	// DefaultHeaders are request headers every http_request call sends unless
	// the call names the header itself. They take the place of the tool's own
	// defaults (its foxxycode-agent User-Agent), and a call's headers take the
	// place of them, an empty value there removing one; an empty value here
	// leaves the header out of every call that does not set it. They go to
	// every destination the tool reaches and nowhere else: webfetch, URL
	// mentions and the model providers never send them.
	DefaultHeaders map[string]string `yaml:"default_headers"`
}

// validate trims the allowlist entries in place and refuses one that cannot
// match, and refuses a default header the tool could not send.
func (h *ToolHTTPRequest) validate() error {
	for i := range h.Allowlist {
		h.Allowlist[i] = strings.TrimSpace(h.Allowlist[i])
		if _, err := parseHTTPAllowRule(h.Allowlist[i]); err != nil {
			return fmt.Errorf("tools.http_request.allowlist[%d]: %w", i, err)
		}
	}
	return validateHTTPDefaultHeaders(h.DefaultHeaders)
}

// httpHeadersOfOneRequest are the headers http_request derives from each call:
// the host of its address, the type of its payload and the payload's framing.
// A value for every request cannot stand for them, so default_headers refuses
// them; a call that needs another value sets it in its own headers.
var httpHeadersOfOneRequest = map[string]bool{
	"Host":              true,
	"Content-Type":      true,
	"Content-Length":    true,
	"Transfer-Encoding": true,
}

// httpHeadersOfOneConnection are the hop-by-hop headers: they describe the
// connection a request travels on, not the client, and one set for every
// request can break them all - HTTP/2 refuses a request carrying Upgrade.
var httpHeadersOfOneConnection = map[string]bool{
	"Connection":       true,
	"Keep-Alive":       true,
	"Proxy-Connection": true,
	"Te":               true,
	"Trailer":          true,
	"Upgrade":          true,
}

// httpProxyCredentialHeader is refused as a default header for another reason:
// through a proxy an https request tunnels, and a header the request carries
// reaches the origin inside the tunnel, so a proxy's credential set here would
// go to every destination rather than to the proxy.
const httpProxyCredentialHeader = "Proxy-Authorization"

// httpDefaultHeaderName is the shape of a name default_headers takes: letters,
// digits, "-" and "_", starting with a letter - the shape of every header in
// use. HTTP allows more (dots, digits first), and a call may send such a name in
// its own headers, but a key of this map is also a segment of a config path
// (config_set, foxxycode -t), where a dot splits it and a number reads as a list
// position.
var httpDefaultHeaderName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// validateHTTPDefaultHeaders refuses a name that is not a header name of that
// shape, two spellings of one header, a header the tool derives from each
// request and a value that would break the header line. The map is left as
// written: the tool trims names and values when it sends them.
func validateHTTPDefaultHeaders(headers map[string]string) error {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	seen := make(map[string]string, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if !httpDefaultHeaderName.MatchString(name) {
			return fmt.Errorf("tools.http_request.default_headers: %q is not a header name this map takes: letters, digits, - and _, starting with a letter (a call can still send any other name in its own headers)", raw)
		}
		key := textproto.CanonicalMIMEHeaderKey(name)
		if first, dup := seen[key]; dup {
			return fmt.Errorf("tools.http_request.default_headers: %q and %q name the same header; keep one", first, raw)
		}
		seen[key] = raw
		if httpHeadersOfOneRequest[key] {
			return fmt.Errorf("tools.http_request.default_headers.%s: %s describes a single request - http_request derives it from each call, and a call that needs another value sets it in its own headers", name, key)
		}
		if httpHeadersOfOneConnection[key] {
			return fmt.Errorf("tools.http_request.default_headers.%s: %s belongs to one connection, not to every request (HTTP/2 refuses a request that carries Upgrade, for one); a call that needs it sets it in its own headers", name, key)
		}
		if key == httpProxyCredentialHeader {
			return fmt.Errorf("tools.http_request.default_headers.%s: an https request carries its headers to the origin through the proxy's tunnel, so this would reach every destination; put the credential into the proxy address instead (HTTPS_PROXY, or a call's proxy as http://user:password@host:port)", name)
		}
		if !httpguts.ValidHeaderFieldValue(strings.TrimSpace(headers[raw])) {
			return fmt.Errorf("tools.http_request.default_headers.%s: the value may not contain line breaks or control characters", name)
		}
	}
	return nil
}

// HTTPAllowlistAllows reports whether any entry of a tools.http_request
// allowlist covers u. An entry that does not parse covers nothing.
func HTTPAllowlistAllows(entries []string, u *url.URL) bool {
	if u == nil {
		return false
	}
	for _, entry := range entries {
		rule, err := parseHTTPAllowRule(strings.TrimSpace(entry))
		if err == nil && rule.matches(u) {
			return true
		}
	}
	return false
}

// httpAllowRule is one parsed allowlist entry.
type httpAllowRule struct {
	any bool
	// scheme is empty when the entry names none and so covers both.
	scheme string
	// host is lower-cased; with wildcard it is the domain the subdomains sit under.
	host     string
	wildcard bool
	// port is empty when any port matches.
	port string
	// path is empty when any path matches.
	path string
}

func parseHTTPAllowRule(entry string) (httpAllowRule, error) {
	if entry == "" {
		return httpAllowRule{}, fmt.Errorf("empty entry")
	}
	if entry == "*" {
		return httpAllowRule{any: true}, nil
	}
	var rule httpAllowRule
	var host string
	if strings.Contains(entry, "://") {
		u, err := url.Parse(entry)
		if err != nil {
			return rule, err
		}
		rule.scheme = strings.ToLower(u.Scheme)
		if rule.scheme != "http" && rule.scheme != "https" {
			return rule, fmt.Errorf("%q: the scheme must be http or https", entry)
		}
		if u.User != nil {
			return rule, fmt.Errorf("%q: credentials do not belong in an allowlist entry", entry)
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return rule, fmt.Errorf("%q: a query or a fragment cannot be matched", entry)
		}
		host = u.Hostname()
		rule.port = u.Port()
		if rule.port == "" {
			rule.port = map[string]string{"http": "80", "https": "443"}[rule.scheme]
		}
		if p := u.EscapedPath(); p != "" && p != "/" {
			rule.path = p
		}
	} else {
		if strings.Contains(entry, "/") {
			return rule, fmt.Errorf("%q: an entry with a path needs a scheme, like https://%s", entry, entry)
		}
		host = entry
		if h, p, err := net.SplitHostPort(entry); err == nil {
			host, rule.port = h, p
		} else if strings.HasPrefix(entry, "[") && strings.HasSuffix(entry, "]") {
			host = strings.Trim(entry, "[]")
		}
	}
	if rule.port != "" {
		if n, err := strconv.Atoi(rule.port); err != nil || n < 1 || n > 65535 {
			return rule, fmt.Errorf("%q: port %q is not a port number", entry, rule.port)
		}
	}
	host = strings.ToLower(host)
	if strings.HasPrefix(host, "*.") {
		rule.wildcard = true
		host = strings.TrimPrefix(host, "*.")
	}
	if host == "" || strings.Contains(host, "*") {
		return rule, fmt.Errorf("%q: a wildcard may only stand for the leftmost labels, as in *.example.com", entry)
	}
	rule.host = host
	return rule, nil
}

func (r httpAllowRule) matches(u *url.URL) bool {
	if r.any {
		return true
	}
	scheme := strings.ToLower(u.Scheme)
	if r.scheme != "" && r.scheme != scheme {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if r.wildcard {
		if !strings.HasSuffix(host, "."+r.host) {
			return false
		}
	} else if host != r.host {
		return false
	}
	if r.port != "" {
		port := u.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443"}[scheme]
		}
		if port != r.port {
			return false
		}
	}
	if r.path != "" {
		path := u.EscapedPath()
		if path == "" {
			path = "/"
		}
		if strings.HasSuffix(r.path, "/") {
			return strings.HasPrefix(path, r.path)
		}
		return path == r.path || strings.HasPrefix(path, r.path+"/")
	}
	return true
}
