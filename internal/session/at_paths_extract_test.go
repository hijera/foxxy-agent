package session_test

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
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

// Parity strings below are shared with external/ui/src/ui/skills/draftAt.test.ts.

func TestExtractAtFileRefsFromTextLineRange(t *testing.T) {
	got := session.ExtractAtFileRefsFromText("see @Dockerfile:21-31 ok")
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	r := got[0]
	if r.Path != "Dockerfile" || r.StartLine != 21 || r.EndLine != 31 {
		t.Fatalf("got %+v", r)
	}
}

func TestExtractAtFileRefsFromTextSingleLineRange(t *testing.T) {
	got := session.ExtractAtFileRefsFromText("@a/b.go:5-5")
	if len(got) != 1 || got[0].Path != "a/b.go" || got[0].StartLine != 5 || got[0].EndLine != 5 {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractAtFileRefsFromTextSingleNumberIsNotARange(t *testing.T) {
	got := session.ExtractAtFileRefsFromText("open @x.go:21 now")
	if len(got) != 1 || got[0].Path != "x.go" || got[0].StartLine != 0 || got[0].EndLine != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractAtFileRefsFromTextRangeTrailingGarbage(t *testing.T) {
	got := session.ExtractAtFileRefsFromText("see @file.go:21-31x here")
	if len(got) != 1 || got[0].Path != "file.go" || got[0].StartLine != 0 || got[0].EndLine != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractAtFileRefsFromTextInvalidRangeDropped(t *testing.T) {
	for _, s := range []string{"@f.go:31-21 x", "@f.go:0-5 x"} {
		got := session.ExtractAtFileRefsFromText(s)
		if len(got) != 1 || got[0].Path != "f.go" || got[0].StartLine != 0 || got[0].EndLine != 0 {
			t.Fatalf("%q: got %+v", s, got)
		}
	}
}

func TestExtractAtFileRefsFromTextRangeAtCRLFBoundary(t *testing.T) {
	got := session.ExtractAtFileRefsFromText("take @f.go:2-4\r\nplease")
	if len(got) != 1 || got[0].Path != "f.go" || got[0].StartLine != 2 || got[0].EndLine != 4 {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractAtFileRefsFromTextRangeBeforePunctuation(t *testing.T) {
	got := session.ExtractAtFileRefsFromText("check @f.go:2-4, then run")
	if len(got) != 1 || got[0].Path != "f.go" || got[0].StartLine != 2 || got[0].EndLine != 4 {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractAtFileRefsFromTextExcludesTerminalToken(t *testing.T) {
	for _, s := range []string{"check @terminal", "check @terminal:dev output", "check @terminal:21-31"} {
		got := session.ExtractAtFileRefsFromText(s)
		if len(got) != 0 {
			t.Fatalf("%q: got %+v", s, got)
		}
	}
}

func TestExtractAtFilePathsFromTextExcludesTerminalToken(t *testing.T) {
	got := session.ExtractAtFilePathsFromText("check @terminal and @terminal:dev")
	if len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestExtractAtFileRefsFromTextDedupesByPathAndRange(t *testing.T) {
	got := session.ExtractAtFileRefsFromText("@f.go:1-2 @f.go:1-2 @f.go:3-4 @f.go")
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].StartLine != 1 || got[0].EndLine != 2 || got[1].StartLine != 3 || got[1].EndLine != 4 || got[2].StartLine != 0 {
		t.Fatalf("got %+v", got)
	}
	paths := session.ExtractAtFilePathsFromText("@f.go:1-2 @f.go:3-4 @f.go")
	if len(paths) != 1 || paths[0] != "f.go" {
		t.Fatalf("got %q", paths)
	}
}
