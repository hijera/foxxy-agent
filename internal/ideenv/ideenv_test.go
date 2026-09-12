package ideenv

import (
	"strings"
	"testing"
)

func TestSetGetRoundTrip(t *testing.T) {
	t.Cleanup(Reset)
	Set([]string{"/ws/a.go", "  ", "/ws/b.go"}, "/ws/a.go", nil)

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
	if got.Selection != nil {
		t.Fatalf("Selection = %+v, want nil", got.Selection)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	t.Cleanup(Reset)
	Set([]string{"/ws/a.go"}, "", nil)
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
	if got.ActiveFile != "" || len(got.OpenFiles) != 0 || got.Selection != nil {
		t.Fatalf("expected empty snapshot, got %+v", got)
	}
}

func TestSetStoresSelection(t *testing.T) {
	t.Cleanup(Reset)
	Set([]string{"/ws/a.go"}, "/ws/a.go", &Selection{File: "/ws/a.go", StartLine: 21, EndLine: 31, Text: "x := 1"})
	got := Get()
	if got.Selection == nil || got.Selection.File != "/ws/a.go" || got.Selection.StartLine != 21 || got.Selection.EndLine != 31 || got.Selection.Text != "x := 1" {
		t.Fatalf("Selection = %+v", got.Selection)
	}
	got.Selection.Text = "mutated"
	if again := Get(); again.Selection.Text != "x := 1" {
		t.Fatal("stored selection was mutated through returned copy")
	}
}

func TestSetDropsInvalidSelection(t *testing.T) {
	t.Cleanup(Reset)
	for _, sel := range []*Selection{
		{File: "  ", StartLine: 1, EndLine: 1, Text: "x"},
		{File: "/ws/a.go", StartLine: 0, EndLine: 3, Text: "x"},
		{File: "/ws/a.go", StartLine: 5, EndLine: 4, Text: "x"},
		{File: "/ws/a.go", StartLine: 1, EndLine: 1, Text: "   "},
	} {
		Set(nil, "", sel)
		if got := Get(); got.Selection != nil {
			t.Fatalf("selection %+v must be dropped, got %+v", sel, got.Selection)
		}
	}
}

func TestSetCapsSelectionText(t *testing.T) {
	t.Cleanup(Reset)
	long := strings.Repeat("y", 20*1024)
	Set(nil, "", &Selection{File: "/ws/a.go", StartLine: 1, EndLine: 2, Text: long})
	got := Get()
	if got.Selection == nil {
		t.Fatal("selection dropped")
	}
	if len(got.Selection.Text) != maxSelectionBytes {
		t.Fatalf("len = %d, want %d", len(got.Selection.Text), maxSelectionBytes)
	}
	if !strings.HasSuffix(long, got.Selection.Text) {
		t.Fatal("cap must keep the tail")
	}
}
