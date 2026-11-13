package cmdprofile

import (
	"os"
	"path/filepath"
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
	if err := store.Record(hash, binary); err != nil {
		t.Fatal(err)
	}
	edited := ffmpegSpec()
	edited.Template = append(edited.Template, "-y")
	otherHash, _ := CanonicalHash(edited)
	if store.Trusted(otherHash, binary) {
		t.Fatal("an edited profile stayed trusted")
	}
	if store.Trusted(hash, filepath.Join(home, "elsewhere", "ffmpeg")) {
		t.Fatal("a moved binary stayed trusted")
	}
	// Recording again after a move updates the pair.
	moved := filepath.Join(home, "elsewhere", "ffmpeg")
	if err := store.Record(hash, moved); err != nil {
		t.Fatal(err)
	}
	if !store.Trusted(hash, moved) {
		t.Fatal("the updated approval was not honored")
	}
}

func TestTrustStoreSurvivesACorruptFile(t *testing.T) {
	home := t.TempDir()
	if err := writeFile(filepath.Join(home, TrustFileName), "{not json"); err != nil {
		t.Fatal(err)
	}
	store := NewTrustStore(home)
	hash, _ := CanonicalHash(ffmpegSpec())
	if store.Trusted(hash, "/usr/bin/ffmpeg") {
		t.Fatal("a corrupt store trusted a profile")
	}
	if err := store.Record(hash, "/usr/bin/ffmpeg"); err != nil {
		t.Fatalf("Record() over a corrupt file = %v", err)
	}
	if !store.Trusted(hash, "/usr/bin/ffmpeg") {
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
