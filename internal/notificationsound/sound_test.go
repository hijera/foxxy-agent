package notificationsound

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreAndLoad(t *testing.T) {
	home := t.TempDir()
	data := append([]byte("ID3"), make([]byte, 64)...)
	path, err := Store(home, `../../alert:".mp3`, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(home, "notification-sounds")+string(os.PathSeparator)) {
		t.Fatalf("sound escaped its directory: %s", path)
	}
	if strings.ContainsAny(filepath.Base(path), `:"`) {
		t.Fatal("unsafe filename")
	}
	loaded, kind, err := Load(home, path)
	if err != nil || kind != "mp3" || !bytes.Equal(data, loaded) {
		t.Fatalf("load: %s, %v", kind, err)
	}
	if again, err := Store(home, `../../alert:".mp3`, data); err != nil || again != path {
		t.Fatal("identical uploads are not reused")
	}
}

func TestRejectInvalidAudio(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("not audio"), bytes.Repeat([]byte("x"), MaxBytes+1)} {
		if _, err := Format(data); err == nil {
			t.Fatal("invalid audio accepted")
		}
	}
	home := t.TempDir()
	if _, _, err := Load(home, home); err == nil {
		t.Fatal("directory accepted as sound")
	}
	if _, _, err := Load(home, "missing.wav"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestMP3WithoutID3(t *testing.T) {
	if kind, err := Format([]byte{0xff, 0xfb, 0x90, 0}); err != nil || kind != "mp3" {
		t.Fatalf("untagged MP3 rejected: %v", err)
	}
}
