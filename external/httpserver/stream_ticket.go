//go:build http

package httpserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EventSource cannot set an Authorization header, so the SSE routes accept the
// credential as ?access_token= — and a query string is exactly what ends up in
// access logs, proxy logs, browser history and Referer headers. Handing the
// long-lived API token to those routes therefore parks a working credential in
// half a dozen places nobody audits.
//
// A stream ticket is the narrow alternative sketched in docs/remote-control.md
// §3.3: an already-authenticated client mints one, uses it for exactly one SSE
// subscription, and by the time the URL reaches a log the value is spent. A
// ticket is useless anywhere else — the auth gate accepts it only on the SSE
// patterns, never on /v1/* or the rest of /foxxycode/*.
const (
	// streamTicketTTL bounds how long an unused ticket stays valid. Long enough to
	// cover a mint-then-connect round trip (including a slow reverse proxy), short
	// enough that a leaked-but-unused ticket is worthless within the minute.
	streamTicketTTL = 60 * time.Second

	// streamTicketMax caps outstanding tickets so a caller that mints without ever
	// connecting cannot grow the map without bound. Reaching the cap evicts the
	// oldest ticket, which is at worst one client having to mint again.
	streamTicketMax = 1024

	// streamTicketBytes is the entropy per ticket (256 bits).
	streamTicketBytes = 32
)

// streamTicketStore holds unspent tickets. Tickets live only in this process:
// they are not persisted, and a restart invalidates every outstanding one, which
// is the correct behaviour for a credential measured in seconds.
type streamTicketStore struct {
	mu      sync.Mutex
	tickets map[string]time.Time // ticket -> expiry
}

func newStreamTicketStore() *streamTicketStore {
	return &streamTicketStore{tickets: map[string]time.Time{}}
}

// mint creates a ticket valid for streamTicketTTL.
func (s *streamTicketStore) mint(now time.Time) (string, time.Time, error) {
	raw := make([]byte, streamTicketBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := now.Add(streamTicketTTL)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	if len(s.tickets) >= streamTicketMax {
		s.evictOldestLocked()
	}
	s.tickets[token] = expires
	return token, expires, nil
}

// consume reports whether got is an unspent, unexpired ticket, removing it in the
// same critical section so a ticket authenticates exactly one subscription even
// when two requests race.
//
// The lookup walks every ticket with a constant-time compare rather than indexing
// the map: a map lookup on attacker-supplied input leaks timing through the hash
// comparison, and the table is small enough (streamTicketMax) that the scan costs
// nothing on the connect path.
func (s *streamTicketStore) consume(got string, now time.Time) bool {
	// Nil-safe: a Server built by something other than New has no store, and a
	// missing store simply means no ticket can be valid.
	if s == nil || got == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	match := ""
	found := false
	for token := range s.tickets {
		if subtle.ConstantTimeCompare([]byte(token), []byte(got)) == 1 {
			match = token
			found = true
		}
	}
	if !found {
		return false
	}
	delete(s.tickets, match)
	return true
}

// pruneLocked drops expired tickets. Called on both paths, so an idle server does
// not need a sweeper goroutine.
func (s *streamTicketStore) pruneLocked(now time.Time) {
	for token, expires := range s.tickets {
		if !now.Before(expires) {
			delete(s.tickets, token)
		}
	}
}

// evictOldestLocked removes the ticket closest to expiry.
func (s *streamTicketStore) evictOldestLocked() {
	oldest := ""
	var oldestAt time.Time
	for token, expires := range s.tickets {
		if oldest == "" || expires.Before(oldestAt) {
			oldest, oldestAt = token, expires
		}
	}
	if oldest != "" {
		delete(s.tickets, oldest)
	}
}

// foxxycodeStreamTicketPost mints a ticket for the caller. It sits behind the
// normal bearer gate, so reaching this handler already proves possession of the
// real credential; the ticket it returns is a strictly weaker capability.
//
// When auth is disabled the endpoint reports 409 rather than minting: a ticket
// would authenticate nothing, and silently returning one would suggest the stream
// is protected when it is not.
func (s *Server) foxxycodeStreamTicketPost(w http.ResponseWriter, r *http.Request) {
	if !s.authPolicyNow().enabled {
		http.Error(w, `{"error":{"message":"stream tickets require httpserver.auth_token (or --auth-token / FOXXYCODE_HTTP_TOKEN)"}}`, http.StatusConflict)
		return
	}
	token, expires, err := s.streamTickets.mint(time.Now())
	if err != nil {
		http.Error(w, `{"error":{"message":"could not mint a stream ticket"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// A credential must never be cached by a proxy or the browser.
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.stream_ticket",
		"ticket":    token,
		"expiresIn": int(streamTicketTTL / time.Second),
		"expiresAt": expires.UTC().Format(time.RFC3339),
	})
}

// streamCredential extracts the query credential offered to an SSE route.
func streamCredential(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("access_token"))
}
