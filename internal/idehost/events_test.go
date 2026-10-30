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
