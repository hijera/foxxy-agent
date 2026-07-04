//go:build http

package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ideEvent is one structured edit event delivered to native editor clients (e.g. the
// IntelliJ plugin) over the /foxxycode/ide/events stream.
type ideEvent struct {
	Type       string `json:"type"` // "edit_proposed" | "edit_applied"
	ToolCallID string `json:"toolCallId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Path       string `json:"path"` // absolute path
	Before     string `json:"before"`
	After      string `json:"after"`
}

// ideEventHub fans structured edit events out to every connected IDE client. It is a
// process-global broadcast (there is one foxxycode http per workspace); subscribers that fall
// behind drop events rather than blocking the agent.
type ideEventHub struct {
	mu   sync.Mutex
	subs map[chan ideEvent]struct{}
}

var ideEvents = &ideEventHub{subs: make(map[chan ideEvent]struct{})}

func (h *ideEventHub) subscribe() chan ideEvent {
	ch := make(chan ideEvent, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *ideEventHub) unsubscribe(ch chan ideEvent) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *ideEventHub) broadcast(ev ideEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default: // drop for slow subscribers
		}
	}
}

// hasSubscribers reports whether any IDE client is connected. Used to avoid computing
// diff previews when nobody is listening.
func (h *ideEventHub) hasSubscribers() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs) > 0
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
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := ideEvents.subscribe()
	defer ideEvents.unsubscribe(ch)

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
