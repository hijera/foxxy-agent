package platform

import (
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// SystemProxyEnvVar turns the operating system's proxy settings off for this
// process when set to "off" (or "0", "false", "no"): FoxxyCode then follows only
// HTTP_PROXY / HTTPS_PROXY and the per-provider proxy, as it did before.
const SystemProxyEnvVar = "FOXXYCODE_SYSTEM_PROXY"

// SystemProxyConfig is the user's proxy setting as the operating system holds
// it: on Windows, what Settings -> Network -> Proxy and Internet Options write.
type SystemProxyConfig struct {
	// AutoDetect is "Automatically detect settings" (WPAD).
	AutoDetect bool
	// AutoConfigURL is "Use setup script": the PAC file's address.
	AutoConfigURL string
	// Proxy is the manual proxy: "host:port", or per scheme
	// "http=host:port;https=host:port;socks=host:port".
	Proxy string
	// Bypass is the manual exception list: "<local>;*.corp.example;10.*".
	Bypass string
}

// Empty reports whether the configuration names no proxy at all.
func (c SystemProxyConfig) Empty() bool {
	return !c.AutoDetect && strings.TrimSpace(c.AutoConfigURL) == "" && strings.TrimSpace(c.Proxy) == ""
}

// Seams over the operating system, replaced in tests. readSystemProxyConfig
// returns a zero config where there is nothing to read; pacLookup evaluates the
// PAC script (or WPAD) for one URL and returns the proxy list it chose, "" for
// DIRECT.
var (
	readSystemProxyConfig = readSystemProxyConfigOS
	pacLookup             = pacLookupOS
)

// Route is where a request should go according to the system proxy.
type Route struct {
	// Proxy is nil for a direct connection.
	Proxy *url.URL
	// Via says how that was decided: "pac", "static", "bypass" or "loopback", and
	// for a PAC that could not be evaluated "pac failed: <reason>" (the static
	// proxy, if any, or a direct connection is used then, as a browser would).
	Via string
}

// pacTTL is how long a PAC answer for one host is reused: the script is
// evaluated per URL, and evaluating it for every request would put a script run,
// and possibly a WPAD lookup, in front of each of them.
const (
	pacTTL        = 5 * time.Minute
	pacFailureTTL = 30 * time.Second
)

type pacEntry struct {
	list    string
	err     error
	expires time.Time
}

// SystemProxy resolves requests against the operating system's proxy settings.
type SystemProxy struct {
	cfg    SystemProxyConfig
	bypass []string

	mu    sync.Mutex
	cache map[string]pacEntry
	now   func() time.Time
}

// LoadSystemProxy reads the operating system's proxy settings. It returns nil
// when there are none, on systems it cannot read them on, and when
// FOXXYCODE_SYSTEM_PROXY turns them off.
func LoadSystemProxy() *SystemProxy {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(SystemProxyEnvVar))) {
	case "off", "0", "false", "no":
		return nil
	}
	cfg, err := readSystemProxyConfig()
	if err != nil || cfg.Empty() {
		return nil
	}
	return NewSystemProxy(cfg)
}

// NewSystemProxy resolves against cfg.
func NewSystemProxy(cfg SystemProxyConfig) *SystemProxy {
	return &SystemProxy{
		cfg:    cfg,
		bypass: splitProxyList(cfg.Bypass),
		cache:  map[string]pacEntry{},
		now:    time.Now,
	}
}

// Config is the configuration this resolver follows.
func (s *SystemProxy) Config() SystemProxyConfig { return s.cfg }

// Describe is a one-line summary for the log, with any credentials redacted.
func (s *SystemProxy) Describe() string {
	var parts []string
	if s.cfg.AutoConfigURL != "" {
		parts = append(parts, "pac "+s.cfg.AutoConfigURL)
	}
	if s.cfg.AutoDetect {
		parts = append(parts, "auto-detect (WPAD)")
	}
	if s.cfg.Proxy != "" {
		parts = append(parts, "proxy "+redactProxyList(s.cfg.Proxy))
	}
	if s.cfg.Bypass != "" {
		parts = append(parts, "bypass "+s.cfg.Bypass)
	}
	return strings.Join(parts, ", ")
}

// Resolve decides the route of one request.
func (s *SystemProxy) Resolve(target *url.URL) Route {
	host := strings.ToLower(target.Hostname())
	if isLoopbackHost(host) {
		return Route{Via: "loopback"}
	}
	if s.bypassed(host) {
		return Route{Via: "bypass"}
	}
	via := "static"
	if s.cfg.AutoConfigURL != "" || s.cfg.AutoDetect {
		list, err := s.pac(target)
		if err == nil {
			return Route{Proxy: firstProxy(list, target.Scheme), Via: "pac"}
		}
		via = "pac failed: " + err.Error()
	}
	return Route{Proxy: staticProxy(s.cfg.Proxy, target.Scheme), Via: via}
}

