package idecopy_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/idecopy"
)

func TestOfferAndMatchFileNormalizesLineEndings(t *testing.T) {
	idecopy.Reset()
	idecopy.Offer(idecopy.Candidate{
		Kind: idecopy.KindFile, PathAbs: "C:\\w\\Dockerfile", StartLine: 21, EndLine: 31,
		Text: "FROM x\r\nRUN y\r\n",
	})
	got, ok := idecopy.MatchFile("FROM x\nRUN y")
	if !ok {
		t.Fatal("expected a match")
	}
	if got.PathAbs != "C:\\w\\Dockerfile" || got.StartLine != 21 || got.EndLine != 31 {
		t.Fatalf("got %+v", got)
	}
}

func TestMatchFileIsExactOnly(t *testing.T) {
	idecopy.Reset()
	idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/a.go", StartLine: 1, EndLine: 2, Text: "alpha\nbeta"})
	if _, ok := idecopy.MatchFile("alpha"); ok {
		t.Fatal("substring must not match")
	}
	if _, ok := idecopy.MatchFile("alpha\nbeta\ngamma"); ok {
		t.Fatal("superset must not match")
	}
}

func TestMatchFilePrefersNewestCandidate(t *testing.T) {
	idecopy.Reset()
	idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/old.go", StartLine: 1, EndLine: 1, Text: "same text\nhere"})
	idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/new.go", StartLine: 7, EndLine: 8, Text: "same text\nhere"})
	got, ok := idecopy.MatchFile("same text\nhere")
	if !ok || got.PathAbs != "/w/new.go" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestOfferRingCapsAndDedupesHead(t *testing.T) {
	idecopy.Reset()
	for i := 0; i < 3; i++ {
		idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/a.go", StartLine: 1, EndLine: 1, Text: "dup\ndup"})
	}
	if n := len(idecopy.Candidates()); n != 1 {
		t.Fatalf("head dedupe failed: %d entries", n)
	}
	texts := []string{"one\n1", "two\n2", "three\n3", "four\n4", "five\n5", "six\n6"}
	for _, s := range texts {
		idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/a.go", StartLine: 1, EndLine: 1, Text: s})
	}
	if n := len(idecopy.Candidates()); n != 5 {
		t.Fatalf("ring cap failed: %d entries", n)
	}
	if _, ok := idecopy.MatchFile("dup\ndup"); ok {
		t.Fatal("oldest entry must have been evicted")
	}
	if _, ok := idecopy.MatchFile("six\n6"); !ok {
		t.Fatal("newest entry must match")
	}
}

func TestOfferDropsOversizeAndEmpty(t *testing.T) {
	idecopy.Reset()
	idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/a.go", StartLine: 1, EndLine: 1, Text: strings.Repeat("x", 64*1024+1)})
	idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/a.go", StartLine: 1, EndLine: 1, Text: "   \n  "})
	if n := len(idecopy.Candidates()); n != 0 {
		t.Fatalf("got %d entries", n)
	}
}

func TestMatchFileIgnoresExpiredCandidates(t *testing.T) {
	idecopy.Reset()
	idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindFile, PathAbs: "/w/a.go", StartLine: 1, EndLine: 1, Text: "stale\ntext", At: time.Now().Add(-16 * time.Minute)})
	if _, ok := idecopy.MatchFile("stale\ntext"); ok {
		t.Fatal("expired candidate must not match")
	}
}

func TestMatchFileSkipsTerminalCandidates(t *testing.T) {
	idecopy.Reset()
	idecopy.Offer(idecopy.Candidate{Kind: idecopy.KindTerminal, TerminalName: "dev", Text: "npm run dev\nready"})
	if _, ok := idecopy.MatchFile("npm run dev\nready"); ok {
		t.Fatal("terminal candidate must not match as file")
	}
	got, ok := idecopy.MatchTerminal("npm run dev\nready")
	if !ok || got.TerminalName != "dev" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestNormalize(t *testing.T) {
	if got := idecopy.Normalize("a\r\nb\r\n"); got != "a\nb" {
		t.Fatalf("got %q", got)
	}
}
