//go:build http

package httpserver

import (
	"crypto/subtle"
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
	// Auth is active whenever at least one token is configured (YAML, CLI, or env).
	pol.enabled = len(pol.tokens) > 0
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
		if !acceptBearer(pol.tokens, got) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="foxxycode"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isProtectedPattern classifies a matched route pattern rather than using a fragile string prefix:
// the SPA shell and static assets fall through to the "/" catch-all and stay public; every
// registered API route (/v1/*, /foxxycode/*) is protected; /docs and /openapi.* are protected
// unless publicDocs is set. The local IDE-integration routes (/foxxycode/ide/*) stay public: they
// are driven by the editor plugin on the same machine and predate the token, so requiring one
// would break the IDE integration for every existing user.
func isProtectedPattern(pattern string, publicDocs bool) bool {
	if pattern == "" || pattern == "/" {
		return false
	}
	if isIDELocalPattern(pattern) {
		return false
	}
	if publicDocs && isDocsPattern(pattern) {
		return false
	}
	return true
}

// isIDELocalPattern reports whether a route belongs to the local IDE integration surface, which is
// exempt from bearer auth (see isProtectedPattern).
func isIDELocalPattern(pattern string) bool {
	return strings.Contains(pattern, " /foxxycode/ide/")
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
