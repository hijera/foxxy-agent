package gitws

import (
	"errors"
	"testing"
)

// A nested repository is listed as its folder with a trailing slash
// ("nested/"), while a discard request names the folder cleaned ("nested"):
// the match has to compare cleaned forms. The selection keeps the raw entry,
// because deleteWorktreeFiles reads its trailing slash as the folder marker.
func TestSelectDiscardMatchesNestedRepositoryFolder(t *testing.T) {
	sel, err := selectDiscard(
		nil,
		[]string{"nested/", "top.txt"},
		[]string{"nested/", "top.txt"},
	)
	if err != nil {
		t.Fatalf("selectDiscard: %v", err)
	}
	want := []string{"nested/", "top.txt"}
	if len(sel.untracked) != len(want) {
		t.Fatalf("untracked = %v, want %v", sel.untracked, want)
	}
	for i, u := range want {
		if sel.untracked[i] != u {
			t.Fatalf("untracked = %v, want %v", sel.untracked, want)
		}
	}
}

func TestSelectDiscardStillRefusesUnknownPaths(t *testing.T) {
	_, err := selectDiscard(nil, []string{"nested/"}, []string{"other"})
	if !errors.Is(err, ErrNotChanged) {
		t.Fatalf("err = %v, want ErrNotChanged", err)
	}
}
