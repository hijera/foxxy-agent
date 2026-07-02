package session_test

import (
	"testing"

	"github.com/hijera/foxxy-agent/internal/session"
)

func TestExtractAtFilePathsFromTextSkipsProseAndFolders(t *testing.T) {
	got := session.ExtractAtFilePathsFromText("see @a/b.txt and @a/ and @a/b.txt")
	if len(got) != 1 || got[0] != "a/b.txt" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractAtFilePathsFromTextSpaceInName(t *testing.T) {
	got := session.ExtractAtFilePathsFromText("open @readme copy.md now")
	if len(got) != 1 || got[0] != "readme copy.md" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractAtFilePathsFromTextSkipsCodeFence(t *testing.T) {
	s := "```\n@x.go\n```"
	got := session.ExtractAtFilePathsFromText(s)
	if len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}
