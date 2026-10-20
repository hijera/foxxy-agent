//go:build http

package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hijera/foxxycode-agent/internal/idehost"
)

type ideEvent = idehost.Event

var ideEvents = idehost.NewHub()

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

	ch := ideEvents.Subscribe()
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
