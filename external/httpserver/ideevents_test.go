//go:build http

package httpserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hijera/foxxy-agent/internal/acp"
	"github.com/hijera/foxxy-agent/internal/config"
)

func recvIdeEvent(t *testing.T, ch chan ideEvent) ideEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ide event")
		return ideEvent{}
	}
}

func TestIdeEventHubBroadcast(t *testing.T) {
	ch := ideEvents.subscribe()
	defer ideEvents.unsubscribe(ch)

	if !ideEvents.hasSubscribers() {
		t.Fatal("hasSubscribers = false after subscribe")
	}
	ideEvents.broadcast(ideEvent{Type: "edit_applied", Path: "/x"})
	if ev := recvIdeEvent(t, ch); ev.Type != "edit_applied" || ev.Path != "/x" {
		t.Fatalf("got %+v", ev)
	}
}

func TestBroadcastEditProposed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("keep old keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	ch := ideEvents.subscribe()
	defer ideEvents.unsubscribe(ch)

	s := NewSender(&config.Config{}, nil, false, "m")
	s.SetCWD(dir)
	s.broadcastEditProposed("sess1", "call1", "edit", `{"path":"f.txt","oldString":"old","newString":"new"}`)

	ev := recvIdeEvent(t, ch)
	if ev.Type != "edit_proposed" || ev.SessionID != "sess1" || ev.ToolCallID != "call1" {
		t.Fatalf("meta wrong: %+v", ev)
	}
	if ev.Path != path {
		t.Errorf("path = %q, want %q", ev.Path, path)
	}
	if ev.Before != "keep old keep" || ev.After != "keep new keep" {
		t.Errorf("before/after = %q/%q", ev.Before, ev.After)
	}
	// The proposal must not have written to disk.
	if b, _ := os.ReadFile(path); string(b) != "keep old keep" {
		t.Errorf("broadcastEditProposed mutated file: %q", b)
	}
}

func TestBroadcastEditProposedSkipsNonWriteTools(t *testing.T) {
	ch := ideEvents.subscribe()
	defer ideEvents.unsubscribe(ch)

	s := NewSender(&config.Config{}, nil, false, "m")
	s.broadcastEditProposed("s", "c", "run_command", `{"command":"ls"}`)

	select {
	case ev := <-ch:
		t.Fatalf("unexpected event for non-write tool: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestSendSessionUpdateFileEditBroadcasts(t *testing.T) {
	ch := ideEvents.subscribe()
	defer ideEvents.unsubscribe(ch)

	// stream=false: file_edit must still broadcast on the side channel.
	s := NewSender(&config.Config{}, nil, false, "m")
	err := s.SendSessionUpdate("sessX", acp.FileEditUpdate{
		SessionUpdate: acp.UpdateTypeFileEdit,
		ToolCallID:    "tcX",
		ToolName:      "write",
		Path:          "/abs/p",
		Before:        "a",
		After:         "b",
	})
	if err != nil {
		t.Fatalf("SendSessionUpdate: %v", err)
	}
	ev := recvIdeEvent(t, ch)
	if ev.Type != "edit_applied" || ev.SessionID != "sessX" || ev.ToolCallID != "tcX" || ev.After != "b" {
		t.Fatalf("got %+v", ev)
	}
}
