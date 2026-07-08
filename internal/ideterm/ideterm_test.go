package ideterm

import (
	"strings"
	"testing"
)

func TestSetGetRoundTrip(t *testing.T) {
	t.Cleanup(Reset)
	Set([]Terminal{
		{ID: "1", Name: "zsh", Shell: "/bin/zsh", Cwd: "/ws", LastCommand: "go test ./...", Output: "ok\n", Active: true},
		{ID: "2", Name: "  ", Output: "dropped"}, // blank name → dropped
		{ID: "3", Name: "dev server", Output: "listening on :3000\n"},
	})

	got := Get()
	if len(got.Terminals) != 2 {
		t.Fatalf("Terminals = %d, want 2 (blank-name dropped)", len(got.Terminals))
	}
	if got.Terminals[0].Name != "zsh" || !got.Terminals[0].Active {
		t.Fatalf("first terminal = %+v", got.Terminals[0])
	}
	if got.Terminals[1].Name != "dev server" {
		t.Fatalf("second terminal = %+v", got.Terminals[1])
	}
	if got.At.IsZero() {
		t.Fatal("At should be set")
	}
}

func TestGetReturnsCopy(t *testing.T) {
	t.Cleanup(Reset)
	Set([]Terminal{{Name: "zsh", Output: "a"}})
	got := Get()
	got.Terminals[0].Output = "mutated"
	if again := Get(); again.Terminals[0].Output != "a" {
		t.Fatalf("stored slice mutated through returned copy: %q", again.Terminals[0].Output)
	}
}

func TestSetCapsOutputTail(t *testing.T) {
	t.Cleanup(Reset)
	big := strings.Repeat("x", maxOutputBytes+500) + "END"
	Set([]Terminal{{Name: "zsh", Output: big}})
	got := Get().Terminals[0].Output
	if len(got) > maxOutputBytes {
		t.Fatalf("output not capped: %d bytes", len(got))
	}
	if !strings.HasSuffix(got, "END") {
		t.Fatalf("cap should keep the tail, got suffix %q", got[len(got)-8:])
	}
}

func TestZeroSnapshot(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	if got := Get(); len(got.Terminals) != 0 {
		t.Fatalf("expected empty snapshot, got %+v", got)
	}
}
