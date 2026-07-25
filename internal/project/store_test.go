package project

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func mustDir(t *testing.T, parent, name string) string {
	t.Helper()
	p := filepath.Join(parent, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	return p
}

func TestOpenMissingFile(t *testing.T) {
	home := t.TempDir()
	s, err := Open(home)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := s.Current(); got != "" {
		t.Fatalf("Current() = %q, want empty", got)
	}
	if got := s.Recent(); len(got) != 0 {
		t.Fatalf("Recent() = %v, want empty", got)
	}
}

func TestOpenCorruptFile(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "projects.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(home)
	if err != nil {
		t.Fatalf("Open on corrupt file: %v", err)
	}
	if got := s.Current(); got != "" {
		t.Fatalf("Current() = %q, want empty", got)
	}
}

func TestSetCurrentPersists(t *testing.T) {
	home := t.TempDir()
	proj := mustDir(t, t.TempDir(), "proj")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent(proj); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}
	if got := s.Current(); got != proj {
		t.Fatalf("Current() = %q, want %q", got, proj)
	}
	recent := s.Recent()
	if len(recent) != 1 || recent[0].Path != proj {
		t.Fatalf("Recent() = %v, want single entry %q", recent, proj)
	}
	if recent[0].LastOpenedAt == "" {
		t.Fatal("LastOpenedAt empty")
	}

	// Reopen from disk.
	s2, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Current(); got != proj {
		t.Fatalf("reopened Current() = %q, want %q", got, proj)
	}
	if got := s2.Recent(); len(got) != 1 || got[0].Path != proj {
		t.Fatalf("reopened Recent() = %v, want single entry %q", got, proj)
	}
}

func TestSetCurrentValidation(t *testing.T) {
	home := t.TempDir()
	base := t.TempDir()
	file := filepath.Join(base, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"nonexistent", filepath.Join(base, "nope")},
		{"file not dir", file},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.SetCurrent(tc.path); err == nil {
				t.Fatalf("SetCurrent(%q) succeeded, want error", tc.path)
			}
		})
	}
	if got := s.Current(); got != "" {
		t.Fatalf("Current() after failed sets = %q, want empty", got)
	}
}

func TestRecentDedupeAndOrder(t *testing.T) {
	home := t.TempDir()
	base := t.TempDir()
	a := mustDir(t, base, "a")
	b := mustDir(t, base, "b")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{a, b, a} {
		if err := s.SetCurrent(p); err != nil {
			t.Fatalf("SetCurrent(%q): %v", p, err)
		}
	}
	recent := s.Recent()
	if len(recent) != 2 {
		t.Fatalf("Recent() has %d entries, want 2: %v", len(recent), recent)
	}
	if recent[0].Path != a || recent[1].Path != b {
		t.Fatalf("Recent() order = [%s, %s], want [a, b] = [%s, %s]", recent[0].Path, recent[1].Path, a, b)
	}
}

func TestRecentDedupeCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only case-insensitivity")
	}
	home := t.TempDir()
	a := mustDir(t, t.TempDir(), "Proj")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent(a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent(strings.ToUpper(a)); err != nil {
		t.Fatal(err)
	}
	if got := s.Recent(); len(got) != 1 {
		t.Fatalf("Recent() has %d entries, want 1 (case-insensitive dedupe): %v", len(got), got)
	}
}

func TestRecentCap(t *testing.T) {
	home := t.TempDir()
	base := t.TempDir()

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxRecent+5; i++ {
		p := mustDir(t, base, "p"+strconv.Itoa(i))
		if err := s.SetCurrent(p); err != nil {
			t.Fatalf("SetCurrent #%d: %v", i, err)
		}
	}
	recent := s.Recent()
	if len(recent) != maxRecent {
		t.Fatalf("Recent() has %d entries, want %d", len(recent), maxRecent)
	}
	// Most recent first.
	want := filepath.Join(base, "p"+strconv.Itoa(maxRecent+4))
	if recent[0].Path != want {
		t.Fatalf("Recent()[0] = %q, want %q", recent[0].Path, want)
	}
}

func TestCurrentEmptyWhenDirVanished(t *testing.T) {
	home := t.TempDir()
	proj := mustDir(t, t.TempDir(), "gone")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent(proj); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	if got := s.Current(); got != "" {
		t.Fatalf("Current() = %q after dir removed, want empty", got)
	}
	// Entry stays in recent so the UI can show it as missing.
	if got := s.Recent(); len(got) != 1 {
		t.Fatalf("Recent() = %v, want the vanished entry kept", got)
	}
}

func TestLastSessionRoundTrip(t *testing.T) {
	home := t.TempDir()
	proj := mustDir(t, t.TempDir(), "proj")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent(proj); err != nil {
		t.Fatal(err)
	}
	if got := s.LastSession(proj); got != "" {
		t.Fatalf("LastSession() = %q before any set, want empty", got)
	}
	if err := s.SetLastSession(proj, "sess_abc"); err != nil {
		t.Fatalf("SetLastSession: %v", err)
	}
	if got := s.LastSession(proj); got != "sess_abc" {
		t.Fatalf("LastSession() = %q, want %q", got, "sess_abc")
	}

	// Survives a reopen: the plugins get a fresh port every IDE start, so this
	// value must come from disk rather than from browser storage.
	s2, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.LastSession(proj); got != "sess_abc" {
		t.Fatalf("reopened LastSession() = %q, want %q", got, "sess_abc")
	}
}

