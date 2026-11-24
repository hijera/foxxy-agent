package idehost

import "testing"

func TestSubscribeHandledDeclaresExecutableTools(t *testing.T) {
	h := NewHub()
	if h.HandlesTool("write") {
		t.Fatal("empty hub claims to handle write")
	}
	ch := h.SubscribeHandled([]string{"write", " apply_patch ", ""})
	defer h.Unsubscribe(ch)

	if !h.HandlesTool("write") || !h.HandlesTool("apply_patch") {
		t.Fatal("declared tools are not reported as handled")
	}
	if h.HandlesTool("read") {
		t.Fatal("undeclared tool reported as handled")
	}

	h.Unsubscribe(ch)
	if h.HandlesTool("write") {
		t.Fatal("unsubscribed declaration still counts")
	}
}

func TestSubscribeHandledStarCoversEveryTool(t *testing.T) {
	h := NewHub()
	ch := h.SubscribeHandled([]string{"*"})
	defer h.Unsubscribe(ch)

	for _, name := range []string{"write", "read", "mcp__srv__tool"} {
		if !h.HandlesTool(name) {
			t.Fatalf("%q not covered by *", name)
		}
	}
}

func TestPlainSubscribeDeclaresNothing(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	if !h.HasSubscribers() {
		t.Fatal("HasSubscribers = false after subscribe")
	}
	if h.HandlesTool("write") {
		t.Fatal("plain subscriber claims to handle write")
	}
}

func TestDispatchToolSkipsUnavailableClients(t *testing.T) {
	h := NewHub()
	watcher := h.Subscribe()
	unrelated := h.SubscribeHandled([]string{"read"})
	full := h.SubscribeHandled([]string{"write"})
	for i := 0; i < cap(full); i++ {
		full <- Event{Type: "edit_applied"}
	}
	ev := Event{Type: "tool_execute", ToolName: "write", Nonce: "secret"}
	if h.DispatchTool(ev) {
		t.Fatal("dispatched with no capable client able to receive")
	}
	available := h.SubscribeHandled([]string{"write"})
	if !h.DispatchTool(ev) {
		t.Fatal("did not skip the full subscriber")
	}
	if got := <-available; got != ev {
		t.Fatalf("delivered %+v, want %+v", got, ev)
	}
	if len(watcher) != 0 || len(unrelated) != 0 || len(full) != cap(full) {
		t.Fatal("execution leaked to another subscriber")
	}
	h.Unsubscribe(available)
	if h.DispatchTool(ev) {
		t.Fatal("dispatched to an unsubscribed client")
	}
}

func TestEditNotificationsStillBroadcast(t *testing.T) {
	h := NewHub()
	watcher := h.Subscribe()
	executor := h.SubscribeHandled([]string{"write"})
	ev := Event{Type: "edit_applied", Path: "file.txt"}
	h.Broadcast(ev)
	for _, ch := range []chan Event{watcher, executor} {
		select {
		case got := <-ch:
			if got != ev {
				t.Fatalf("got %+v, want %+v", got, ev)
			}
		default:
			t.Fatal("edit notification was not broadcast")
		}
	}
}
