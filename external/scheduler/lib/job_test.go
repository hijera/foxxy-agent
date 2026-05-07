//go:build scheduler

package scheduler

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseCronUTC_MatchesStandardFiveField(t *testing.T) {
	_, err := ParseCronUTC("0 9 * * 1-5")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNextScheduledUTCHourly(t *testing.T) {
	s, err := ParseCronUTC("0 * * * *")
	if err != nil {
		t.Fatal(err)
	}
	last, err := time.Parse(time.RFC3339, "2020-01-15T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	next := NextScheduledUTC(s, last)
	if want := "2020-01-15T11:00:00Z"; next.Format(time.RFC3339) != want {
		t.Fatalf("got %s want %s", next.Format(time.RFC3339), want)
	}
}

func TestStatePathLockPath(t *testing.T) {
	p := filepath.FromSlash("/x/y/job.md")
	if g := StatePath(p); g != filepath.FromSlash("/x/y/job.state") {
		t.Fatalf("StatePath %q", g)
	}
	if g := LockPath(p); g != filepath.FromSlash("/x/y/job.lock") {
		t.Fatalf("LockPath %q", g)
	}
}

func TestParseJobFromBytes(t *testing.T) {
	raw := `---
description: "Test"
schedule: "0 0 * * *"
---
Do something
`
	fm, err := ParseJobFromBytes([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if fm.Description != "Test" {
		t.Fatalf("description %q", fm.Description)
	}
}