func TestSetLastSessionClears(t *testing.T) {
	home := t.TempDir()
	proj := mustDir(t, t.TempDir(), "proj")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastSession(proj, "sess_abc"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastSession(proj, ""); err != nil {
		t.Fatalf("SetLastSession(clear): %v", err)
	}
	if got := s.LastSession(proj); got != "" {
		t.Fatalf("LastSession() = %q after clear, want empty", got)
	}
}

func TestSetLastSessionUpsertsUnknownDir(t *testing.T) {
	home := t.TempDir()
	proj := mustDir(t, t.TempDir(), "proj")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	// No SetCurrent first: a server started without an explicit -cwd never
	// seeds the recent list, but must still remember its last session.
	if err := s.SetLastSession(proj, "sess_abc"); err != nil {
		t.Fatalf("SetLastSession: %v", err)
	}
	recent := s.Recent()
	if len(recent) != 1 || recent[0].Path != proj || recent[0].LastSessionID != "sess_abc" {
		t.Fatalf("Recent() = %v, want an upserted entry for %q", recent, proj)
	}
	if got := s.Current(); got != "" {
		t.Fatalf("Current() = %q, want SetLastSession to leave it untouched", got)
	}
}

func TestSetLastSessionKeepsRecentOrder(t *testing.T) {
	home := t.TempDir()
	base := t.TempDir()
	a := mustDir(t, base, "a")
	b := mustDir(t, base, "b")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{a, b} {
		if err := s.SetCurrent(p); err != nil {
			t.Fatal(err)
		}
	}
	// b is current and first in recent; touching a must not reorder the list.
	if err := s.SetLastSession(a, "sess_a"); err != nil {
		t.Fatal(err)
	}
	recent := s.Recent()
	if len(recent) != 2 || recent[0].Path != b || recent[1].Path != a {
		t.Fatalf("Recent() = %v, want order unchanged [b, a]", recent)
	}
	if recent[1].LastSessionID != "sess_a" {
		t.Fatalf("Recent()[1].LastSessionID = %q, want %q", recent[1].LastSessionID, "sess_a")
	}
	if got := s.Current(); got != b {
		t.Fatalf("Current() = %q, want %q", got, b)
	}
}

func TestSetCurrentKeepsLastSession(t *testing.T) {
	// `foxxycode http --cwd <project>` re-seeds the current project on every
	// launch, which is exactly when the record must survive.
	home := t.TempDir()
	base := t.TempDir()
	a := mustDir(t, base, "a")
	b := mustDir(t, base, "b")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent(a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastSession(a, "sess_a"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastSession(b, "sess_b"); err != nil {
		t.Fatal(err)
	}

	// Reopen a: its own record survives and b's is untouched.
	if err := s.SetCurrent(a); err != nil {
		t.Fatal(err)
	}
	if got := s.LastSession(a); got != "sess_a" {
		t.Fatalf("LastSession(a) = %q after reopening a, want %q", got, "sess_a")
	}
	if got := s.LastSession(b); got != "sess_b" {
		t.Fatalf("LastSession(b) = %q, want %q", got, "sess_b")
	}
}

func TestLastSessionCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only case-insensitivity")
	}
	home := t.TempDir()
	proj := mustDir(t, t.TempDir(), "Proj")

	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastSession(proj, "sess_abc"); err != nil {
		t.Fatal(err)
	}
	if got := s.LastSession(strings.ToUpper(proj)); got != "sess_abc" {
		t.Fatalf("LastSession(upper) = %q, want %q", got, "sess_abc")
	}
	if got := s.Recent(); len(got) != 1 {
		t.Fatalf("Recent() has %d entries, want 1 (case-insensitive upsert): %v", len(got), got)
	}
}

func TestSetLastSessionValidatesDir(t *testing.T) {
	home := t.TempDir()
	s, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetLastSession("", "sess_abc"); err == nil {
		t.Fatal("SetLastSession with empty dir succeeded, want error")
	}
	if got := s.LastSession(""); got != "" {
		t.Fatalf("LastSession(\"\") = %q, want empty", got)
	}
}

func TestValidateDir(t *testing.T) {
	base := t.TempDir()
	dir := mustDir(t, base, "d")
	file := filepath.Join(base, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ValidateDir(dir)
	if err != nil {
		t.Fatalf("ValidateDir(%q): %v", dir, err)
	}
	if got != dir {
		t.Fatalf("ValidateDir(%q) = %q, want cleaned same path", dir, got)
	}
	if _, err := ValidateDir(file); err == nil {
		t.Fatal("ValidateDir(file) succeeded, want error")
	}
	if _, err := ValidateDir(""); err == nil {
		t.Fatal("ValidateDir(\"\") succeeded, want error")
	}
}
