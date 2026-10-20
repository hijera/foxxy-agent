package idehost

import "sync"

// Event is one structured edit event delivered to native editor clients (e.g. the
// IntelliJ plugin) over the /foxxycode/ide/events stream.
type Event struct {
	// "edit_proposed" | "edit_applied" | "open_file" | "reveal_file".
	// open_file opens the path in the editor; reveal_file selects it in the OS
	// file manager, which is what a downloaded PDF or DOCX needs.
	Type       string `json:"type"`
	ToolCallID string `json:"toolCallId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Path       string `json:"path"` // absolute path
	Before     string `json:"before"`
	After      string `json:"after"`
}

// Hub fans structured edit events out to every connected IDE client. It is a
// process-global broadcast (there is one foxxycode http per workspace); subscribers that fall
// behind drop events rather than blocking the agent.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewHub creates an independent host event stream.
func NewHub() *Hub { return &Hub{subs: make(map[chan Event]struct{})} }

func (h *Hub) Subscribe() chan Event {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default: // drop for slow subscribers
		}
	}
}

// HasSubscribers reports whether any IDE client is connected. Used to avoid computing
// diff previews when nobody is listening.
func (h *Hub) HasSubscribers() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs) > 0
}
