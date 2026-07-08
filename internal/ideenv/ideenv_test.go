package ideenv

import "testing"

func TestSetGetRoundTrip(t *testing.T) {
	t.Cleanup(Reset)
	Set([]string{"/ws/a.go", "  ", "/ws/b.go"}, "/ws/a.go")

	got := Get()
	if got.ActiveFile != "/ws/a.go" {
		t.Fatalf("ActiveFile = %q, want /ws/a.go", got.ActiveFile)
	}
	if len(got.OpenFiles) != 2 || got.OpenFiles[0] != "/ws/a.go" || got.OpenFiles[1] != "/ws/b.go" {
		t.Fatalf("OpenFiles = %v, want [/ws/a.go /ws/b.go] (blanks dropped)", got.OpenFiles)
	}
	if got.At.IsZero() {
		t.Fatal("At should be set")
	}
}

func TestGetReturnsCopy(t *testing.T) {
	t.Cleanup(Reset)
	Set([]string{"/ws/a.go"}, "")
	got := Get()
	got.OpenFiles[0] = "mutated"
	if again := Get(); again.OpenFiles[0] != "/ws/a.go" {
		t.Fatalf("stored slice was mutated through returned copy: %v", again.OpenFiles)
	}
}

func TestZeroSnapshot(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	got := Get()
	if got.ActiveFile != "" || len(got.OpenFiles) != 0 {
		t.Fatalf("expected empty snapshot, got %+v", got)
	}
}
