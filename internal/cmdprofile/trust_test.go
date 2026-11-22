package cmdprofile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTrustStoreRoundTrip(t *testing.T) {
	home := t.TempDir()
	store := NewTrustStore(home)
	hash, err := CanonicalHash(ffmpegSpec())
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(home, "bin", "ffmpeg.exe")
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(binary, "fake binary"); err != nil {
		t.Fatal(err)
	}

	if store.Trusted(hash, binary) {
		t.Fatal("empty store trusted a profile")
	}
	if err := store.Record(hash, binary); err != nil {
		t.Fatalf("Record() = %v", err)
	}
	if !store.Trusted(hash, binary) {
		t.Fatal("recorded approval was not honored")
	}
	if _, err := os.Stat(filepath.Join(home, TrustFileName)); err != nil {
		t.Fatalf("trust file missing: %v", err)
	}
	// A second store over the same home sees the same approvals (re-read per call).
	if !NewTrustStore(home).Trusted(hash, binary) {
		t.Fatal("a fresh store did not see the persisted approval")
	}
}

// Trust binds the profile content AND the binary it resolved to: a different
// hash (edited profile) or a moved binary must re-prompt.
func TestTrustStoreRequiresBothHashAndPath(t *testing.T) {
	home := t.TempDir()
	store := NewTrustStore(home)
	hash, _ := CanonicalHash(ffmpegSpec())
	binary := filepath.Join(home, "ffmpeg")
	if err := writeFile(binary, "fake binary"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(hash, binary); err != nil {
		t.Fatal(err)
	}
	edited := ffmpegSpec()
	edited.Template = append(edited.Template, "-y")
	otherHash, _ := CanonicalHash(edited)
	if store.Trusted(otherHash, binary) {
		t.Fatal("an edited profile stayed trusted")
	}
	// Same bytes at a new path is still a moved binary: the approval binds the
	// path the operator resolved too, not only the content.
	moved := filepath.Join(home, "elsewhere", "ffmpeg")
	if err := os.MkdirAll(filepath.Dir(moved), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(moved, "fake binary"); err != nil {
		t.Fatal(err)
	}
	if store.Trusted(hash, moved) {
		t.Fatal("a moved binary stayed trusted")
	}
	// Recording again after a move updates the pair.
	if err := store.Record(hash, moved); err != nil {
		t.Fatal(err)
	}
	if !store.Trusted(hash, moved) {
		t.Fatal("the updated approval was not honored")
	}
}

// The approval binds the digest of the binary bytes: replacing the file at the
// approved path (same path, different content) must re-prompt.
func TestTrustStoreRejectsAReplacedBinary(t *testing.T) {
	home := t.TempDir()
	store := NewTrustStore(home)
	hash, _ := CanonicalHash(ffmpegSpec())
	binary := filepath.Join(home, "ffmpeg")
	if err := writeFile(binary, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(hash, binary); err != nil {
		t.Fatal(err)
	}
	if !store.Trusted(hash, binary) {
		t.Fatal("the recorded approval was not honored")
	}
	if err := writeFile(binary, "v2"); err != nil {
		t.Fatal(err)
	}
	if store.Trusted(hash, binary) {
		t.Fatal("a replaced binary stayed trusted")
	}
	// Recording again over the new bytes restores the approval.
	if err := store.Record(hash, binary); err != nil {
		t.Fatal(err)
	}
	if !store.Trusted(hash, binary) {
		t.Fatal("re-approval over the new bytes was not honored")
	}
}

// A vanished or unreadable binary cannot be trusted: the digest cannot be
// verified, so the store fails closed rather than falling back to the path.
func TestTrustStoreRejectsAnUnreadableBinary(t *testing.T) {
	home := t.TempDir()
	store := NewTrustStore(home)
	hash, _ := CanonicalHash(ffmpegSpec())
	binary := filepath.Join(home, "ffmpeg")
	if err := writeFile(binary, "fake binary"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(hash, binary); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if store.Trusted(hash, binary) {
		t.Fatal("a vanished binary stayed trusted")
	}
	if err := store.Record(hash, binary); err == nil {
		t.Fatal("Record approved a binary that is no longer on disk")
	}
}

// Windows spells the same path in different cases; the store must not re-prompt
// for case alone.
func TestTrustStoreComparesPathsCaseInsensitivelyOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive path comparison is a Windows filesystem property")
	}
	home := t.TempDir()
	store := NewTrustStore(home)
	hash, _ := CanonicalHash(ffmpegSpec())
	binary := filepath.Join(home, "FFMPEG.EXE")
	if err := writeFile(binary, "fake binary"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(hash, binary); err != nil {
		t.Fatal(err)
	}
	// TempDir may return a mixed-case root; exercise the case difference on the
	// file name, which this test fully controls.
	lower := filepath.Join(filepath.Dir(binary), "ffmpeg.exe")
	if !store.Trusted(hash, lower) {
		t.Fatal("the same binary with a different case spelling re-prompted")
	}
}

func TestTrustStoreSurvivesACorruptFile(t *testing.T) {
	home := t.TempDir()
	if err := writeFile(filepath.Join(home, TrustFileName), "{not json"); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(home, "ffmpeg")
	if err := writeFile(binary, "fake binary"); err != nil {
		t.Fatal(err)
	}
	store := NewTrustStore(home)
	hash, _ := CanonicalHash(ffmpegSpec())
	if store.Trusted(hash, binary) {
		t.Fatal("a corrupt store trusted a profile")
	}
	if err := store.Record(hash, binary); err != nil {
		t.Fatalf("Record() over a corrupt file = %v", err)
	}
	if !store.Trusted(hash, binary) {
		t.Fatal("recovery write was not honored")
	}
}

func TestTrustStoreWithEmptyHomeIsInert(t *testing.T) {
	store := NewTrustStore("")
	hash, _ := CanonicalHash(ffmpegSpec())
	if store.Trusted(hash, "/usr/bin/ffmpeg") {
		t.Fatal("a home-less store trusted a profile")
	}
	if err := store.Record(hash, "/usr/bin/ffmpeg"); err == nil {
		t.Fatal("a home-less store accepted a record")
	}
}
