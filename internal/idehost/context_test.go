package idehost_test

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/idehost"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
)

func TestContextUsesSuppliedSnapshots(t *testing.T) {
	ideenv.Set([]string{"global.go"}, "global.go", nil)
	t.Cleanup(ideenv.Reset)
	note := idehost.EditorNote("", ideenv.Snapshot{ActiveFile: "local.go"})
	if !strings.Contains(note, "local.go") || strings.Contains(note, "global.go") {
		t.Fatalf("context leaked across hosts: %s", note)
	}
	snap := ideterm.Snapshot{Terminals: []ideterm.Terminal{
		{ID: "a", Name: `build"<&`, Active: true, Output: "done\n"},
	}}
	note = idehost.TerminalMentionNote("@terminal @terminal", snap)
	if strings.Count(note, "<foxxycode_terminal_output ") != 1 || !strings.Contains(note, "&#34;&lt;&amp;") {
		t.Fatalf("mention escaping or deduplication changed: %s", note)
	}
}

func TestHubDoesNotBlockOnSlowSubscribers(t *testing.T) {
	hub := idehost.NewHub()
	ch := hub.Subscribe()
	for i := 0; i < 1000; i++ {
		hub.Broadcast(idehost.Event{Type: "edit_applied", Path: "main.go"})
	}
	if event := <-ch; event.Path != "main.go" {
		t.Fatalf("wrong event: %+v", event)
	}
	hub.Unsubscribe(ch)
	if hub.HasSubscribers() {
		t.Fatal("subscriber retained after disconnect")
	}
}

func TestIndependentHubsDoNotShareEvents(t *testing.T) {
	first, second := idehost.NewHub(), idehost.NewHub()
	a, b := first.Subscribe(), second.Subscribe()
	defer first.Unsubscribe(a)
	defer second.Unsubscribe(b)
	first.Broadcast(idehost.Event{Type: "open_file", Path: "first.go"})
	if event := <-a; event.Path != "first.go" {
		t.Fatalf("wrong event: %+v", event)
	}
	select {
	case event := <-b:
		t.Fatalf("event leaked between hosts: %+v", event)
	default:
	}
}
