//go:build http

package httpserver

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"time"
)

// authPolicy is the effective bearer-token policy for one request, snapshotting the live
// config plus any out-of-band tokens so PUT /foxxycode/config hot reloads take effect immediately.
type authPolicy struct {
	enabled    bool
	tokens     []string
	publicDocs bool
	// ticketsOnly mirrors httpserver.stream_tickets_only: when set, an SSE route accepts a
	// stream ticket in ?access_token= but no longer the durable bearer token.
	ticketsOnly bool
	// login is the web sign-in form as it stands for this request. A browser
	// that has passed it reaches the same routes a bearer token reaches.
	login loginPolicy
}

// SetExtraAuthTokens registers bearer tokens supplied via --auth-token / FOXXYCODE_HTTP_TOKEN.
// These enable auth on their own and are kept out of config.yaml (no redaction round-trip).
func (s *Server) SetExtraAuthTokens(tokens []string) {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if v := strings.TrimSpace(t); v != "" {
			out = append(out, v)
		}
	}
	s.extraAuthTokens = out
}

// authPolicyNow builds the current policy from the atomic config and any extra tokens.
func (s *Server) authPolicyNow() authPolicy {
	var pol authPolicy
	if c := s.activeCfg(); c != nil {
		pol.tokens = append(pol.tokens, c.HTTPServer.EffectiveAuthTokens()...)
		pol.publicDocs = c.HTTPServer.PublicDocs
		pol.ticketsOnly = c.HTTPServer.StreamTicketsOnly
	}
	if len(s.extraAuthTokens) > 0 {
		pol.tokens = append(pol.tokens, s.extraAuthTokens...)
	}
	pol.login = s.loginPolicyNow()
	// Auth is active whenever at least one credential is configured: a token
	// (YAML, CLI, or env) or the web sign-in account. Either one closes the same
	// gate, so turning on the form protects the API even with no token set.
	pol.enabled = len(pol.tokens) > 0 || pol.login.enabled
	return pol
}

