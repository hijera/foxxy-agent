//go:build miniapps

package miniapps

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestSetWindowsPESubsystemSelectsConsoleOrUI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interpreter.exe")
	raw := make([]byte, 256)
	raw[0], raw[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(raw[0x3c:], 128)
	copy(raw[128:], []byte{'P', 'E', 0, 0})
	if err := os.WriteFile(path, raw, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	for _, test := range []struct {
		mode string
		want uint16
	}{{"console", 3}, {"ui", 2}} {
		if err := setWindowsPESubsystem(file, test.mode); err != nil {
			t.Fatal(err)
		}
		var got [2]byte
		if _, err := file.ReadAt(got[:], 128+24+68); err != nil {
			t.Fatal(err)
		}
		if binary.LittleEndian.Uint16(got[:]) != test.want {
			t.Fatalf("%s subsystem = %d, want %d", test.mode, binary.LittleEndian.Uint16(got[:]), test.want)
		}
	}
}

func TestBundleAndSingleExecutableRoundTrip(t *testing.T) {
	root := t.TempDir()
	app := greetingApp()
	app.State = StateReleased
	app.Version = "1.0.0"
	portable := Portable{
		App: app,
		Manifest: BundleManifest{
			Format: bundleFormat,
		},
		Files: map[string][]byte{"files/helper.txt": []byte("bundled")},
	}
	bundlePath := filepath.Join(root, "app.foxxyapp")
	if err := WriteBundle(bundlePath, portable); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPortable(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Files["files/helper.txt"]) != "bundled" {
		t.Fatal("bundle file did not round-trip")
	}

	interpreter := filepath.Join(root, "foxxycode.exe")
	raw := make([]byte, 256)
	raw[0], raw[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(raw[0x3c:], 128)
	copy(raw[128:], []byte{'P', 'E', 0, 0})
	if err := os.WriteFile(interpreter, raw, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "miniapp.exe")
	if err := BuildExecutable(interpreter, executable, loaded, "ui"); err != nil {
		t.Fatal(err)
	}
	embedded, mode, ok, err := ReadEmbeddedExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || mode != "ui" || embedded.App.ID != app.ID {
		t.Fatalf("embedded payload = ok:%v mode:%q app:%q", ok, mode, embedded.App.ID)
	}
}
