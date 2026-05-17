package agent

import (
	"strings"
	"testing"
)

func TestDiscardedPlansPromptBlockEmpty(t *testing.T) {
	if got := discardedPlansPromptBlock(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestDiscardedPlansPromptBlockListsSlugs(t *testing.T) {
	got := discardedPlansPromptBlock([]string{"alpha", "beta"})
	if got == "" {
		t.Fatal("expected block")
	}
	for _, want := range []string{"alpha", "beta", "fresh slug"} {
		if !strings.Contains(got, want) {
			t.Fatalf("block missing %q: %s", want, got)
		}
	}
}