func (s *SystemProxy) pac(target *url.URL) (string, error) {
	key := target.Scheme + "://" + target.Host
	now := s.now()
	s.mu.Lock()
	if e, ok := s.cache[key]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return e.list, e.err
	}
	s.mu.Unlock()

	// The script sees the URL without credentials, path or query: some of it is
	// nobody else's business, and a per-path answer would defeat the cache.
	list, err := pacLookup(target.Scheme+"://"+target.Host+"/", s.cfg)
	ttl := pacTTL
	if err != nil {
		ttl = pacFailureTTL
	}
	s.mu.Lock()
	s.cache[key] = pacEntry{list: list, err: err, expires: now.Add(ttl)}
	s.mu.Unlock()
	return list, err
}

func (s *SystemProxy) bypassed(host string) bool {
	for _, p := range s.bypass {
		p = strings.ToLower(p)
		if i := strings.Index(p, "://"); i >= 0 {
			p = p[i+3:]
		}
		if p == "<local>" {
			// A plain intranet name, as Windows means it.
			if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
				return true
			}
			continue
		}
		if h, _, err := net.SplitHostPort(p); err == nil {
			p = h
		}
		if wildcardMatch(p, host) {
			return true
		}
	}
	return false
}

// wildcardMatch matches host against a pattern where * stands for any run of
// characters, the only wildcard the Windows exception list has.
func wildcardMatch(pattern, host string) bool {
	if pattern == "" {
		return false
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == host
	}
	if !strings.HasPrefix(host, parts[0]) {
		return false
	}
	rest := host[len(parts[0]):]
	for i, part := range parts[1:] {
		last := i == len(parts)-2
		if last {
			return strings.HasSuffix(rest, part)
		}
		j := strings.Index(rest, part)
		if j < 0 {
			return false
		}
		rest = rest[j+len(part):]
	}
	return true
}

func isLoopbackHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func splitProxyList(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' || r == '\t' || r == '\r' || r == '\n' }) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// staticProxy picks the manual proxy for a scheme. A bare "host:port" serves
// every scheme; the per-scheme form serves the schemes it names, then a socks=
// entry serves the rest, and a scheme with neither goes direct - the way Windows
// itself reads the setting.
func staticProxy(list, scheme string) *url.URL {
	var socks string
	for _, e := range splitProxyList(list) {
		name, value, keyed := strings.Cut(e, "=")
		if !keyed {
			return proxyURL(e, "http")
		}
		switch strings.ToLower(name) {
		case strings.ToLower(scheme):
			return proxyURL(value, "http")
		case "socks":
			socks = value
		}
	}
	if socks != "" {
		return proxyURL(socks, "socks5")
	}
	return nil
}

// firstProxy takes the first proxy of a PAC answer. WinHTTP hands back the
// "PROXY a; PROXY b" list as "a;b" (DIRECT entries dropped), sometimes in the
// per-scheme form of the manual setting; the keyword form is read too.
func firstProxy(list, scheme string) *url.URL {
	var first string
	for _, e := range strings.FieldsFunc(list, func(r rune) bool { return r == ';' || r == ',' }) {
		if e = strings.TrimSpace(e); e != "" {
			first = e
			break
		}
	}
	if first == "" || strings.EqualFold(first, "DIRECT") {
		return nil
	}
	if strings.Contains(first, "=") {
		return staticProxy(list, scheme)
	}
	keyword, rest, spaced := strings.Cut(first, " ")
	if !spaced {
		return proxyURL(first, "http")
	}
	switch strings.ToUpper(keyword) {
	case "PROXY", "HTTP":
		return proxyURL(rest, "http")
	case "HTTPS":
		return proxyURL(rest, "https")
	case "SOCKS", "SOCKS5":
		return proxyURL(rest, "socks5")
	}
	return nil
}

// proxyURL turns "host:port" (or a value that already names its scheme) into a
// proxy URL.
func proxyURL(v, scheme string) *url.URL {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if !strings.Contains(v, "://") {
		v = scheme + "://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

// redactProxyList hides credentials a manual proxy may carry ("user:pw@host").
func redactProxyList(s string) string {
	entries := splitProxyList(s)
	for i, e := range entries {
		if at := strings.LastIndex(e, "@"); at >= 0 {
			prefix := ""
			if eq := strings.Index(e, "="); eq >= 0 && eq < at {
				prefix = e[:eq+1]
			}
			if sch := strings.Index(e, "://"); sch >= 0 && sch < at {
				prefix = e[:sch+3]
			}
			entries[i] = prefix + "redacted" + e[at:]
		}
	}
	return strings.Join(entries, ";")
}
