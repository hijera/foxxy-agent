//go:build http

package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/idehost"
)

type ideEvent = idehost.Event

var ideEvents = idehost.NewHub()

// ideHandlesFromQuery reads the comma-separated tool names a subscriber can
// execute from ?handles=... on the SSE stream ("*" means all of them). A
// subscriber that sends none only watches events.
func ideHandlesFromQuery(r *http.Request) []string {
	raw := r.URL.Query().Get("handles")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// foxxycodeIdeEvents serves the IDE edit-event SSE stream (GET /foxxycode/ide/events).
func (s *Server) foxxycodeIdeEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":{"message":"streaming unsupported"}}`, http.StatusInternalServerError)
		return
	}
	writeSSEHeaders(w)

	ch := ideEvents.SubscribeHandled(ideHandlesFromQuery(r))
	defer ideEvents.Unsubscribe(ch)

	// Prime the stream so clients see a response immediately.
	_, _ = io.WriteString(w, ": ide events connected\n\n")
	fl.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			line, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, line); err != nil {
				return
			}
			fl.Flush()
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