// authGate wraps next with per-request bearer authentication. When no policy is active it is a
// transparent pass-through, so unauthenticated deployments behave exactly as before.
func (s *Server) authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pol := s.authPolicyNow()
		_, pattern := s.mux.Handler(r)
		if !pol.enabled || !isProtectedPattern(pattern, pol.publicDocs) {
			next.ServeHTTP(w, r)
			return
		}
		// The editor plugins drive the IDE routes from this machine and present
		// no credential. They are open to exactly that client and to nobody else:
		// GET /foxxycode/ide/events streams the contents of the files the editor
		// has open, which is not something a token-less remote may read.
		if isIDELocalPattern(pattern) && isDirectLoopbackClient(r) {
			next.ServeHTTP(w, r)
			return
		}
		got := bearerToken(r)
		if got == "" && isSSETokenPattern(pattern) {
			// EventSource cannot set an Authorization header cross-origin, so the SSE routes
			// also accept ?access_token= (these routes only). A query string is logged all
			// over the place, so a single-use stream ticket is tried first and consumed here;
			// the durable token still works unless the operator set stream_tickets_only.
			offered := streamCredential(r)
			if s.streamTickets.consume(offered, time.Now()) {
				next.ServeHTTP(w, r)
				return
			}
			if !pol.ticketsOnly {
				got = offered
			}
		}
		if acceptBearer(pol.tokens, got) {
			next.ServeHTTP(w, r)
			return
		}
		// A signed-in browser carries a cookie instead of a token. Same-origin
		// requests send it on their own, which is why the SPA needs no
		// ?access_token= on its event streams.
		if _, ok := s.sessionFromRequest(r, pol.login); ok {
			// A cookie travels with any request the browser makes, including one
			// a page on another site caused, so the writes are checked for where
			// they came from. Token clients never reach this branch.
			if !isStateChanging(r) || isSameOriginRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		if s.loopbackPassesSignIn(r, pol) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="foxxycode"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// subtleEqual compares two strings without leaking which byte differed.
func subtleEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// isProtectedPattern classifies a matched route pattern rather than using a fragile string prefix:
// the SPA shell and static assets fall through to the "/" catch-all and stay public; every
// registered API route (/v1/*, /foxxycode/*) is protected except the sign-in routes themselves;
// /docs and /openapi.* are protected unless publicDocs is set. The local IDE-integration routes
// (/foxxycode/ide/*) are protected too, and authGate opens them to a direct loopback client
// alone (see isDirectLoopbackClient).
func isProtectedPattern(pattern string, publicDocs bool) bool {
	if pattern == "" || pattern == "/" {
		return false
	}
	// The sign-in routes are the way through the gate, so they cannot be behind
	// it: a browser with no credential yet has to be able to ask whether one is
	// needed and to present one.
	if isAuthRoutePattern(pattern) {
		return false
	}
	if publicDocs && isDocsPattern(pattern) {
		return false
	}
	return true
}

// isIDELocalPattern reports whether a route belongs to the local IDE integration surface, which
// authGate opens to a direct loopback client without a credential.
func isIDELocalPattern(pattern string) bool {
	return strings.Contains(pattern, " /foxxycode/ide/")
}

// SetTrustLoopbackClients makes a direct loopback client count as signed in at the web sign-in
// form. `foxxycode http` turns it on: the IntelliJ and VS Code plugins and `foxxycode desktop`
// start that command on 127.0.0.1 and call it with no credential, so a form would lock their
// panels out. `foxxycode serve` leaves it off, and there the form means what it says. It is a
// property of the server, not a config key, so a settings save (ReplaceConfig) keeps it.
func (s *Server) SetTrustLoopbackClients(on bool) {
	s.trustLoopback.Store(on)
}

// loopbackPassesSignIn reports a request `foxxycode http` lets through as if it had signed in:
// the server trusts loopback clients, the only credential the gate asks for is the sign-in form,
// and the request comes straight from this machine. A bearer token keeps its meaning: with one
// configured, a loopback client presents it as before.
func (s *Server) loopbackPassesSignIn(r *http.Request, pol authPolicy) bool {
	return s.trustLoopback.Load() && pol.login.enabled && len(pol.tokens) == 0 && isDirectLoopbackClient(r)
}

// isDirectLoopbackClient reports a request that comes straight from this machine: the peer is a
// loopback address, the Host it was addressed to is a loopback name, and nothing on the way
// announced itself as a proxy. A reverse proxy on the same host also connects from 127.0.0.1,
// which is why any forwarding header disqualifies the request; a page on another site that
// resolves its own name to 127.0.0.1 (DNS rebinding) still sends that name as Host.
func isDirectLoopbackClient(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(peer, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	if !isLoopbackHostHeader(r.Host) {
		return false
	}
	for _, h := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-IP"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	return true
}

// isLoopbackHostHeader reports a Host header naming this machine: localhost, 127.0.0.1 or ::1,
// with or without a port.
func isLoopbackHostHeader(hostport string) bool {
	h := strings.TrimSpace(hostport)
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	switch strings.ToLower(strings.Trim(h, "[]")) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func isDocsPattern(pattern string) bool {
	switch pattern {
	case "GET /docs", "GET /docs/", "GET /openapi.yaml", "GET /openapi.json":
		return true
	default:
		return false
	}
}

// isSSETokenPattern reports whether a route may authenticate via ?access_token= (EventSource).
func isSSETokenPattern(pattern string) bool {
	return pattern == "GET /foxxycode/sessions/{id}/composer-stream" ||
		pattern == "GET /foxxycode/events"
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) >= len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// acceptBearer reports whether got matches any accepted token using a constant-time compare.
func acceptBearer(tokens []string, got string) bool {
	if got == "" {
		return false
	}
	ok := false
	for _, t := range tokens {
		if subtle.ConstantTimeCompare([]byte(t), []byte(got)) == 1 {
			ok = true
		}
	}
	return ok
}
