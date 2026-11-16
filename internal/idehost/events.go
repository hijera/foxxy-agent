package idehost

import (
	"strings"
	"sync"
)

// Event is one structured edit event delivered to native editor clients (e.g. the
// IntelliJ plugin) over the /foxxycode/ide/events stream.
type Event struct {
	// "edit_proposed" | "edit_applied" | "open_file" | "reveal_file" | "tool_execute".
	// open_file opens the path in the editor; reveal_file selects it in the OS
	// file manager, which is what a downloaded PDF or DOCX needs. tool_execute
	// asks the client to execute a tool call (see ArgsJSON/Nonce) and answer on
	// POST /foxxycode/ide/tool-result.
	Type       string `json:"type"`
	ToolCallID string `json:"toolCallId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Path       string `json:"path"` // absolute path
	Before     string `json:"before"`
	After      string `json:"after"`
	// ToolName, ArgsJSON and Nonce are set on tool_execute only: which tool
	// the model called (built-in or MCP name), the raw arguments the client
	// executes, and the token its result must echo.
	ToolName string `json:"tool,omitempty"`
	ArgsJSON string `json:"args,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
}

// Hub fans structured edit events out to every connected IDE client. It is a
// process-global broadcast (there is one foxxycode http per workspace); subscribers that fall
// behind drop events rather than blocking the agent.
type Hub struct {
	mu sync.Mutex
	// subs maps each subscriber to the tool names it declared itself able to
	// execute ("*" means all of them).
	subs map[chan Event]map[string]struct{}
}

// NewHub creates an independent host event stream.
func NewHub() *Hub { return &Hub{subs: make(map[chan Event]map[string]struct{})} }

// Subscribe registers a subscriber that declares no tool executions.
func (h *Hub) Subscribe() chan Event { return h.SubscribeHandled(nil) }

// SubscribeHandled registers a subscriber that can execute the named tool
// calls through the editor. Names are matched exactly as the model called the
// tool, so MCP names ("server__tool") work, and "*" covers every call.
func (h *Hub) SubscribeHandled(handles []string) chan Event {
	set := make(map[string]struct{}, len(handles))
	for _, name := range handles {
		if name = strings.TrimSpace(name); name != "" {
			set[name] = struct{}{}
		}
	}
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = set
	h.mu.Unlock()
	return ch
}

// HandlesTool reports whether some connected client declared itself able to
// execute the named tool call.
func (h *Hub) HandlesTool(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		if _, all := set["*"]; all {
			return true
		}
		if _, ok := set[name]; ok {
			return true
		}
	}
	return false
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
